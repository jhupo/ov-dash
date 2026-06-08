package servers

import "time"

type Connection struct {
	ID                  string
	Name                string
	GroupName           string
	Region              string
	Host                string
	Port                int
	Username            string
	AuthType            string
	Password            string
	PrivateKey          string
	PasswordSecretID    string
	PrivateKeySecretID  string
	ExpiresAt           *time.Time
	CollectInterval     int
	NextCollectAt       time.Time
	CollectorInstalled  bool
	AgentPort           int
	CollectStatus       string
	CollectError        string
	CollectFailureCount int
	LastCollectedAt     *time.Time
	AgentLastSeenAt     *time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
	Metric              *Metric
}

type Metric struct {
	ServerID         string         `json:"server_id"`
	CPUPercent       float64        `json:"cpu_percent"`
	CPUCores         int64          `json:"cpu_cores"`
	LatencyMS        float64        `json:"latency_ms"`
	MemoryUsedBytes  int64          `json:"memory_used_bytes"`
	MemoryTotalBytes int64          `json:"memory_total_bytes"`
	SwapUsedBytes    int64          `json:"swap_used_bytes"`
	SwapTotalBytes   int64          `json:"swap_total_bytes"`
	DiskUsedBytes    int64          `json:"disk_used_bytes"`
	DiskTotalBytes   int64          `json:"disk_total_bytes"`
	NetworkRXBytes   int64          `json:"network_rx_bytes"`
	NetworkTXBytes   int64          `json:"network_tx_bytes"`
	NetworkRXRateBps float64        `json:"network_rx_rate_bps"`
	NetworkTXRateBps float64        `json:"network_tx_rate_bps"`
	Load1            float64        `json:"load1"`
	Load5            float64        `json:"load5"`
	Load15           float64        `json:"load15"`
	TCPConnections   int64          `json:"tcp_connections"`
	UDPConnections   int64          `json:"udp_connections"`
	ProcessCount     int64          `json:"process_count"`
	UptimeSeconds    int64          `json:"uptime_seconds"`
	Architecture     string         `json:"architecture"`
	Virtualization   string         `json:"virtualization"`
	OSName           string         `json:"os_name"`
	CPUModel         string         `json:"cpu_model"`
	GPUModel         string         `json:"gpu_model"`
	Region           string         `json:"region"`
	Raw              map[string]any `json:"raw"`
	CollectedAt      time.Time      `json:"collected_at"`
}

type PublicConnection struct {
	ID                  string     `json:"id"`
	Name                string     `json:"name"`
	GroupName           string     `json:"group_name"`
	Region              string     `json:"region"`
	Host                string     `json:"host"`
	Port                int        `json:"port"`
	Username            string     `json:"username"`
	AuthType            string     `json:"auth_type"`
	HasPassword         bool       `json:"has_password"`
	HasPrivateKey       bool       `json:"has_private_key"`
	ConnectionHint      string     `json:"connection_hint"`
	ExpiresAt           *time.Time `json:"expires_at"`
	CollectInterval     int        `json:"collect_interval_seconds"`
	NextCollectAt       time.Time  `json:"next_collect_at"`
	CollectorInstalled  bool       `json:"collector_installed"`
	AgentPort           int        `json:"agent_port"`
	CollectStatus       string     `json:"collect_status"`
	CollectError        string     `json:"collect_error"`
	CollectFailureCount int        `json:"collect_failure_count"`
	LastCollectedAt     *time.Time `json:"last_collected_at"`
	AgentLastSeenAt     *time.Time `json:"agent_last_seen_at"`
	Metric              *Metric    `json:"metric"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

type SaveInput struct {
	ID              string
	Name            string
	GroupName       string
	Region          string
	Host            string
	Port            int
	Username        string
	AuthType        string
	Password        *string
	PrivateKey      *string
	ExpiresAt       *time.Time
	CollectInterval int
	ClearSecret     bool
}

func (c Connection) Public() PublicConnection {
	region := c.Region
	if c.Metric != nil && c.Metric.Region != "" {
		region = c.Metric.Region
	}
	return PublicConnection{
		ID:                  c.ID,
		Name:                c.Name,
		GroupName:           c.GroupName,
		Region:              region,
		Host:                c.Host,
		Port:                c.Port,
		Username:            c.Username,
		AuthType:            c.AuthType,
		HasPassword:         c.Password != "" || c.PasswordSecretID != "",
		HasPrivateKey:       c.PrivateKey != "" || c.PrivateKeySecretID != "",
		ConnectionHint:      c.ConnectionHint(),
		ExpiresAt:           c.ExpiresAt,
		CollectInterval:     c.CollectInterval,
		NextCollectAt:       c.NextCollectAt,
		CollectorInstalled:  c.CollectorInstalled,
		AgentPort:           c.AgentPort,
		CollectStatus:       c.CollectStatus,
		CollectError:        c.CollectError,
		CollectFailureCount: c.CollectFailureCount,
		LastCollectedAt:     c.LastCollectedAt,
		AgentLastSeenAt:     c.AgentLastSeenAt,
		Metric:              c.Metric,
		CreatedAt:           c.CreatedAt,
		UpdatedAt:           c.UpdatedAt,
	}
}

func (c Connection) ConnectionHint() string {
	return c.Username + "@" + c.Host
}
