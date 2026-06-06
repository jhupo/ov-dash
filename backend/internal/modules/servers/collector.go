package servers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

func (c *Collector) collect(ctx context.Context, item Connection) (Metric, error) {
	client, err := c.connect(ctx, item)
	if err != nil {
		return Metric{}, err
	}
	defer client.Close()

	if err := runSSH(client, installCommand()); err != nil {
		return Metric{}, err
	}

	output, err := outputSSH(client, agentPath)
	if err != nil {
		return Metric{}, err
	}

	var payload agentPayload
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		return Metric{}, err
	}

	return payload.metric(item.ID, time.Now().UTC()), nil
}

func (c *Collector) connect(ctx context.Context, item Connection) (*ssh.Client, error) {
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

func runSSH(client *ssh.Client, command string) error {
	_, err := outputSSH(client, command)
	return err
}

func outputSSH(client *ssh.Client, command string) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr
	if err := session.Run(command); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message != "" {
			return "", fmt.Errorf("%w: %s", err, message)
		}
		return "", err
	}
	return stdout.String(), nil
}

func installCommand() string {
	return fmt.Sprintf("cat > %s <<'OVDASH_AGENT'\n%s\nOVDASH_AGENT\nchmod 0755 %s", agentPath, agentScript, agentPath)
}

func trimError(err error) string {
	message := strings.TrimSpace(err.Error())
	if len(message) > 400 {
		return message[:400]
	}
	return message
}

type agentPayload struct {
	CPUPercent      float64 `json:"cpu_percent"`
	MemoryUsedBytes int64   `json:"memory_used_bytes"`
	MemoryTotalBytes int64  `json:"memory_total_bytes"`
	SwapUsedBytes    int64  `json:"swap_used_bytes"`
	SwapTotalBytes   int64  `json:"swap_total_bytes"`
	DiskUsedBytes    int64  `json:"disk_used_bytes"`
	DiskTotalBytes   int64  `json:"disk_total_bytes"`
	NetworkRXBytes   int64  `json:"network_rx_bytes"`
	NetworkTXBytes   int64  `json:"network_tx_bytes"`
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
		CPUPercent:      p.CPUPercent,
		MemoryUsedBytes: p.MemoryUsedBytes,
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
