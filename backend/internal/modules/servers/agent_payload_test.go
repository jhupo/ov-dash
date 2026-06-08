package servers

import (
	"testing"
	"time"
)

func TestDecodeAgentMetricMapsPayload(t *testing.T) {
	collectedAt := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	metric, err := decodeAgentMetric("server-1", `{
		"cpu_percent": 12.5,
		"cpu_cores": 8,
		"memory_used_bytes": 1024,
		"memory_total_bytes": 4096,
		"swap_used_bytes": 128,
		"swap_total_bytes": 512,
		"disk_used_bytes": 2048,
		"disk_total_bytes": 8192,
		"network_rx_bytes": 100,
		"network_tx_bytes": 200,
		"network_rx_rate_bps": 1.5,
		"network_tx_rate_bps": 2.5,
		"load1": 0.1,
		"load5": 0.2,
		"load15": 0.3,
		"tcp_connections": 10,
		"udp_connections": 20,
		"process_count": 30,
		"uptime_seconds": 3600,
		"architecture": "amd64",
		"virtualization": "kvm",
		"os_name": "Ubuntu",
		"cpu_model": "EPYC",
		"gpu_model": "none",
		"region": "cn"
	}`, collectedAt)
	if err != nil {
		t.Fatalf("decodeAgentMetric returned error: %v", err)
	}

	if metric.ServerID != "server-1" || metric.CollectedAt != collectedAt {
		t.Fatalf("metric identity mismatch: %+v", metric)
	}
	if metric.CPUPercent != 12.5 || metric.CPUCores != 8 || metric.MemoryTotalBytes != 4096 {
		t.Fatalf("metric numeric fields not mapped: %+v", metric)
	}
	if metric.Architecture != "amd64" || metric.Virtualization != "kvm" || metric.Region != "cn" {
		t.Fatalf("metric string fields not mapped: %+v", metric)
	}
	if metric.Raw["cpu_percent"] != 12.5 || metric.Raw["architecture"] != "amd64" {
		t.Fatalf("metric raw payload not preserved: %#v", metric.Raw)
	}
}

func TestDecodeAgentMetricRejectsInvalidJSON(t *testing.T) {
	if _, err := decodeAgentMetric("server-1", "{", time.Now()); err == nil {
		t.Fatal("decodeAgentMetric returned nil error for invalid JSON")
	}
}
