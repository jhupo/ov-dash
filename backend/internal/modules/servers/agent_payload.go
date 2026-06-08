package servers

import (
	"encoding/json"
	"time"
)

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

func decodeAgentMetric(serverID string, rawPayload string, collectedAt time.Time) (Metric, error) {
	var payload agentPayload
	if err := json.Unmarshal([]byte(rawPayload), &payload); err != nil {
		return Metric{}, err
	}
	return payload.metric(serverID, collectedAt), nil
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
