package auth

import "time"

type BootstrapAdminInput struct {
	Email     string
	Password  string
	Username  string
	FirstName string
	LastName  string
}

type User struct {
	ID           string     `json:"id"`
	FirstName    string     `json:"firstName"`
	LastName     string     `json:"lastName"`
	Username     string     `json:"username"`
	Email        string     `json:"email"`
	PhoneNumber  string     `json:"phoneNumber"`
	Status       string     `json:"status"`
	Role         string     `json:"role"`
	Capabilities []string   `json:"capabilities"`
	LastLoginAt  *time.Time `json:"lastLoginAt,omitempty"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
}

type userWithPassword struct {
	User
	PasswordHash string
}

type Session struct {
	ID        string
	UserID    string
	TokenHash string
	ExpiresAt time.Time
	RevokedAt *time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}
