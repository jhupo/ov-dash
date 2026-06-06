package servers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

const agentPath = "/usr/local/bin/ovdash-agent"

type Collector struct {
	repository *Repository
	timeout    time.Duration
}

func NewCollector(repository *Repository) *Collector {
	return &Collector{
		repository: repository,
		timeout:    20 * time.Second,
	}
}

func (c *Collector) CollectAll(ctx context.Context) {
	items, err := c.repository.List(ctx)
	if err != nil {
		return
	}
	for _, item := range items {
		select {
		case <-ctx.Done():
			return
		default:
		}
		_ = c.Collect(ctx, item.ID)
	}
}

func (c *Collector) Collect(ctx context.Context, id string) error {
	item, err := c.repository.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := c.repository.MarkCollecting(ctx, item.ID); err != nil {
		return err
	}

	metric, err := c.collect(ctx, item)
	if err != nil {
		_ = c.repository.MarkCollectFailed(ctx, item.ID, trimError(err))
		return err
	}
	return c.repository.SaveMetric(ctx, metric)
}

func (c *Collector) Install(ctx context.Context, id string) error {
	item, err := c.repository.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := c.repository.MarkCollecting(ctx, item.ID); err != nil {
		return err
	}
	if err := c.installAgent(ctx, item); err != nil {
		_ = c.repository.MarkCollectFailed(ctx, item.ID, trimError(err))
		return err
	}
	metric, err := c.collectFromAgent(ctx, item)
	if err != nil {
		_ = c.repository.MarkCollectFailed(ctx, item.ID, trimError(err))
		return err
	}
	return c.repository.SaveAgentMetric(ctx, metric)
}

func (c *Collector) RunCommand(ctx context.Context, id string, command string) (string, error) {
	item, err := c.repository.Get(ctx, id)
	if err != nil {
		return "", err
	}
	client, err := c.connect(ctx, item)
	if err != nil {
		return "", err
	}
	defer client.Close()
	return outputSSH(ctx, client, command)
}

func (c *Collector) Shell(ctx context.Context, id string) (*ssh.Client, *ssh.Session, error) {
	item, err := c.repository.Get(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	client, err := c.connect(ctx, item)
	if err != nil {
		return nil, nil, err
	}
	session, err := client.NewSession()
	if err != nil {
		client.Close()
		return nil, nil, err
	}
	if err := session.RequestPty("xterm-256color", 40, 120, ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}); err != nil {
		session.Close()
		client.Close()
		return nil, nil, err
	}
	return client, session, nil
}

func (c *Collector) collect(ctx context.Context, item Connection) (Metric, error) {
	if err := c.installAgent(ctx, item); err != nil {
		return Metric{}, err
	}
	return c.collectFromAgent(ctx, item)
}

func (c *Collector) installAgent(ctx context.Context, item Connection) error {
	client, err := c.connect(ctx, item)
	if err != nil {
		return err
	}
	defer client.Close()

	if err := runSSH(ctx, client, privilegedInstallCommand(item.AgentPort)); err != nil {
		return err
	}
	return c.waitForAgent(ctx, item)
}

func (c *Collector) waitForAgent(ctx context.Context, item Connection) error {
	var lastErr error
	for attempt := 0; attempt < 10; attempt++ {
		conn, err := c.dialAgent(ctx, item)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return lastErr
}

func (c *Collector) collectFromAgent(ctx context.Context, item Connection) (Metric, error) {
	conn, err := c.dialAgent(ctx, item)
	if err != nil {
		return Metric{}, err
	}
	defer conn.Close()
	return c.collectFromAgentConn(ctx, item, conn, bufio.NewReader(conn))
}

func (c *Collector) collectFromAgentConn(ctx context.Context, item Connection, conn net.Conn, reader *bufio.Reader) (Metric, error) {
	start := time.Now()

	_ = conn.SetDeadline(time.Now().Add(8 * time.Second))
	if _, err := io.WriteString(conn, "metrics\n"); err != nil {
		return Metric{}, err
	}

	output, err := reader.ReadString('\n')
	if err != nil {
		return Metric{}, err
	}
	latencyMS := float64(time.Since(start).Microseconds()) / 1000

	var payload agentPayload
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		return Metric{}, err
	}

	metric := payload.metric(item.ID, time.Now().UTC())
	metric.LatencyMS = latencyMS
	return metric, nil
}

func (c *Collector) dialAgent(ctx context.Context, item Connection) (net.Conn, error) {
	port := item.AgentPort
	if port == 0 {
		port = 19087
	}
	dialer := net.Dialer{Timeout: 5 * time.Second}
	return dialer.DialContext(ctx, "tcp", fmt.Sprintf("%s:%d", item.Host, port))
}

func (c *Collector) CollectAgentLoop(ctx context.Context, id string) error {
	item, err := c.repository.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := c.repository.MarkCollecting(ctx, item.ID); err != nil {
		return err
	}

	conn, err := c.dialAgent(ctx, item)
	if err != nil {
		if err := c.installAgent(ctx, item); err != nil {
			_ = c.repository.MarkCollectFailed(ctx, item.ID, trimError(err))
			return err
		}
		conn, err = c.dialAgent(ctx, item)
	}
	if err != nil {
		_ = c.repository.MarkCollectFailed(ctx, item.ID, trimError(err))
		return err
	}
	defer conn.Close()
	reader := bufio.NewReader(conn)

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		metric, err := c.collectFromAgentConn(ctx, item, conn, reader)
		if err != nil {
			_ = c.repository.MarkCollectFailed(ctx, item.ID, trimError(err))
			return err
		}
		if err := c.repository.SaveAgentMetric(ctx, metric); err != nil {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (c *Collector) connect(ctx context.Context, item Connection) (*ssh.Client, error) {
	if err := keyAuthError(item); err != nil {
		return nil, err
	}
	config := &ssh.ClientConfig{
		User:            item.Username,
		Auth:            authMethods(item),
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         c.timeout,
	}
	if len(config.Auth) == 0 {
		return nil, errors.New("missing server credential")
	}

	dialer := net.Dialer{Timeout: c.timeout}
	conn, err := dialer.DialContext(ctx, "tcp", fmt.Sprintf("%s:%d", item.Host, item.Port))
	if err != nil {
		return nil, err
	}

	done := make(chan resultClient, 1)
	go func() {
		sshConn, chans, reqs, err := ssh.NewClientConn(conn, fmt.Sprintf("%s:%d", item.Host, item.Port), config)
		if err != nil {
			done <- resultClient{err: err}
			return
		}
		done <- resultClient{client: ssh.NewClient(sshConn, chans, reqs)}
	}()

	select {
	case <-ctx.Done():
		_ = conn.Close()
		return nil, ctx.Err()
	case result := <-done:
		return result.client, result.err
	}
}

type resultClient struct {
	client *ssh.Client
	err    error
}

func authMethods(item Connection) []ssh.AuthMethod {
	methods := make([]ssh.AuthMethod, 0, 2)
	if item.AuthType == "key" && strings.TrimSpace(item.PrivateKey) != "" {
		if signer, err := ssh.ParsePrivateKey([]byte(item.PrivateKey)); err == nil {
			methods = append(methods, ssh.PublicKeys(signer))
		}
	}
	if strings.TrimSpace(item.Password) != "" {
		methods = append(methods, ssh.Password(item.Password))
	}
	return methods
}

func keyAuthError(item Connection) error {
	if item.AuthType != "key" || strings.TrimSpace(item.PrivateKey) == "" {
		return nil
	}
	_, err := ssh.ParsePrivateKey([]byte(item.PrivateKey))
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "passphrase") {
		return errors.New("private key passphrase is not supported yet")
	}
	return fmt.Errorf("invalid private key: %w", err)
}

func runSSH(ctx context.Context, client *ssh.Client, command string) error {
	_, err := outputSSH(ctx, client, command)
	return err
}

func outputSSH(ctx context.Context, client *ssh.Client, command string) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	done := make(chan error, 1)
	go func() {
		done <- session.Run(command)
	}()

	select {
	case <-ctx.Done():
		_ = session.Close()
		return "", ctx.Err()
	case err := <-done:
		if err == nil {
			return stdout.String(), nil
		}
		message := strings.TrimSpace(stderr.String())
		if message != "" {
			return "", fmt.Errorf("%w: %s", err, message)
		}
		return "", err
	}
}

func installCommand(port int) string {
	if port == 0 {
		port = 19087
	}
	return fmt.Sprintf(`cat > %[1]s <<'OVDASH_AGENT'
%[2]s
OVDASH_AGENT
chmod 0755 %[1]s
if command -v apt-get >/dev/null 2>&1 && ! command -v socat >/dev/null 2>&1; then apt-get update >/dev/null 2>&1 && apt-get install -y socat >/dev/null 2>&1 || true; fi
if command -v yum >/dev/null 2>&1 && ! command -v socat >/dev/null 2>&1; then yum install -y socat >/dev/null 2>&1 || true; fi
if command -v dnf >/dev/null 2>&1 && ! command -v socat >/dev/null 2>&1; then dnf install -y socat >/dev/null 2>&1 || true; fi
cat > /etc/systemd/system/ovdash-agent.service <<'OVDASH_SERVICE'
[Unit]
Description=OV Dash TCP metrics agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
Environment=OVDASH_AGENT_PORT=%[3]d
ExecStart=%[1]s serve
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
OVDASH_SERVICE
systemctl daemon-reload >/dev/null 2>&1 || true
systemctl enable --now ovdash-agent.service >/dev/null 2>&1 || true
systemctl restart ovdash-agent.service >/dev/null 2>&1 || true
	`, agentPath, agentScript, port)
}

func privilegedInstallCommand(port int) string {
	command := installCommand(port)
	escaped := strings.ReplaceAll(command, "'", "'\"'\"'")
	return fmt.Sprintf("if [ \"$(id -u)\" = \"0\" ]; then sh -c '%s'; elif command -v sudo >/dev/null 2>&1; then sudo -n sh -c '%s'; else echo 'root or passwordless sudo is required to install ovdash-agent' >&2; exit 1; fi", escaped, escaped)
}

func trimError(err error) string {
	message := strings.TrimSpace(err.Error())
	if len(message) > 400 {
		return message[:400]
	}
	return message
}

type agentPayload struct {
	CPUPercent       float64 `json:"cpu_percent"`
	CPUCores         int64   `json:"cpu_cores"`
	MemoryUsedBytes  int64   `json:"memory_used_bytes"`
	MemoryTotalBytes int64   `json:"memory_total_bytes"`
	SwapUsedBytes    int64   `json:"swap_used_bytes"`
	SwapTotalBytes   int64   `json:"swap_total_bytes"`
	DiskUsedBytes    int64   `json:"disk_used_bytes"`
	DiskTotalBytes   int64   `json:"disk_total_bytes"`
	NetworkRXBytes   int64   `json:"network_rx_bytes"`
	NetworkTXBytes   int64   `json:"network_tx_bytes"`
	NetworkRXRateBps float64 `json:"network_rx_rate_bps"`
	NetworkTXRateBps float64 `json:"network_tx_rate_bps"`
	Load1            float64 `json:"load1"`
	Load5            float64 `json:"load5"`
	Load15           float64 `json:"load15"`
	TCPConnections   int64   `json:"tcp_connections"`
	UDPConnections   int64   `json:"udp_connections"`
	ProcessCount     int64   `json:"process_count"`
	UptimeSeconds    int64   `json:"uptime_seconds"`
	Architecture     string  `json:"architecture"`
	Virtualization   string  `json:"virtualization"`
	OSName           string  `json:"os_name"`
	CPUModel         string  `json:"cpu_model"`
	GPUModel         string  `json:"gpu_model"`
	Region           string  `json:"region"`
}

func (p agentPayload) metric(serverID string, collectedAt time.Time) Metric {
	raw := map[string]any{}
	encoded, _ := json.Marshal(p)
	_ = json.Unmarshal(encoded, &raw)
	return Metric{
		ServerID:         serverID,
		CPUPercent:       p.CPUPercent,
		CPUCores:         p.CPUCores,
		MemoryUsedBytes:  p.MemoryUsedBytes,
		MemoryTotalBytes: p.MemoryTotalBytes,
		SwapUsedBytes:    p.SwapUsedBytes,
		SwapTotalBytes:   p.SwapTotalBytes,
		DiskUsedBytes:    p.DiskUsedBytes,
		DiskTotalBytes:   p.DiskTotalBytes,
		NetworkRXBytes:   p.NetworkRXBytes,
		NetworkTXBytes:   p.NetworkTXBytes,
		NetworkRXRateBps: p.NetworkRXRateBps,
		NetworkTXRateBps: p.NetworkTXRateBps,
		Load1:            p.Load1,
		Load5:            p.Load5,
		Load15:           p.Load15,
		TCPConnections:   p.TCPConnections,
		UDPConnections:   p.UDPConnections,
		ProcessCount:     p.ProcessCount,
		UptimeSeconds:    p.UptimeSeconds,
		Architecture:     p.Architecture,
		Virtualization:   p.Virtualization,
		OSName:           p.OSName,
		CPUModel:         p.CPUModel,
		GPUModel:         p.GPUModel,
		Region:           p.Region,
		Raw:              raw,
		CollectedAt:      collectedAt,
	}
}
