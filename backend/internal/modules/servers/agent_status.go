package servers

import (
	"encoding/json"
	"fmt"
	"time"
)

const currentAgentVersion = "2026.06.09.1"

type AgentStatus struct {
	ServerID      string    `json:"server_id"`
	Version       string    `json:"version"`
	Port          int       `json:"port"`
	Socat         bool      `json:"socat"`
	NC            bool      `json:"nc"`
	ServiceActive bool      `json:"service_active"`
	Raw           string    `json:"raw"`
	CheckedAt     time.Time `json:"checked_at"`
}

func decodeAgentStatus(serverID string, rawPayload string, checkedAt time.Time) (AgentStatus, error) {
	var status AgentStatus
	if err := json.Unmarshal([]byte(rawPayload), &status); err != nil {
		return AgentStatus{}, err
	}
	status.ServerID = serverID
	status.Raw = rawPayload
	status.CheckedAt = checkedAt
	return status, nil
}

func (s AgentStatus) ValidateVersion() error {
	if s.Version == "" {
		return fmt.Errorf("agent status missing version")
	}
	if s.Version != currentAgentVersion {
		return fmt.Errorf("agent version mismatch: got %s want %s", s.Version, currentAgentVersion)
	}
	return nil
}
