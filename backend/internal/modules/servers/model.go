package servers

import "time"

type Connection struct {
	ID                 string
	Name               string
	GroupName          string
	Region             string
	Host               string
	Port               int
	Username           string
	AuthType           string
	Password           string
	PrivateKey         string
	ExpiresAt          *time.Time
	CollectorInstalled bool
	CollectStatus      string
	CollectError       string
	LastCollectedAt    *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
	Metric             *Metric
}

type Metric struct {
	ServerID         string         `json:"server_id"`
	CPUPercent      float64        `json:"cpu_percent"`
	MemoryUsedBytes int64          `json:"memory_used_bytes"`
	MemoryTotalBytes int64         `json:"memory_total_bytes"`
	SwapUsedBytes    int64         `json:"swap_used_bytes"`
	SwapTotalBytes   int64         `json:"swap_total_bytes"`
	DiskUsedBytes    int64         `json:"disk_used_bytes"`
	DiskTotalBytes   int64         `json:"disk_total_bytes"`
	NetworkRXBytes   int64         `json:"network_rx_bytes"`
	NetworkTXBytes   int64         `json:"network_tx_bytes"`
	NetworkRXRateBps float64       `json:"network_rx_rate_bps"`
	NetworkTXRateBps float64       `json:"network_tx_rate_bps"`
	Load1            float64       `json:"load1"`
	Load5            float64       `json:"load5"`
	Load15           float64       `json:"load15"`
	UptimeSeconds    int64         `json:"uptime_seconds"`
	Architecture     string        `json:"architecture"`
	Virtualization   string        `json:"virtualization"`
	OSName           string        `json:"os_name"`
	CPUModel         string        `json:"cpu_model"`
	GPUModel         string        `json:"gpu_model"`
	Raw              map[string]any `json:"raw"`
	CollectedAt      time.Time     `json:"collected_at"`
}

type PublicConnection struct {
	ID                 string     `json:"id"`
	Name               string     `json:"name"`
	GroupName          string     `json:"group_name"`
	Region             string     `json:"region"`
	Host               string     `json:"host"`
	Port               int        `json:"port"`
	Username           string     `json:"username"`
	AuthType           string     `json:"auth_type"`
	HasPassword        bool       `json:"has_password"`
	HasPrivateKey      bool       `json:"has_private_key"`
	ConnectionHint      string     `json:"connection_hint"`
	ExpiresAt          *time.Time `json:"expires_at"`
	CollectorInstalled bool       `json:"collector_installed"`
	CollectStatus      string     `json:"collect_status"`
	CollectError       string     `json:"collect_error"`
	LastCollectedAt    *time.Time `json:"last_collected_at"`
	Metric             *Metric    `json:"metric"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

type SaveInput struct {
	ID          string
	Name        string
	GroupName   string
	Region      string
	Host        string
	Port        int
	Username    string
	AuthType    string
	Password    *string
	PrivateKey  *string
	ExpiresAt   *time.Time
	ClearSecret bool
}

func (c Connection) Public() PublicConnection {
	return PublicConnection{
		ID:                 c.ID,
		Name:               c.Name,
		GroupName:          c.GroupName,
		Region:             c.Region,
		Host:               c.Host,
		Port:               c.Port,
		Username:           c.Username,
		AuthType:           c.AuthType,
		HasPassword:        c.Password != "",
		HasPrivateKey:      c.PrivateKey != "",
		ConnectionHint:      c.Username + "@" + c.Host,
		ExpiresAt:          c.ExpiresAt,
		CollectorInstalled: c.CollectorInstalled,
		CollectStatus:      c.CollectStatus,
		CollectError:       c.CollectError,
		LastCollectedAt:    c.LastCollectedAt,
		Metric:             c.Metric,
		CreatedAt:          c.CreatedAt,
		UpdatedAt:          c.UpdatedAt,
	}
}
