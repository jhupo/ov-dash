package servers

import (
	"context"
	"errors"
	"time"

	"ov-dash/backend/internal/platform/redact"
)

const (
	DiagnosticOK      = "ok"
	DiagnosticWarning = "warning"
	DiagnosticError   = "error"
)

type AgentDiagnostics struct {
	ServerID        string                 `json:"server_id"`
	Status          string                 `json:"status"`
	AgentStatus     *AgentStatus           `json:"agent_status,omitempty"`
	StatusSource    string                 `json:"status_source"`
	ExpectedVersion string                 `json:"expected_version"`
	AgentPort       int                    `json:"agent_port"`
	NeedsUpdate     bool                   `json:"needs_update"`
	Checks          []AgentDiagnosticCheck `json:"checks"`
	CheckedAt       time.Time              `json:"checked_at"`
}

type AgentDiagnosticCheck struct {
	ID       string         `json:"id"`
	Status   string         `json:"status"`
	Message  string         `json:"message"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

func (p *AgentProbe) Diagnostics(ctx context.Context, item Connection) AgentDiagnostics {
	diagnostics := newAgentDiagnostics(item, time.Now().UTC())

	if err := keyAuthError(item); err != nil {
		diagnostics.addCheck("credential", DiagnosticError, "server credential is invalid", errorMetadata(err))
		diagnostics.finalize()
		return diagnostics
	}
	if len(authMethods(item)) == 0 {
		diagnostics.addCheck("credential", DiagnosticError, "server credential is missing", nil)
		diagnostics.finalize()
		return diagnostics
	}
	diagnostics.addCheck("credential", DiagnosticOK, "server credential is configured", map[string]any{
		"auth_type": item.AuthType,
	})

	tcpStatus, tcpErr := p.StatusTCP(ctx, item)
	if tcpErr != nil {
		diagnostics.addCheck("tcp_status", DiagnosticWarning, "agent TCP status is unavailable", errorMetadata(tcpErr))
	} else {
		diagnostics.addCheck("tcp_status", DiagnosticOK, "agent TCP status is available", nil)
		diagnostics.setAgentStatus("tcp", tcpStatus)
	}

	sshStatus, sshErr := p.StatusOnce(ctx, item)
	if sshErr != nil {
		status := DiagnosticWarning
		if diagnostics.AgentStatus == nil {
			status = DiagnosticError
		}
		diagnostics.addCheck("ssh_status", status, "agent SSH status fallback is unavailable", errorMetadata(sshErr))
	} else {
		diagnostics.addCheck("ssh_status", DiagnosticOK, "agent SSH status fallback is available", nil)
		if diagnostics.AgentStatus == nil {
			diagnostics.setAgentStatus("ssh_once", sshStatus)
		}
	}

	metric, metricErr := p.CollectOnce(ctx, item)
	if metricErr != nil {
		status := DiagnosticWarning
		if tcpErr != nil {
			status = DiagnosticError
		}
		diagnostics.addCheck("ssh_once_metrics", status, "agent SSH one-shot metrics collection failed", errorMetadata(metricErr))
	} else {
		diagnostics.addCheck("ssh_once_metrics", DiagnosticOK, "agent SSH one-shot metrics collection works", map[string]any{
			"latency_ms": metric.LatencyMS,
		})
	}

	diagnostics.finalize()
	return diagnostics
}

func newAgentDiagnostics(item Connection, checkedAt time.Time) AgentDiagnostics {
	return AgentDiagnostics{
		ServerID:        item.ID,
		Status:          DiagnosticOK,
		ExpectedVersion: currentAgentVersion,
		AgentPort:       agentPort(item),
		Checks:          []AgentDiagnosticCheck{},
		CheckedAt:       checkedAt,
	}
}

func (d *AgentDiagnostics) setAgentStatus(source string, status AgentStatus) {
	statusCopy := status
	d.AgentStatus = &statusCopy
	d.StatusSource = source
}

func (d *AgentDiagnostics) addCheck(id string, status string, message string, metadata map[string]any) {
	d.Checks = append(d.Checks, AgentDiagnosticCheck{
		ID:       id,
		Status:   status,
		Message:  message,
		Metadata: metadata,
	})
}

func (d *AgentDiagnostics) finalize() {
	if d.AgentStatus == nil {
		d.addCheck("version", DiagnosticError, "agent status is unavailable", nil)
		d.setOverallStatus()
		return
	}

	if err := d.AgentStatus.ValidateVersion(); err != nil {
		status := DiagnosticWarning
		if errors.Is(err, ErrAgentStatusMissingVersion) {
			status = DiagnosticError
		}
		d.NeedsUpdate = true
		d.addCheck("version", status, "agent version is not current", map[string]any{
			"version":          d.AgentStatus.Version,
			"expected_version": currentAgentVersion,
			"error":            err.Error(),
		})
	} else {
		d.addCheck("version", DiagnosticOK, "agent version is current", map[string]any{
			"version": d.AgentStatus.Version,
		})
	}

	if d.AgentStatus.Port != 0 && d.AgentStatus.Port != d.AgentPort {
		d.addCheck("port", DiagnosticWarning, "agent reported a different port than the server configuration", map[string]any{
			"reported_port": d.AgentStatus.Port,
			"expected_port": d.AgentPort,
		})
	} else {
		d.addCheck("port", DiagnosticOK, "agent port matches server configuration", map[string]any{
			"port": d.AgentPort,
		})
	}

	if d.AgentStatus.ServiceActive {
		d.addCheck("service", DiagnosticOK, "agent service is active", nil)
	} else {
		d.addCheck("service", DiagnosticWarning, "agent service is not active", nil)
	}

	if d.AgentStatus.Socat || d.AgentStatus.NC {
		d.addCheck("listener_dependency", DiagnosticOK, "agent listener dependency is available", map[string]any{
			"socat": d.AgentStatus.Socat,
			"nc":    d.AgentStatus.NC,
		})
	} else {
		d.addCheck("listener_dependency", DiagnosticWarning, "agent listener dependency is missing", map[string]any{
			"socat": d.AgentStatus.Socat,
			"nc":    d.AgentStatus.NC,
		})
	}

	d.setOverallStatus()
}

func (d *AgentDiagnostics) setOverallStatus() {
	status := DiagnosticOK
	for _, check := range d.Checks {
		switch check.Status {
		case DiagnosticError:
			d.Status = DiagnosticError
			return
		case DiagnosticWarning:
			status = DiagnosticWarning
		}
	}
	d.Status = status
}

func errorMetadata(err error) map[string]any {
	if err == nil {
		return nil
	}
	return map[string]any{"error": redact.Text(trimError(err))}
}
