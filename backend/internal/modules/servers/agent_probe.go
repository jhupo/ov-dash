package servers

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

const (
	agentPath        = "/usr/local/bin/ovdash-agent"
	defaultAgentPort = 19087
)

type AgentProbe struct {
	ssh         *SSHExecutor
	dialTimeout time.Duration
	readTimeout time.Duration
}

func NewAgentProbe(ssh *SSHExecutor) *AgentProbe {
	return &AgentProbe{
		ssh:         ssh,
		dialTimeout: 5 * time.Second,
		readTimeout: 8 * time.Second,
	}
}

func (p *AgentProbe) Install(ctx context.Context, item Connection) error {
	client, err := p.ssh.Connect(ctx, item)
	if err != nil {
		return err
	}
	defer client.Close()

	if err := p.ssh.Run(ctx, client, privilegedInstallCommand(item.AgentPort)); err != nil {
		return err
	}
	return p.Wait(ctx, item)
}

func (p *AgentProbe) Wait(ctx context.Context, item Connection) error {
	var lastErr error
	for attempt := 0; attempt < 10; attempt++ {
		conn, err := p.Dial(ctx, item)
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

func (p *AgentProbe) Collect(ctx context.Context, item Connection) (Metric, error) {
	conn, err := p.Dial(ctx, item)
	if err != nil {
		return Metric{}, err
	}
	defer conn.Close()
	return p.CollectConn(ctx, item, conn, bufio.NewReader(conn))
}

func (p *AgentProbe) CollectConn(ctx context.Context, item Connection, conn net.Conn, reader *bufio.Reader) (Metric, error) {
	start := time.Now()

	_ = conn.SetDeadline(time.Now().Add(p.readTimeout))
	if _, err := io.WriteString(conn, "metrics\n"); err != nil {
		return Metric{}, err
	}

	output, err := reader.ReadString('\n')
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

func (p *AgentProbe) Dial(ctx context.Context, item Connection) (net.Conn, error) {
	dialer := net.Dialer{Timeout: p.dialTimeout}
	return dialer.DialContext(ctx, "tcp", fmt.Sprintf("%s:%d", item.Host, agentPort(item)))
}

func agentPort(item Connection) int {
	if item.AgentPort == 0 {
		return defaultAgentPort
	}
	return item.AgentPort
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
