package notifications

import "time"

const DefaultTelegramSettingsID = "default"

type TelegramSettings struct {
	ID                   string    `json:"id"`
	Enabled              bool      `json:"enabled"`
	BotToken             string    `json:"-"`
	BotTokenSecretID     string    `json:"-"`
	InboundToken         string    `json:"-"`
	InboundTokenSecretID string    `json:"-"`
	GroupEnabled         bool      `json:"group_enabled"`
	GroupChatID          string    `json:"group_chat_id"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type PublicTelegramSettings struct {
	ID              string    `json:"id"`
	Enabled         bool      `json:"enabled"`
	HasBotToken     bool      `json:"has_bot_token"`
	HasInboundToken bool      `json:"has_inbound_token"`
	GroupEnabled    bool      `json:"group_enabled"`
	GroupChatID     string    `json:"group_chat_id"`
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
	GroupEnabled      bool
	GroupChatID       string
	ClearBotToken     bool
	ClearInboundToken bool
}

type UpdateUserTelegramSettingsInput struct {
	UserID  string
	Enabled bool
	ChatID  string
}

type IncomingMessageInput struct {
	UserID         string
	Username       string
	Title          string
	Message        string
	Source         string
	DeliverToGroup bool
}

type IncomingMessageResult struct {
	Delivered      bool   `json:"delivered"`
	UserDelivered  bool   `json:"user_delivered,omitempty"`
	GroupDelivered bool   `json:"group_delivered,omitempty"`
	UserID         string `json:"user_id,omitempty"`
	Username       string `json:"username,omitempty"`
}

func (s TelegramSettings) Public() PublicTelegramSettings {
	return PublicTelegramSettings{
		ID:              s.ID,
		Enabled:         s.Enabled,
		HasBotToken:     s.BotToken != "",
		HasInboundToken: s.InboundToken != "",
		GroupEnabled:    s.GroupEnabled,
		GroupChatID:     s.GroupChatID,
		UpdatedAt:       s.UpdatedAt,
	}
}
