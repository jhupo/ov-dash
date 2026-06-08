package servers

import (
	"strings"
	"testing"
	"time"
)

func TestDecodeAgentStatusMapsPayload(t *testing.T) {
	checkedAt := time.Date(2026, 6, 9, 16, 0, 0, 0, time.UTC)
	status, err := decodeAgentStatus("server-1", `{"version":"2026.06.09.1","port":19087,"socat":true,"nc":false,"service_active":true}`, checkedAt)
	if err != nil {
		t.Fatalf("decodeAgentStatus returned error: %v", err)
	}
	if status.ServerID != "server-1" || status.Version != currentAgentVersion || status.Port != 19087 {
		t.Fatalf("status fields not mapped: %+v", status)
	}
	if !status.Socat || status.NC || !status.ServiceActive {
		t.Fatalf("status booleans not mapped: %+v", status)
	}
	if status.Raw == "" || status.CheckedAt != checkedAt {
		t.Fatalf("status metadata not set: %+v", status)
	}
}

func TestAgentStatusValidateVersion(t *testing.T) {
	if err := (AgentStatus{Version: currentAgentVersion}).ValidateVersion(); err != nil {
		t.Fatalf("ValidateVersion returned error: %v", err)
	}
	err := (AgentStatus{Version: "old"}).ValidateVersion()
	if err == nil || !strings.Contains(err.Error(), "agent version mismatch") {
		t.Fatalf("ValidateVersion mismatch error = %v", err)
	}
	if err := (AgentStatus{}).ValidateVersion(); err == nil || !strings.Contains(err.Error(), "missing version") {
		t.Fatalf("ValidateVersion missing error = %v", err)
	}
}
