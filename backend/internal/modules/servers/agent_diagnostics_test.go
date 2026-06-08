package servers

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func TestAgentDiagnosticsReportsSSHFallbackWhenTCPUnavailable(t *testing.T) {
	probe := &AgentProbe{
		ssh:         &diagnosticSSHExecutor{},
		dialTimeout: time.Millisecond,
		readTimeout: time.Millisecond,
		dialContext: func(context.Context, string, string) (net.Conn, error) {
			return nil, errors.New("tcp unavailable")
		},
	}
	item := diagnosticConnection()

	diagnostics := probe.Diagnostics(context.Background(), item)

	if diagnostics.Status != DiagnosticWarning {
		t.Fatalf("status = %q, want warning: %+v", diagnostics.Status, diagnostics)
	}
	if diagnostics.StatusSource != "ssh_once" {
		t.Fatalf("status source = %q, want ssh_once", diagnostics.StatusSource)
	}
	if diagnostics.AgentStatus == nil || diagnostics.AgentStatus.Version != currentAgentVersion {
		t.Fatalf("agent status = %+v", diagnostics.AgentStatus)
	}
	assertCheckStatus(t, diagnostics, "tcp_status", DiagnosticWarning)
	assertCheckStatus(t, diagnostics, "ssh_status", DiagnosticOK)
	assertCheckStatus(t, diagnostics, "ssh_once_metrics", DiagnosticOK)
	assertCheckStatus(t, diagnostics, "version", DiagnosticOK)
}

func TestAgentDiagnosticsMarksNeedsUpdateOnVersionMismatch(t *testing.T) {
	probe := &AgentProbe{
		ssh:         &diagnosticSSHExecutor{statusVersion: "2026.01.01.1"},
		dialTimeout: time.Millisecond,
		readTimeout: time.Millisecond,
		dialContext: func(context.Context, string, string) (net.Conn, error) {
			return nil, errors.New("tcp unavailable")
		},
	}

	diagnostics := probe.Diagnostics(context.Background(), diagnosticConnection())

	if diagnostics.Status != DiagnosticWarning {
		t.Fatalf("status = %q, want warning: %+v", diagnostics.Status, diagnostics)
	}
	if !diagnostics.NeedsUpdate {
		t.Fatal("expected diagnostics to mark needs_update")
	}
	version := findCheck(t, diagnostics, "version")
	if version.Metadata["version"] != "2026.01.01.1" || version.Metadata["expected_version"] != currentAgentVersion {
		t.Fatalf("version metadata = %#v", version.Metadata)
	}
}

func TestAgentDiagnosticsStopsOnMissingCredential(t *testing.T) {
	probe := &AgentProbe{
		ssh:         &diagnosticSSHExecutor{},
		dialTimeout: time.Millisecond,
		readTimeout: time.Millisecond,
		dialContext: func(context.Context, string, string) (net.Conn, error) {
			return nil, errors.New("tcp unavailable")
		},
	}
	item := diagnosticConnection()
	item.Password = ""

	diagnostics := probe.Diagnostics(context.Background(), item)

	if diagnostics.Status != DiagnosticError {
		t.Fatalf("status = %q, want error: %+v", diagnostics.Status, diagnostics)
	}
	assertCheckStatus(t, diagnostics, "credential", DiagnosticError)
	assertCheckStatus(t, diagnostics, "version", DiagnosticError)
	if diagnostics.AgentStatus != nil {
		t.Fatalf("agent status = %+v, want nil", diagnostics.AgentStatus)
	}
}

func TestErrorMetadataRedactsSecrets(t *testing.T) {
	metadata := errorMetadata(errors.New("run failed password=super-secret"))

	value, _ := metadata["error"].(string)
	if strings.Contains(value, "super-secret") {
		t.Fatalf("error metadata was not redacted: %#v", metadata)
	}
}

func diagnosticConnection() Connection {
	return Connection{
		ID:       "srv-1",
		Host:     "127.0.0.1",
		Port:     22,
		Username: "root",
		AuthType: "password",
		Password: "secret",
	}
}

func assertCheckStatus(t *testing.T, diagnostics AgentDiagnostics, id string, status string) {
	t.Helper()
	check := findCheck(t, diagnostics, id)
	if check.Status != status {
		t.Fatalf("check %s status = %q, want %q: %#v", id, check.Status, status, check)
	}
}

func findCheck(t *testing.T, diagnostics AgentDiagnostics, id string) AgentDiagnosticCheck {
	t.Helper()
	for _, check := range diagnostics.Checks {
		if check.ID == id {
			return check
		}
	}
	t.Fatalf("check %s not found in %#v", id, diagnostics.Checks)
	return AgentDiagnosticCheck{}
}

type diagnosticSSHExecutor struct {
	statusVersion string
	outputErr     error
}

func (e *diagnosticSSHExecutor) Connect(ctx context.Context, item Connection) (*ssh.Client, error) {
	return nil, nil
}

func (e *diagnosticSSHExecutor) Run(ctx context.Context, client *ssh.Client, command string) error {
	_, err := e.Output(ctx, client, command)
	return err
}

func (e *diagnosticSSHExecutor) Output(ctx context.Context, client *ssh.Client, command string) (string, error) {
	if e.outputErr != nil {
		return "", e.outputErr
	}
	version := e.statusVersion
	if version == "" {
		version = currentAgentVersion
	}
	if strings.Contains(command, " status") {
		return `{"version":"` + version + `","port":19087,"socat":true,"nc":false,"service_active":true}` + "\n", nil
	}
	if strings.Contains(command, " once") {
		return `{"cpu_percent":12,"cpu_cores":2,"memory_total_bytes":1024}` + "\n", nil
	}
	return "", errors.New("unexpected command")
}
