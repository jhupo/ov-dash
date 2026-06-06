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
			c.password, c.private_key, c.expires_at, c.collector_installed,
			c.collect_status, c.collect_error, c.last_collected_at, c.created_at, c.updated_at,
			m.server_id, m.cpu_percent, m.memory_used_bytes, m.memory_total_bytes,
			m.swap_used_bytes, m.swap_total_bytes, m.disk_used_bytes, m.disk_total_bytes,
			m.network_rx_bytes, m.network_tx_bytes, m.network_rx_rate_bps, m.network_tx_rate_bps,
			m.load1, m.load5, m.load15, m.uptime_seconds, m.architecture, m.virtualization,
			m.os_name, m.cpu_model, m.gpu_model, m.raw, m.collected_at
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
			c.password, c.private_key, c.expires_at, c.collector_installed,
			c.collect_status, c.collect_error, c.last_collected_at, c.created_at, c.updated_at,
			m.server_id, m.cpu_percent, m.memory_used_bytes, m.memory_total_bytes,
			m.swap_used_bytes, m.swap_total_bytes, m.disk_used_bytes, m.disk_total_bytes,
			m.network_rx_bytes, m.network_tx_bytes, m.network_rx_rate_bps, m.network_tx_rate_bps,
			m.load1, m.load5, m.load15, m.uptime_seconds, m.architecture, m.virtualization,
			m.os_name, m.cpu_model, m.gpu_model, m.raw, m.collected_at
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
			password, private_key, expires_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, COALESCE($9, ''), COALESCE($10, ''), $11)
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			group_name = EXCLUDED.group_name,
			region = EXCLUDED.region,
			host = EXCLUDED.host,
			port = EXCLUDED.port,
			username = EXCLUDED.username,
			auth_type = EXCLUDED.auth_type,
			expires_at = EXCLUDED.expires_at,
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
		          password, private_key, expires_at, collector_installed,
		          collect_status, collect_error, last_collected_at, created_at, updated_at
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
		&item.CollectorInstalled,
		&item.CollectStatus,
		&item.CollectError,
		&item.LastCollectedAt,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	return item, err
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
		SET collect_status = 'error', collect_error = $2, updated_at = now()
		WHERE id = $1
	`, id, message)
	return err
}

func (r *Repository) SaveMetric(ctx context.Context, metric Metric) error {
	raw, err := json.Marshal(metric.Raw)
	if err != nil {
		return err
	}

	_, err = r.db.Exec(ctx, `
		INSERT INTO server_metrics (
			server_id, cpu_percent, memory_used_bytes, memory_total_bytes,
			swap_used_bytes, swap_total_bytes, disk_used_bytes, disk_total_bytes,
			network_rx_bytes, network_tx_bytes, network_rx_rate_bps, network_tx_rate_bps,
			load1, load5, load15, uptime_seconds, architecture, virtualization,
			os_name, cpu_model, gpu_model, raw, collected_at
		)
		VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12,
			$13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23
		)
		ON CONFLICT (server_id) DO UPDATE SET
			cpu_percent = EXCLUDED.cpu_percent,
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
			uptime_seconds = EXCLUDED.uptime_seconds,
			architecture = EXCLUDED.architecture,
			virtualization = EXCLUDED.virtualization,
			os_name = EXCLUDED.os_name,
			cpu_model = EXCLUDED.cpu_model,
			gpu_model = EXCLUDED.gpu_model,
			raw = EXCLUDED.raw,
			collected_at = EXCLUDED.collected_at
	`, metric.ServerID, metric.CPUPercent, metric.MemoryUsedBytes, metric.MemoryTotalBytes,
		metric.SwapUsedBytes, metric.SwapTotalBytes, metric.DiskUsedBytes, metric.DiskTotalBytes,
		metric.NetworkRXBytes, metric.NetworkTXBytes, metric.NetworkRXRateBps, metric.NetworkTXRateBps,
		metric.Load1, metric.Load5, metric.Load15, metric.UptimeSeconds, metric.Architecture,
		metric.Virtualization, metric.OSName, metric.CPUModel, metric.GPUModel, raw, metric.CollectedAt)
	if err != nil {
		return err
	}

	_, err = r.db.Exec(ctx, `
		UPDATE server_connections
		SET collector_installed = true,
		    collect_status = 'ok',
		    collect_error = '',
		    last_collected_at = $2,
		    updated_at = now()
		WHERE id = $1
	`, metric.ServerID, metric.CollectedAt)
	return err
}

type connectionScanner interface {
	Scan(dest ...any) error
}

func scanConnection(row connectionScanner) (Connection, error) {
	var item Connection
	var metricID sql.NullString
	var cpuPercent sql.NullFloat64
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
	var uptimeSeconds sql.NullInt64
	var architecture sql.NullString
	var virtualization sql.NullString
	var osName sql.NullString
	var cpuModel sql.NullString
	var gpuModel sql.NullString
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
		&item.CollectorInstalled,
		&item.CollectStatus,
		&item.CollectError,
		&item.LastCollectedAt,
		&item.CreatedAt,
		&item.UpdatedAt,
		&metricID,
		&cpuPercent,
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
		&uptimeSeconds,
		&architecture,
		&virtualization,
		&osName,
		&cpuModel,
		&gpuModel,
		&raw,
		&collectedAt,
	)
	if err != nil {
		return Connection{}, err
	}
	if metricID.Valid {
		metric := Metric{
			ServerID:         metricID.String,
			CPUPercent:      nullFloat(cpuPercent),
			MemoryUsedBytes: nullInt(memoryUsedBytes),
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
			UptimeSeconds:    nullInt(uptimeSeconds),
			Architecture:     nullString(architecture),
			Virtualization:   nullString(virtualization),
			OSName:           nullString(osName),
			CPUModel:         nullString(cpuModel),
			GPUModel:         nullString(gpuModel),
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
