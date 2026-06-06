package notifications

import "time"

const DefaultTelegramSettingsID = "default"

type TelegramSettings struct {
	ID           string    `json:"id"`
	Enabled      bool      `json:"enabled"`
	BotToken     string    `json:"-"`
	InboundToken string    `json:"-"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type PublicTelegramSettings struct {
	ID              string    `json:"id"`
	Enabled         bool      `json:"enabled"`
	HasBotToken     bool      `json:"has_bot_token"`
	HasInboundToken bool      `json:"has_inbound_token"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type UserTelegramSettings struct {
	UserID    string    `json:"user_id"`
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	FullName  string    `json:"full_name"`
	Enabled   bool      `json:"enabled"`
	ChatID    string    `json:"chat_id"`
	UpdatedAt time.Time `json:"updated_at"`
}

type UpdateTelegramSettingsInput struct {
	Enabled           bool
	BotToken          *string
	InboundToken      *string
	ClearBotToken     bool
	ClearInboundToken bool
}

type UpdateUserTelegramSettingsInput struct {
	UserID  string
	Enabled bool
	ChatID  string
}

type IncomingMessageInput struct {
	UserID   string
	Username string
	Title    string
	Message  string
	Source   string
}

type IncomingMessageResult struct {
	Delivered bool   `json:"delivered"`
	UserID    string `json:"user_id,omitempty"`
	Username  string `json:"username,omitempty"`
}

func (s TelegramSettings) Public() PublicTelegramSettings {
	return PublicTelegramSettings{
		ID:              s.ID,
		Enabled:         s.Enabled,
		HasBotToken:     s.BotToken != "",
		HasInboundToken: s.InboundToken != "",
		UpdatedAt:       s.UpdatedAt,
	}
}
