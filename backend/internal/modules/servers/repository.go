package servers

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"ov-dash/backend/internal/db"
	"ov-dash/backend/internal/platform/secret"

	"github.com/jackc/pgx/v5"
)

type Repository struct {
	db          *db.Pool
	credentials *CredentialResolver
	metrics     *MetricsStore
}

func NewRepository(db *db.Pool) *Repository {
	return NewRepositoryWithCredentials(db, nil)
}

func NewRepositoryWithSecrets(db *db.Pool, secrets *secret.Store) *Repository {
	return NewRepositoryWithCredentials(db, NewCredentialResolver(secrets))
}

func NewRepositoryWithCredentials(db *db.Pool, credentials *CredentialResolver) *Repository {
	return &Repository{
		db:          db,
		credentials: credentials,
		metrics:     NewMetricsStore(db),
	}
}

func (r *Repository) credentialResolver() *CredentialResolver {
	if r.credentials != nil {
		return r.credentials
	}
	return NewCredentialResolver(nil)
}

func (r *Repository) metricStore() *MetricsStore {
	if r.metrics != nil {
		return r.metrics
	}
	return NewMetricsStore(r.db)
}

func (r *Repository) List(ctx context.Context) ([]Connection, error) {
	rows, err := r.db.Query(ctx, `
		SELECT
			c.id, c.name, c.group_name, c.region, c.host, c.port, c.username, c.auth_type,
			c.password, c.private_key, c.password_secret_id, c.private_key_secret_id,
			c.expires_at, c.collect_interval_seconds, c.next_collect_at, c.collector_installed, c.agent_port,
			c.collect_status, c.collect_error, c.collect_failure_count, c.last_collected_at, c.agent_last_seen_at, c.created_at, c.updated_at,
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
		item = r.credentialResolver().Resolve(ctx, item)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) Get(ctx context.Context, id string) (Connection, error) {
	row := r.db.QueryRow(ctx, `
		SELECT
			c.id, c.name, c.group_name, c.region, c.host, c.port, c.username, c.auth_type,
			c.password, c.private_key, c.password_secret_id, c.private_key_secret_id,
			c.expires_at, c.collect_interval_seconds, c.next_collect_at, c.collector_installed, c.agent_port,
			c.collect_status, c.collect_error, c.collect_failure_count, c.last_collected_at, c.agent_last_seen_at, c.created_at, c.updated_at,
			m.server_id, m.cpu_percent, m.cpu_cores, m.latency_ms, m.memory_used_bytes, m.memory_total_bytes,
			m.swap_used_bytes, m.swap_total_bytes, m.disk_used_bytes, m.disk_total_bytes,
			m.network_rx_bytes, m.network_tx_bytes, m.network_rx_rate_bps, m.network_tx_rate_bps,
			m.load1, m.load5, m.load15, m.tcp_connections, m.udp_connections, m.process_count, m.uptime_seconds, m.architecture, m.virtualization,
			m.os_name, m.cpu_model, m.gpu_model, m.region, m.raw, m.collected_at
		FROM server_connections c
		LEFT JOIN server_metrics m ON m.server_id = c.id
		WHERE c.id = $1
	`, id)
	item, err := scanConnection(row)
	if err != nil {
		return Connection{}, err
	}
	return r.credentialResolver().Resolve(ctx, item), nil
}

func (r *Repository) Upsert(ctx context.Context, input SaveInput) (Connection, error) {
	credentials, err := r.credentialResolver().PrepareSave(ctx, input)
	if err != nil {
		return Connection{}, err
	}

	var item Connection
	err = r.db.QueryRow(ctx, `
		INSERT INTO server_connections (
			id, name, group_name, region, host, port, username, auth_type,
			password, private_key, password_secret_id, private_key_secret_id,
			expires_at, collect_interval_seconds, next_collect_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, COALESCE($9, ''), COALESCE($10, ''), $11, $12, $13, $15, now())
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
				WHEN $14 THEN ''
				WHEN $9::text IS NULL THEN server_connections.password
				ELSE EXCLUDED.password
			END,
			private_key = CASE
				WHEN $14 THEN ''
				WHEN $10::text IS NULL THEN server_connections.private_key
				ELSE EXCLUDED.private_key
			END,
			password_secret_id = CASE
				WHEN $14 THEN ''
				WHEN NOT $16::boolean THEN server_connections.password_secret_id
				ELSE EXCLUDED.password_secret_id
			END,
			private_key_secret_id = CASE
				WHEN $14 THEN ''
				WHEN NOT $17::boolean THEN server_connections.private_key_secret_id
				ELSE EXCLUDED.private_key_secret_id
			END,
			collect_status = 'pending',
			collect_error = '',
			collect_failure_count = 0,
			updated_at = now()
		RETURNING id, name, group_name, region, host, port, username, auth_type,
		          password, private_key, password_secret_id, private_key_secret_id,
		          expires_at, collect_interval_seconds, next_collect_at, collector_installed, agent_port,
		          collect_status, collect_error, collect_failure_count, last_collected_at, agent_last_seen_at, created_at, updated_at
	`,
		input.ID,
		input.Name,
		input.GroupName,
		input.Region,
		input.Host,
		input.Port,
		input.Username,
		input.AuthType,
		credentials.Password,
		credentials.PrivateKey,
		credentials.PasswordSecretID,
		credentials.PrivateKeySecretID,
		input.ExpiresAt,
		input.ClearSecret,
		input.CollectInterval,
		input.Password != nil,
		input.PrivateKey != nil,
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
		&item.PasswordSecretID,
		&item.PrivateKeySecretID,
		&item.ExpiresAt,
		&item.CollectInterval,
		&item.NextCollectAt,
		&item.CollectorInstalled,
		&item.AgentPort,
		&item.CollectStatus,
		&item.CollectError,
		&item.CollectFailureCount,
		&item.LastCollectedAt,
		&item.AgentLastSeenAt,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		return Connection{}, err
	}
	return r.credentialResolver().Resolve(ctx, item), nil
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
			c.password, c.private_key, c.password_secret_id, c.private_key_secret_id,
			c.expires_at, c.collect_interval_seconds, c.next_collect_at,
			c.collector_installed, c.agent_port, c.collect_status, c.collect_error, c.collect_failure_count, c.last_collected_at,
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
		item = r.credentialResolver().Resolve(ctx, item)
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
		    collect_failure_count = collect_failure_count + 1,
		    next_collect_at = now() + make_interval(secs => LEAST(3600, GREATEST(10, collect_interval_seconds) * power(2, LEAST(8, collect_failure_count))::int)),
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
	return r.metricStore().Samples(ctx, id, since)
}

func (r *Repository) Delete(ctx context.Context, id string) error {
	item, _ := r.Get(ctx, id)
	_, err := r.db.Exec(ctx, `DELETE FROM server_connections WHERE id = $1`, id)
	if err == nil {
		_ = r.credentialResolver().Delete(ctx, item)
	}
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
		    collect_failure_count = collect_failure_count + 1,
		    next_collect_at = now() + make_interval(secs => LEAST(3600, GREATEST(10, collect_interval_seconds) * power(2, LEAST(8, collect_failure_count))::int)),
		    updated_at = now()
		WHERE id = $1
	`, id, message)
	return err
}

func (r *Repository) SaveMetric(ctx context.Context, metric Metric) error {
	return r.metricStore().Save(ctx, metric, true)
}

func (r *Repository) SaveAgentMetric(ctx context.Context, metric Metric) error {
	return r.metricStore().Save(ctx, metric, false)
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
		&item.PasswordSecretID,
		&item.PrivateKeySecretID,
		&item.ExpiresAt,
		&item.CollectInterval,
		&item.NextCollectAt,
		&item.CollectorInstalled,
		&item.AgentPort,
		&item.CollectStatus,
		&item.CollectError,
		&item.CollectFailureCount,
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

func collectionRetryDelaySeconds(intervalSeconds int, failureCount int) int {
	if intervalSeconds < 10 {
		intervalSeconds = 10
	}
	if failureCount < 1 {
		failureCount = 1
	}
	if failureCount > 9 {
		failureCount = 9
	}
	delay := intervalSeconds
	for i := 1; i < failureCount; i++ {
		delay *= 2
		if delay >= 3600 {
			return 3600
		}
	}
	if delay > 3600 {
		return 3600
	}
	return delay
}

func IsNotFound(err error) bool {
	return err == pgx.ErrNoRows
}
