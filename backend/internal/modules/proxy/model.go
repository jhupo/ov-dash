package proxy

import "time"

const DefaultSettingsID = "default"

type Settings struct {
	ID        string    `json:"id"`
	Enabled   bool      `json:"enabled"`
	Scheme    string    `json:"scheme"`
	Host      string    `json:"host"`
	Port      int       `json:"port"`
	Username  string    `json:"username"`
	Password  string    `json:"-"`
	UpdatedAt time.Time `json:"updated_at"`
}

type PublicSettings struct {
	ID          string    `json:"id"`
	Enabled     bool      `json:"enabled"`
	Scheme      string    `json:"scheme"`
	Host        string    `json:"host"`
	Port        int       `json:"port"`
	Username    string    `json:"username"`
	HasPassword bool      `json:"has_password"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type UpdateSettingsInput struct {
	Enabled       bool
	Host          string
	Port          int
	Username      string
	Password      *string
	ClearPassword bool
}

func (s Settings) Public() PublicSettings {
	return PublicSettings{
		ID:          s.ID,
		Enabled:     s.Enabled,
		Scheme:      s.Scheme,
		Host:        s.Host,
		Port:        s.Port,
		Username:    s.Username,
		HasPassword: s.Password != "",
		UpdatedAt:   s.UpdatedAt,
	}
}
