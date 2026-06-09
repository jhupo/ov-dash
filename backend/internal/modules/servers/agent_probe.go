package servers

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

const (
	agentPath        = "/usr/local/bin/ovdash-agent"
	defaultAgentPort = 19087
)

type AgentProbe struct {
	ssh         agentSSHExecutor
	dialTimeout time.Duration
	readTimeout time.Duration
	dialContext func(ctx context.Context, network string, address string) (net.Conn, error)
}

func NewAgentProbe(ssh *SSHExecutor) *AgentProbe {
	return &AgentProbe{
		ssh:         ssh,
		dialTimeout: 5 * time.Second,
		readTimeout: 8 * time.Second,
	}
}

type agentSSHExecutor interface {
	Connect(ctx context.Context, item Connection) (*ssh.Client, error)
	Run(ctx context.Context, client *ssh.Client, command string) error
	RunResult(ctx context.Context, client *ssh.Client, command string) (SSHCommandResult, error)
	Output(ctx context.Context, client *ssh.Client, command string) (string, error)
}

func (p *AgentProbe) Install(ctx context.Context, item Connection) error {
	return p.InstallWithObserver(ctx, item, nil)
}

func (p *AgentProbe) InstallWithObserver(ctx context.Context, item Connection, observer CollectionObserver) error {
	observeCollection(ctx, observer, item.ID, "agent.install.start", "", "installing server agent", nil)
	client, err := p.ssh.Connect(ctx, item)
	if err != nil {
		observeCollection(ctx, observer, item.ID, "agent.install.failed", "", "connect for server agent install failed", map[string]any{"error": err.Error()})
		return err
	}
	defer closeSSHClient(client)

	result, err := p.ssh.RunResult(ctx, client, privilegedInstallCommand(item.AgentPort))
	p.observeInstallOutput(ctx, observer, item.ID, result)
	if err != nil {
		observeCollection(ctx, observer, item.ID, "agent.install.failed", "", "server agent install command failed", map[string]any{"error": err.Error()})
		return err
	}
	observeCollection(ctx, observer, item.ID, "agent.install.command_ok", "", "server agent install command completed", nil)
	status, err := p.Wait(ctx, item)
	if err != nil {
		observeCollection(ctx, observer, item.ID, "agent.install.status_failed", "", "server agent status check failed after install", map[string]any{"error": err.Error()})
		return err
	}
	if err := status.ValidateVersion(); err != nil {
		observeCollection(ctx, observer, item.ID, "agent.install.version_failed", "", "server agent version check failed after install", map[string]any{
			"version":          status.Version,
			"expected_version": currentAgentVersion,
			"error":            err.Error(),
		})
		return err
	}
	observeCollection(ctx, observer, item.ID, "agent.install.ready", "", "server agent is installed and current", map[string]any{
		"version": status.Version,
		"port":    status.Port,
	})
	return nil
}

func (p *AgentProbe) observeInstallOutput(ctx context.Context, observer CollectionObserver, serverID string, result SSHCommandResult) {
	if strings.TrimSpace(result.Stdout) != "" {
		observeCollection(ctx, observer, serverID, "agent.install.stdout", "", strings.TrimSpace(result.Stdout), nil)
	}
	if strings.TrimSpace(result.Stderr) != "" {
		observeCollection(ctx, observer, serverID, "agent.install.stderr", "", strings.TrimSpace(result.Stderr), nil)
	}
}

func (p *AgentProbe) EnsureCurrent(ctx context.Context, item Connection) error {
	status, err := p.Status(ctx, item)
	if err == nil && status.ValidateVersion() == nil {
		return nil
	}
	if errors.Is(err, errMissingServerCredential) {
		return err
	}
	return p.Install(ctx, item)
}

func (p *AgentProbe) Wait(ctx context.Context, item Connection) (AgentStatus, error) {
	var lastErr error
	for attempt := 0; attempt < 10; attempt++ {
		status, err := p.Status(ctx, item)
		if err == nil {
			return status, nil
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return AgentStatus{}, ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return AgentStatus{}, lastErr
}

func (p *AgentProbe) Status(ctx context.Context, item Connection) (AgentStatus, error) {
	status, err := p.StatusTCP(ctx, item)
	if err == nil {
		return status, nil
	}
	status, fallbackErr := p.StatusOnce(ctx, item)
	if fallbackErr == nil {
		return status, nil
	}
	return AgentStatus{}, fmt.Errorf("%w; ssh status fallback failed: %v", err, fallbackErr)
}

func (p *AgentProbe) StatusTCP(ctx context.Context, item Connection) (AgentStatus, error) {
	conn, err := p.Dial(ctx, item)
	if err != nil {
		return AgentStatus{}, err
	}
	defer conn.Close()
	return p.StatusConn(ctx, item, conn, bufio.NewReader(conn))
}

func (p *AgentProbe) StatusOnce(ctx context.Context, item Connection) (AgentStatus, error) {
	client, err := p.ssh.Connect(ctx, item)
	if err != nil {
		return AgentStatus{}, err
	}
	defer closeSSHClient(client)

	output, err := p.ssh.Output(ctx, client, agentPath+" status")
	if err != nil {
		return AgentStatus{}, err
	}
	return decodeAgentStatus(item.ID, output, time.Now().UTC())
}

func (p *AgentProbe) StatusConn(ctx context.Context, item Connection, conn net.Conn, reader *bufio.Reader) (AgentStatus, error) {
	output, err := p.requestConn(ctx, conn, reader, "status\n")
	if err != nil {
		return AgentStatus{}, err
	}
	return decodeAgentStatus(item.ID, output, time.Now().UTC())
}

func (p *AgentProbe) Collect(ctx context.Context, item Connection) (Metric, error) {
	conn, err := p.Dial(ctx, item)
	if err != nil {
		return Metric{}, err
	}
	defer conn.Close()
	return p.CollectConn(ctx, item, conn, bufio.NewReader(conn))
}

func (p *AgentProbe) CollectOnce(ctx context.Context, item Connection) (Metric, error) {
	client, err := p.ssh.Connect(ctx, item)
	if err != nil {
		return Metric{}, err
	}
	defer closeSSHClient(client)

	start := time.Now()
	output, err := p.ssh.Output(ctx, client, agentPath+" once")
	if err != nil {
		return Metric{}, err
	}
	metric, err := decodeAgentMetric(item.ID, output, time.Now().UTC())
	if err != nil {
		return Metric{}, err
	}
	metric.LatencyMS = float64(time.Since(start).Microseconds()) / 1000
	return metric, nil
}

func (p *AgentProbe) CollectConn(ctx context.Context, item Connection, conn net.Conn, reader *bufio.Reader) (Metric, error) {
	start := time.Now()

	output, err := p.requestConn(ctx, conn, reader, "metrics\n")
	if err != nil {
		return Metric{}, err
	}
	latencyMS := float64(time.Since(start).Microseconds()) / 1000

	metric, err := decodeAgentMetric(item.ID, output, time.Now().UTC())
	if err != nil {
		return Metric{}, err
	}
	metric.LatencyMS = latencyMS
	return metric, nil
}

func (p *AgentProbe) requestConn(ctx context.Context, conn net.Conn, reader *bufio.Reader, command string) (string, error) {
	deadline := time.Now().Add(p.readTimeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	_ = conn.SetDeadline(deadline)
	if _, err := io.WriteString(conn, command); err != nil {
		return "", err
	}
	return reader.ReadString('\n')
}

func (p *AgentProbe) Dial(ctx context.Context, item Connection) (net.Conn, error) {
	address := fmt.Sprintf("%s:%d", item.Host, agentPort(item))
	if p.dialContext != nil {
		return p.dialContext(ctx, "tcp", address)
	}
	dialer := net.Dialer{Timeout: p.dialTimeout}
	return dialer.DialContext(ctx, "tcp", address)
}

func agentPort(item Connection) int {
	if item.AgentPort == 0 {
		return defaultAgentPort
	}
	return item.AgentPort
}

func closeSSHClient(client *ssh.Client) {
	if client != nil {
		_ = client.Close()
	}
}

func installCommand(port int) string {
	if port == 0 {
		port = defaultAgentPort
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
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
OVDASH_SERVICE
systemctl daemon-reload >/dev/null 2>&1 || true
systemctl enable --now ovdash-agent.service >/dev/null 2>&1 || true
systemctl restart ovdash-agent.service >/dev/null 2>&1 || true
%[1]s status
	`, agentPath, agentScript, port)
}

func privilegedInstallCommand(port int) string {
	command := installCommand(port)
	escaped := strings.ReplaceAll(command, "'", "'\"'\"'")
	return fmt.Sprintf("if [ \"$(id -u)\" = \"0\" ]; then sh -c '%s'; elif command -v sudo >/dev/null 2>&1; then sudo -n sh -c '%s'; else echo 'root or passwordless sudo is required to install ovdash-agent' >&2; exit 1; fi", escaped, escaped)
}
