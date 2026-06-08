package servers

import (
	"context"
	"encoding/json"
	"time"

	"ov-dash/backend/internal/db"
)

type MetricsStore struct {
	db *db.Pool
}

func NewMetricsStore(db *db.Pool) *MetricsStore {
	return &MetricsStore{db: db}
}

func (s *MetricsStore) Samples(ctx context.Context, id string, since time.Time) ([]Metric, error) {
	rows, err := s.db.Query(ctx, `
		SELECT
			server_id, cpu_percent, cpu_cores, latency_ms, memory_used_bytes, memory_total_bytes,
			swap_used_bytes, swap_total_bytes, disk_used_bytes, disk_total_bytes,
			network_rx_bytes, network_tx_bytes, network_rx_rate_bps, network_tx_rate_bps,
			load1, load5, load15, tcp_connections, udp_connections, process_count, uptime_seconds, architecture, virtualization,
			os_name, cpu_model, gpu_model, region, raw, collected_at
		FROM server_metric_samples
		WHERE server_id = $1
		  AND collected_at >= $2
		ORDER BY collected_at ASC
	`, id, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]Metric, 0)
	for rows.Next() {
		item, err := scanMetricSample(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *MetricsStore) Save(ctx context.Context, metric Metric, scheduleNext bool) error {
	raw, err := json.Marshal(metric.Raw)
	if err != nil {
		return err
	}

	_, err = s.db.Exec(ctx, `
		INSERT INTO server_metrics (
			server_id, cpu_percent, cpu_cores, latency_ms, memory_used_bytes, memory_total_bytes,
			swap_used_bytes, swap_total_bytes, disk_used_bytes, disk_total_bytes,
			network_rx_bytes, network_tx_bytes, network_rx_rate_bps, network_tx_rate_bps,
			load1, load5, load15, tcp_connections, udp_connections, process_count, uptime_seconds, architecture, virtualization,
			os_name, cpu_model, gpu_model, region, raw, collected_at
		)
		VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12,
			$13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28, $29
		)
		ON CONFLICT (server_id) DO UPDATE SET
			cpu_percent = EXCLUDED.cpu_percent,
			cpu_cores = EXCLUDED.cpu_cores,
			latency_ms = EXCLUDED.latency_ms,
			memory_used_bytes = EXCLUDED.memory_used_bytes,
			memory_total_bytes = EXCLUDED.memory_total_bytes,
			swap_used_bytes = EXCLUDED.swap_used_bytes,
			swap_total_bytes = EXCLUDED.swap_total_bytes,
			disk_used_bytes = EXCLUDED.disk_used_bytes,
			disk_total_bytes = EXCLUDED.disk_total_bytes,
			network_rx_bytes = EXCLUDED.network_rx_bytes,
			network_tx_bytes = EXCLUDED.network_tx_bytes,
			network_rx_rate_bps = EXCLUDED.network_rx_rate_bps,
			network_tx_rate_bps = EXCLUDED.network_tx_rate_bps,
			load1 = EXCLUDED.load1,
			load5 = EXCLUDED.load5,
			load15 = EXCLUDED.load15,
			tcp_connections = EXCLUDED.tcp_connections,
			udp_connections = EXCLUDED.udp_connections,
			process_count = EXCLUDED.process_count,
			uptime_seconds = EXCLUDED.uptime_seconds,
			architecture = EXCLUDED.architecture,
			virtualization = EXCLUDED.virtualization,
			os_name = EXCLUDED.os_name,
			cpu_model = EXCLUDED.cpu_model,
			gpu_model = EXCLUDED.gpu_model,
			region = EXCLUDED.region,
			raw = EXCLUDED.raw,
			collected_at = EXCLUDED.collected_at
	`, metric.ServerID, metric.CPUPercent, metric.CPUCores, metric.LatencyMS, metric.MemoryUsedBytes, metric.MemoryTotalBytes,
		metric.SwapUsedBytes, metric.SwapTotalBytes, metric.DiskUsedBytes, metric.DiskTotalBytes,
		metric.NetworkRXBytes, metric.NetworkTXBytes, metric.NetworkRXRateBps, metric.NetworkTXRateBps,
		metric.Load1, metric.Load5, metric.Load15, metric.TCPConnections, metric.UDPConnections, metric.ProcessCount, metric.UptimeSeconds, metric.Architecture,
		metric.Virtualization, metric.OSName, metric.CPUModel, metric.GPUModel, metric.Region, raw, metric.CollectedAt)
	if err != nil {
		return err
	}

	_, err = s.db.Exec(ctx, `
		INSERT INTO server_metric_samples (
			server_id, cpu_percent, cpu_cores, latency_ms, memory_used_bytes, memory_total_bytes,
			swap_used_bytes, swap_total_bytes, disk_used_bytes, disk_total_bytes,
			network_rx_bytes, network_tx_bytes, network_rx_rate_bps, network_tx_rate_bps,
			load1, load5, load15, tcp_connections, udp_connections, process_count, uptime_seconds, architecture, virtualization,
			os_name, cpu_model, gpu_model, region, raw, collected_at
		)
		VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12,
			$13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28, $29
		)
	`, metric.ServerID, metric.CPUPercent, metric.CPUCores, metric.LatencyMS, metric.MemoryUsedBytes, metric.MemoryTotalBytes,
		metric.SwapUsedBytes, metric.SwapTotalBytes, metric.DiskUsedBytes, metric.DiskTotalBytes,
		metric.NetworkRXBytes, metric.NetworkTXBytes, metric.NetworkRXRateBps, metric.NetworkTXRateBps,
		metric.Load1, metric.Load5, metric.Load15, metric.TCPConnections, metric.UDPConnections, metric.ProcessCount, metric.UptimeSeconds, metric.Architecture,
		metric.Virtualization, metric.OSName, metric.CPUModel, metric.GPUModel, metric.Region, raw, metric.CollectedAt)
	if err != nil {
		return err
	}

	_, err = s.db.Exec(ctx, `
		UPDATE server_connections
		SET collector_installed = true,
		    collect_status = 'ok',
		    collect_error = '',
		    collect_failure_count = 0,
		    last_collected_at = $2::timestamptz,
		    agent_last_seen_at = $2::timestamptz,
		    next_collect_at = CASE
		        WHEN $3::boolean THEN $2::timestamptz + make_interval(secs => collect_interval_seconds)
		        ELSE $2::timestamptz + interval '20 seconds'
		    END,
		    updated_at = now()
		WHERE id = $1
	`, metric.ServerID, metric.CollectedAt, scheduleNext)
	return err
}

type metricScanner interface {
	Scan(dest ...any) error
}

func scanMetricSample(row metricScanner) (Metric, error) {
	var item Metric
	var raw []byte
	if err := row.Scan(
		&item.ServerID,
		&item.CPUPercent,
		&item.CPUCores,
		&item.LatencyMS,
		&item.MemoryUsedBytes,
		&item.MemoryTotalBytes,
		&item.SwapUsedBytes,
		&item.SwapTotalBytes,
		&item.DiskUsedBytes,
		&item.DiskTotalBytes,
		&item.NetworkRXBytes,
		&item.NetworkTXBytes,
		&item.NetworkRXRateBps,
		&item.NetworkTXRateBps,
		&item.Load1,
		&item.Load5,
		&item.Load15,
		&item.TCPConnections,
		&item.UDPConnections,
		&item.ProcessCount,
		&item.UptimeSeconds,
		&item.Architecture,
		&item.Virtualization,
		&item.OSName,
		&item.CPUModel,
		&item.GPUModel,
		&item.Region,
		&raw,
		&item.CollectedAt,
	); err != nil {
		return Metric{}, err
	}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &item.Raw)
	}
	return item, nil
}
