package servers

import "time"

type Connection struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	GroupName  string    `json:"group_name"`
	Region     string    `json:"region"`
	Host       string    `json:"host"`
	Port       int       `json:"port"`
	Username   string    `json:"username"`
	AuthType   string    `json:"auth_type"`
	Password   string    `json:"-"`
	PrivateKey string    `json:"-"`
	Passphrase string    `json:"-"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type PublicConnection struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	GroupName      string    `json:"group_name"`
	Region         string    `json:"region"`
	Host           string    `json:"host"`
	Port           int       `json:"port"`
	Username       string    `json:"username"`
	AuthType       string    `json:"auth_type"`
	HasPassword    bool      `json:"has_password"`
	HasPrivateKey  bool      `json:"has_private_key"`
	HasPassphrase  bool      `json:"has_passphrase"`
	ConnectionHint string    `json:"connection_hint"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
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
	Passphrase  *string
	ClearSecret bool
}

func (c Connection) Public() PublicConnection {
	return PublicConnection{
		ID:             c.ID,
		Name:           c.Name,
		GroupName:      c.GroupName,
		Region:         c.Region,
		Host:           c.Host,
		Port:           c.Port,
		Username:       c.Username,
		AuthType:       c.AuthType,
		HasPassword:    c.Password != "",
		HasPrivateKey:  c.PrivateKey != "",
		HasPassphrase:  c.Passphrase != "",
		ConnectionHint: c.Username + "@" + c.Host,
		CreatedAt:      c.CreatedAt,
		UpdatedAt:      c.UpdatedAt,
	}
}
