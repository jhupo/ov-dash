package servers

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"ov-dash/backend/internal/db"

	"github.com/jackc/pgx/v5"
)

type Repository struct {
	db *db.Pool
}

func NewRepository(db *db.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) List(ctx context.Context) ([]Connection, error) {
	rows, err := r.db.Query(ctx, `
		SELECT
			c.id, c.name, c.group_name, c.region, c.host, c.port, c.username, c.auth_type,
			c.password, c.private_key, c.expires_at, c.collect_interval_seconds, c.next_collect_at, c.collector_installed, c.agent_port,
			c.collect_status, c.collect_error, c.last_collected_at, c.agent_last_seen_at, c.created_at, c.updated_at,
			m.server_id, m.cpu_percent, m.cpu_cores, m.latency_ms, m.memory_used_bytes, m.memory_total_bytes,
			m.swap_used_bytes, m.swap_total_bytes, m.disk_used_bytes, m.disk_total_bytes,
			m.network_rx_bytes, m.network_tx_bytes, m.network_rx_rate_bps, m.network_tx_rate_bps,
			m.load1, m.load5, m.load15, m.tcp_connections, m.udp_connections, m.process_count, m.uptime_seconds, m.architecture, m.virtualization,
			m.os_name, m.cpu_model, m.gpu_model, m.region, m.raw, m.collected_at
		FROM server_connections c
		LEFT JOIN server_metrics m ON m.server_id = c.id
		ORDER BY c.group_name ASC, c.name ASC, c.host ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]Connection, 0)
	for rows.Next() {
		item, err := scanConnection(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) Get(ctx context.Context, id string) (Connection, error) {
	row := r.db.QueryRow(ctx, `
		SELECT
			c.id, c.name, c.group_name, c.region, c.host, c.port, c.username, c.auth_type,
			c.password, c.private_key, c.expires_at, c.collect_interval_seconds, c.next_collect_at, c.collector_installed, c.agent_port,
			c.collect_status, c.collect_error, c.last_collected_at, c.agent_last_seen_at, c.created_at, c.updated_at,
			m.server_id, m.cpu_percent, m.cpu_cores, m.latency_ms, m.memory_used_bytes, m.memory_total_bytes,
			m.swap_used_bytes, m.swap_total_bytes, m.disk_used_bytes, m.disk_total_bytes,
			m.network_rx_bytes, m.network_tx_bytes, m.network_rx_rate_bps, m.network_tx_rate_bps,
			m.load1, m.load5, m.load15, m.tcp_connections, m.udp_connections, m.process_count, m.uptime_seconds, m.architecture, m.virtualization,
			m.os_name, m.cpu_model, m.gpu_model, m.region, m.raw, m.collected_at
		FROM server_connections c
		LEFT JOIN server_metrics m ON m.server_id = c.id
		WHERE c.id = $1
	`, id)
	return scanConnection(row)
}

func (r *Repository) Upsert(ctx context.Context, input SaveInput) (Connection, error) {
	var item Connection
	err := r.db.QueryRow(ctx, `
		INSERT INTO server_connections (
			id, name, group_name, region, host, port, username, auth_type,
			password, private_key, expires_at, collect_interval_seconds, next_collect_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, COALESCE($9, ''), COALESCE($10, ''), $11, $13, now())
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			group_name = EXCLUDED.group_name,
			region = EXCLUDED.region,
			host = EXCLUDED.host,
			port = EXCLUDED.port,
			username = EXCLUDED.username,
			auth_type = EXCLUDED.auth_type,
			expires_at = EXCLUDED.expires_at,
			collect_interval_seconds = EXCLUDED.collect_interval_seconds,
			password = CASE
				WHEN $12 THEN ''
				WHEN $9::text IS NULL THEN server_connections.password
				ELSE EXCLUDED.password
			END,
			private_key = CASE
				WHEN $12 THEN ''
				WHEN $10::text IS NULL THEN server_connections.private_key
				ELSE EXCLUDED.private_key
			END,
			collect_status = 'pending',
			collect_error = '',
			updated_at = now()
		RETURNING id, name, group_name, region, host, port, username, auth_type,
		          password, private_key, expires_at, collect_interval_seconds, next_collect_at, collector_installed, agent_port,
		          collect_status, collect_error, last_collected_at, agent_last_seen_at, created_at, updated_at
	`,
		input.ID,
		input.Name,
		input.GroupName,
		input.Region,
		input.Host,
		input.Port,
		input.Username,
		input.AuthType,
		input.Password,
		input.PrivateKey,
		input.ExpiresAt,
		input.ClearSecret,
		input.CollectInterval,
	).Scan(
		&item.ID,
		&item.Name,
		&item.GroupName,
		&item.Region,
		&item.Host,
		&item.Port,
		&item.Username,
		&item.AuthType,
		&item.Password,
		&item.PrivateKey,
		&item.ExpiresAt,
		&item.CollectInterval,
		&item.NextCollectAt,
		&item.CollectorInstalled,
		&item.AgentPort,
		&item.CollectStatus,
		&item.CollectError,
		&item.LastCollectedAt,
		&item.AgentLastSeenAt,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	return item, err
}

func (r *Repository) DueForCollection(ctx context.Context, limit int) ([]Connection, error) {
	active, err := r.MonitorActive(ctx)
	if err != nil {
		return nil, err
	}
	if !active {
		return []Connection{}, nil
	}
	if err := r.ResetStaleCollecting(ctx, 2*time.Minute); err != nil {
		return nil, err
	}
	rows, err := r.db.Query(ctx, `
		SELECT
			c.id, c.name, c.group_name, c.region, c.host, c.port, c.username, c.auth_type,
			c.password, c.private_key, c.expires_at, c.collect_interval_seconds, c.next_collect_at,
			c.collector_installed, c.agent_port, c.collect_status, c.collect_error, c.last_collected_at,
			c.agent_last_seen_at, c.created_at, c.updated_at,
			m.server_id, m.cpu_percent, m.cpu_cores, m.latency_ms, m.memory_used_bytes, m.memory_total_bytes,
			m.swap_used_bytes, m.swap_total_bytes, m.disk_used_bytes, m.disk_total_bytes,
			m.network_rx_bytes, m.network_tx_bytes, m.network_rx_rate_bps, m.network_tx_rate_bps,
			m.load1, m.load5, m.load15, m.tcp_connections, m.udp_connections, m.process_count, m.uptime_seconds, m.architecture, m.virtualization,
			m.os_name, m.cpu_model, m.gpu_model, m.region, m.raw, m.collected_at
		FROM server_connections c
		LEFT JOIN server_metrics m ON m.server_id = c.id
		WHERE c.next_collect_at <= now()
		  AND c.collect_status <> 'collecting'
		  AND (c.agent_last_seen_at IS NULL OR c.agent_last_seen_at < now() - interval '10 seconds')
		ORDER BY c.next_collect_at ASC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]Connection, 0)
	for rows.Next() {
		item, err := scanConnection(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) TouchMonitorActivity(ctx context.Context, ttl time.Duration) error {
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	seconds := int(ttl / time.Second)
	if ttl%time.Second != 0 {
		seconds++
	}
	if seconds < 1 {
		seconds = 1
	}
	_, err := r.db.Exec(ctx, `
		INSERT INTO server_monitor_activity (id, active_until)
		VALUES (true, now() + make_interval(secs => $1))
		ON CONFLICT (id) DO UPDATE SET
			active_until = GREATEST(server_monitor_activity.active_until, EXCLUDED.active_until),
			updated_at = now()
	`, seconds)
	return err
}

func (r *Repository) MonitorActive(ctx context.Context) (bool, error) {
	var active bool
	err := r.db.QueryRow(ctx, `
		SELECT COALESCE(max(active_until) > now(), false)
		FROM server_monitor_activity
	`).Scan(&active)
	return active, err
}

func (r *Repository) ResetStaleCollecting(ctx context.Context, maxAge time.Duration) error {
	_, err := r.db.Exec(ctx, `
		UPDATE server_connections
		SET collect_status = 'pending',
		    collect_error = '采集超时，已重新进入队列',
		    next_collect_at = now(),
		    updated_at = now()
		WHERE collect_status = 'collecting'
		  AND updated_at < now() - $1::interval
	`, maxAge.String())
	return err
}

func (r *Repository) MarkCollectQueued(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE server_connections
		SET collect_status = 'pending',
		    collect_error = '',
		    next_collect_at = now() + make_interval(secs => collect_interval_seconds),
		    updated_at = now()
		WHERE id = $1
	`, id)
	return err
}

func (r *Repository) Samples(ctx context.Context, id string, since time.Time) ([]Metric, error) {
	rows, err := r.db.Query(ctx, `
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

func (r *Repository) Delete(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM server_connections WHERE id = $1`, id)
	return err
}

func (r *Repository) MarkCollecting(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE server_connections
		SET collect_status = 'collecting', collect_error = '', updated_at = now()
		WHERE id = $1
	`, id)
	return err
}

func (r *Repository) MarkCollectFailed(ctx context.Context, id string, message string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE server_connections
		SET collect_status = 'error',
		    collect_error = $2,
		    next_collect_at = now() + make_interval(secs => collect_interval_seconds),
		    updated_at = now()
		WHERE id = $1
	`, id, message)
	return err
}

func (r *Repository) SaveMetric(ctx context.Context, metric Metric) error {
	return r.saveMetric(ctx, metric, true)
}

func (r *Repository) SaveAgentMetric(ctx context.Context, metric Metric) error {
	return r.saveMetric(ctx, metric, false)
}

func (r *Repository) saveMetric(ctx context.Context, metric Metric, scheduleNext bool) error {
	raw, err := json.Marshal(metric.Raw)
	if err != nil {
		return err
	}

	_, err = r.db.Exec(ctx, `
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

	_, err = r.db.Exec(ctx, `
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

	_, err = r.db.Exec(ctx, `
		UPDATE server_connections
		SET collector_installed = true,
		    collect_status = 'ok',
		    collect_error = '',
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

type connectionScanner interface {
	Scan(dest ...any) error
}

func scanConnection(row connectionScanner) (Connection, error) {
	var item Connection
	var metricID sql.NullString
	var cpuPercent sql.NullFloat64
	var cpuCores sql.NullInt64
	var latencyMS sql.NullFloat64
	var memoryUsedBytes sql.NullInt64
	var memoryTotalBytes sql.NullInt64
	var swapUsedBytes sql.NullInt64
	var swapTotalBytes sql.NullInt64
	var diskUsedBytes sql.NullInt64
	var diskTotalBytes sql.NullInt64
	var networkRXBytes sql.NullInt64
	var networkTXBytes sql.NullInt64
	var networkRXRateBps sql.NullFloat64
	var networkTXRateBps sql.NullFloat64
	var load1 sql.NullFloat64
	var load5 sql.NullFloat64
	var load15 sql.NullFloat64
	var tcpConnections sql.NullInt64
	var udpConnections sql.NullInt64
	var processCount sql.NullInt64
	var uptimeSeconds sql.NullInt64
	var architecture sql.NullString
	var virtualization sql.NullString
	var osName sql.NullString
	var cpuModel sql.NullString
	var gpuModel sql.NullString
	var region sql.NullString
	var collectedAt sql.NullTime
	var raw []byte
	err := row.Scan(
		&item.ID,
		&item.Name,
		&item.GroupName,
		&item.Region,
		&item.Host,
		&item.Port,
		&item.Username,
		&item.AuthType,
		&item.Password,
		&item.PrivateKey,
		&item.ExpiresAt,
		&item.CollectInterval,
		&item.NextCollectAt,
		&item.CollectorInstalled,
		&item.AgentPort,
		&item.CollectStatus,
		&item.CollectError,
		&item.LastCollectedAt,
		&item.AgentLastSeenAt,
		&item.CreatedAt,
		&item.UpdatedAt,
		&metricID,
		&cpuPercent,
		&cpuCores,
		&latencyMS,
		&memoryUsedBytes,
		&memoryTotalBytes,
		&swapUsedBytes,
		&swapTotalBytes,
		&diskUsedBytes,
		&diskTotalBytes,
		&networkRXBytes,
		&networkTXBytes,
		&networkRXRateBps,
		&networkTXRateBps,
		&load1,
		&load5,
		&load15,
		&tcpConnections,
		&udpConnections,
		&processCount,
		&uptimeSeconds,
		&architecture,
		&virtualization,
		&osName,
		&cpuModel,
		&gpuModel,
		&region,
		&raw,
		&collectedAt,
	)
	if err != nil {
		return Connection{}, err
	}
	if metricID.Valid {
		metric := Metric{
			ServerID:         metricID.String,
			CPUPercent:       nullFloat(cpuPercent),
			CPUCores:         nullInt(cpuCores),
			LatencyMS:        nullFloat(latencyMS),
			MemoryUsedBytes:  nullInt(memoryUsedBytes),
			MemoryTotalBytes: nullInt(memoryTotalBytes),
			SwapUsedBytes:    nullInt(swapUsedBytes),
			SwapTotalBytes:   nullInt(swapTotalBytes),
			DiskUsedBytes:    nullInt(diskUsedBytes),
			DiskTotalBytes:   nullInt(diskTotalBytes),
			NetworkRXBytes:   nullInt(networkRXBytes),
			NetworkTXBytes:   nullInt(networkTXBytes),
			NetworkRXRateBps: nullFloat(networkRXRateBps),
			NetworkTXRateBps: nullFloat(networkTXRateBps),
			Load1:            nullFloat(load1),
			Load5:            nullFloat(load5),
			Load15:           nullFloat(load15),
			TCPConnections:   nullInt(tcpConnections),
			UDPConnections:   nullInt(udpConnections),
			ProcessCount:     nullInt(processCount),
			UptimeSeconds:    nullInt(uptimeSeconds),
			Architecture:     nullString(architecture),
			Virtualization:   nullString(virtualization),
			OSName:           nullString(osName),
			CPUModel:         nullString(cpuModel),
			GPUModel:         nullString(gpuModel),
			Region:           nullString(region),
			CollectedAt:      nullTime(collectedAt),
		}
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &metric.Raw)
		}
		item.Metric = &metric
	}
	return item, nil
}

func nullFloat(value sql.NullFloat64) float64 {
	if !value.Valid {
		return 0
	}
	return value.Float64
}

func nullInt(value sql.NullInt64) int64 {
	if !value.Valid {
		return 0
	}
	return value.Int64
}

func nullString(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func nullTime(value sql.NullTime) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time
}

func IsNotFound(err error) bool {
	return err == pgx.ErrNoRows
}
