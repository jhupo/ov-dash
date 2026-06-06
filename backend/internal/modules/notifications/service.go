package notifications

import (
	"context"
	"errors"
	"html"
	"strings"
)

var (
	ErrTelegramBotTokenRequired = errors.New("telegram bot token is required")
	ErrInboundTokenRequired     = errors.New("inbound token is required")
	ErrInvalidInboundToken      = errors.New("invalid inbound token")
	ErrRecipientRequired        = errors.New("user_id or username is required")
	ErrRecipientNotFound        = errors.New("notification recipient not found")
	ErrRecipientDisabled        = errors.New("recipient telegram notifications are disabled")
	ErrTelegramChatIDRequired   = errors.New("telegram chat id is required")
	ErrTelegramGroupDisabled    = errors.New("telegram group notifications are disabled")
	ErrTelegramGroupIDRequired  = errors.New("telegram group chat id is required")
	ErrMessageRequired          = errors.New("message is required")
	ErrNotificationsDisabled    = errors.New("telegram notifications are disabled")
)

type Sender interface {
	SendMessage(ctx context.Context, botToken string, chatID string, text string) error
}

type Service struct {
	repo   *Repository
	sender Sender
}

func NewService(repo *Repository, sender Sender) *Service {
	if sender == nil {
		sender = NewTelegramClient(nil)
	}
	return &Service{repo: repo, sender: sender}
}

func (s *Service) GetTelegramSettings(ctx context.Context) (TelegramSettings, error) {
	return s.repo.GetTelegramSettings(ctx)
}

func (s *Service) UpdateTelegramSettings(ctx context.Context, input UpdateTelegramSettingsInput) (TelegramSettings, error) {
	if input.BotToken != nil {
		value := strings.TrimSpace(*input.BotToken)
		input.BotToken = &value
	}
	if input.InboundToken != nil {
		value := strings.TrimSpace(*input.InboundToken)
		input.InboundToken = &value
	}
	input.GroupChatID = strings.TrimSpace(input.GroupChatID)

	nextBotToken := ""
	nextInboundToken := ""
	current, err := s.repo.GetTelegramSettings(ctx)
	if err != nil {
		return TelegramSettings{}, err
	}
	nextBotToken = current.BotToken
	nextInboundToken = current.InboundToken
	if input.ClearBotToken {
		nextBotToken = ""
	} else if input.BotToken != nil {
		nextBotToken = *input.BotToken
	}
	if input.ClearInboundToken {
		nextInboundToken = ""
	} else if input.InboundToken != nil {
		nextInboundToken = *input.InboundToken
	}

	if input.Enabled && nextBotToken == "" {
		return TelegramSettings{}, ErrTelegramBotTokenRequired
	}
	if input.Enabled && nextInboundToken == "" {
		return TelegramSettings{}, ErrInboundTokenRequired
	}
	if input.GroupEnabled && input.GroupChatID == "" {
		return TelegramSettings{}, ErrTelegramGroupIDRequired
	}

	return s.repo.UpdateTelegramSettings(ctx, input)
}

func (s *Service) ListUserTelegramSettings(ctx context.Context) ([]UserTelegramSettings, error) {
	return s.repo.ListUserTelegramSettings(ctx)
}

func (s *Service) UpdateUserTelegramSettings(ctx context.Context, input UpdateUserTelegramSettingsInput) (UserTelegramSettings, error) {
	input.UserID = strings.TrimSpace(input.UserID)
	input.ChatID = strings.TrimSpace(input.ChatID)
	if input.UserID == "" {
		return UserTelegramSettings{}, ErrRecipientRequired
	}
	if input.Enabled && input.ChatID == "" {
		return UserTelegramSettings{}, ErrTelegramChatIDRequired
	}
	return s.repo.UpdateUserTelegramSettings(ctx, input)
}

func (s *Service) DeliverIncomingMessage(ctx context.Context, inboundToken string, input IncomingMessageInput) (IncomingMessageResult, error) {
	settings, err := s.repo.GetTelegramSettings(ctx)
	if err != nil {
		return IncomingMessageResult{}, err
	}
	if !settings.Enabled {
		return IncomingMessageResult{}, ErrNotificationsDisabled
	}
	if settings.BotToken == "" {
		return IncomingMessageResult{}, ErrTelegramBotTokenRequired
	}
	if settings.InboundToken == "" {
		return IncomingMessageResult{}, ErrInboundTokenRequired
	}
	if strings.TrimSpace(inboundToken) != settings.InboundToken {
		return IncomingMessageResult{}, ErrInvalidInboundToken
	}

	input.UserID = strings.TrimSpace(input.UserID)
	input.Username = strings.TrimSpace(input.Username)
	input.Title = strings.TrimSpace(input.Title)
	input.Message = strings.TrimSpace(input.Message)
	input.Source = strings.TrimSpace(input.Source)
	if input.Message == "" {
		return IncomingMessageResult{}, ErrMessageRequired
	}

	sendToUser := input.UserID != "" || input.Username != ""
	sendToGroup := input.DeliverToGroup || !sendToUser
	if !sendToUser && !sendToGroup {
		return IncomingMessageResult{}, ErrRecipientRequired
	}
	if sendToGroup {
		if !settings.GroupEnabled {
			return IncomingMessageResult{}, ErrTelegramGroupDisabled
		}
		if settings.GroupChatID == "" {
			return IncomingMessageResult{}, ErrTelegramGroupIDRequired
		}
	}

	result := IncomingMessageResult{}
	text := formatTelegramMessage(input)

	if sendToUser {
		recipient, err := s.repo.FindRecipient(ctx, input.UserID, input.Username)
		if IsNotFound(err) {
			return IncomingMessageResult{}, ErrRecipientNotFound
		}
		if err != nil {
			return IncomingMessageResult{}, err
		}
		if !recipient.Enabled {
			return IncomingMessageResult{}, ErrRecipientDisabled
		}
		if recipient.ChatID == "" {
			return IncomingMessageResult{}, ErrTelegramChatIDRequired
		}
		if err := s.sender.SendMessage(ctx, settings.BotToken, recipient.ChatID, text); err != nil {
			return IncomingMessageResult{}, err
		}
		result.UserDelivered = true
		result.UserID = recipient.UserID
		result.Username = recipient.Username
	}

	if sendToGroup {
		if err := s.sender.SendMessage(ctx, settings.BotToken, settings.GroupChatID, text); err != nil {
			return IncomingMessageResult{}, err
		}
		result.GroupDelivered = true
	}

	result.Delivered = result.UserDelivered || result.GroupDelivered
	return result, nil
}

func formatTelegramMessage(input IncomingMessageInput) string {
	parts := make([]string, 0, 3)
	if input.Title != "" {
		parts = append(parts, "<b>"+html.EscapeString(input.Title)+"</b>")
	}
	if input.Source != "" {
		parts = append(parts, "<i>"+html.EscapeString(input.Source)+"</i>")
	}
	parts = append(parts, html.EscapeString(input.Message))
	return strings.Join(parts, "\n\n")
}
