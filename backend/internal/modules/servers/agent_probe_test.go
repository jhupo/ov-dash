package servers

import (
	"bufio"
	"context"
	"net"
	"strings"
	"testing"
	"time"
)

func TestAgentPortDefaultsWhenUnset(t *testing.T) {
	if got := agentPort(Connection{}); got != defaultAgentPort {
		t.Fatalf("agentPort = %d, want %d", got, defaultAgentPort)
	}
	if got := agentPort(Connection{AgentPort: 20000}); got != 20000 {
		t.Fatalf("agentPort = %d, want explicit port", got)
	}
}

func TestInstallCommandUsesRequestedPort(t *testing.T) {
	command := installCommand(20000)

	if !strings.Contains(command, "Environment=OVDASH_AGENT_PORT=20000") {
		t.Fatalf("install command did not include requested port: %s", command)
	}
	if !strings.Contains(command, agentPath) {
		t.Fatalf("install command did not include agent path %q", agentPath)
	}
}

func TestPrivilegedInstallCommandFallsBackToSudo(t *testing.T) {
	command := privilegedInstallCommand(0)

	for _, snippet := range []string{"id -u", "sudo -n", "root or passwordless sudo is required"} {
		if !strings.Contains(command, snippet) {
			t.Fatalf("privileged install command missing %q: %s", snippet, command)
		}
	}
}

func TestInstallCommandIncludesAgentStatusCheck(t *testing.T) {
	command := installCommand(0)

	for _, snippet := range []string{"StandardOutput=journal", "ovdash-agent.service", agentPath + " status"} {
		if !strings.Contains(command, snippet) {
			t.Fatalf("install command missing %q: %s", snippet, command)
		}
	}
}

func TestCollectConnWritesMetricsCommandAndDecodesResponse(t *testing.T) {
	clientConn, agentConn := net.Pipe()
	defer clientConn.Close()
	defer agentConn.Close()

	errs := make(chan error, 1)
	go func() {
		reader := bufio.NewReader(agentConn)
		command, err := reader.ReadString('\n')
		if err != nil {
			errs <- err
			return
		}
		if command != "metrics\n" {
			t.Errorf("command = %q, want metrics newline", command)
		}
		_, err = agentConn.Write([]byte(`{"cpu_percent":42,"cpu_cores":4,"memory_total_bytes":8192,"region":"edge"}` + "\n"))
		errs <- err
	}()

	probe := &AgentProbe{readTimeout: time.Second}
	metric, err := probe.CollectConn(context.Background(), Connection{ID: "server-1"}, clientConn, bufio.NewReader(clientConn))
	if err != nil {
		t.Fatalf("CollectConn returned error: %v", err)
	}
	if err := <-errs; err != nil {
		t.Fatalf("agent side returned error: %v", err)
	}
	if metric.ServerID != "server-1" || metric.CPUPercent != 42 || metric.CPUCores != 4 || metric.Region != "edge" {
		t.Fatalf("metric not decoded from agent response: %+v", metric)
	}
	if metric.LatencyMS < 0 {
		t.Fatalf("metric latency must not be negative: %f", metric.LatencyMS)
	}
}

func TestStatusConnWritesStatusCommandAndDecodesResponse(t *testing.T) {
	clientConn, agentConn := net.Pipe()
	defer clientConn.Close()
	defer agentConn.Close()

	errs := make(chan error, 1)
	go func() {
		reader := bufio.NewReader(agentConn)
		command, err := reader.ReadString('\n')
		if err != nil {
			errs <- err
			return
		}
		if command != "status\n" {
			t.Errorf("command = %q, want status newline", command)
		}
		_, err = agentConn.Write([]byte(`{"version":"2026.06.09.1","port":19087,"socat":true,"nc":false,"service_active":true}` + "\n"))
		errs <- err
	}()

	probe := &AgentProbe{readTimeout: time.Second}
	status, err := probe.StatusConn(context.Background(), Connection{ID: "server-1"}, clientConn, bufio.NewReader(clientConn))
	if err != nil {
		t.Fatalf("StatusConn returned error: %v", err)
	}
	if err := <-errs; err != nil {
		t.Fatalf("agent side returned error: %v", err)
	}
	if status.ServerID != "server-1" || status.Version != currentAgentVersion || !status.ServiceActive {
		t.Fatalf("status not decoded from agent response: %+v", status)
	}
}
