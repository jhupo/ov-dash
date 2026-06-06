package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type TelegramClient struct {
	httpClient         *http.Client
	httpClientProvider HTTPClientProvider
}

type HTTPClientProvider interface {
	HTTPClient(ctx context.Context) (*http.Client, error)
}

func NewTelegramClient(httpClient *http.Client) *TelegramClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &TelegramClient{httpClient: httpClient}
}

func NewProxiedTelegramClient(httpClientProvider HTTPClientProvider) *TelegramClient {
	return &TelegramClient{httpClientProvider: httpClientProvider}
}

type telegramSendMessageRequest struct {
	ChatID                string `json:"chat_id"`
	Text                  string `json:"text"`
	ParseMode             string `json:"parse_mode,omitempty"`
	DisableWebPagePreview bool   `json:"disable_web_page_preview"`
}

func (c *TelegramClient) SendMessage(ctx context.Context, botToken string, chatID string, text string) error {
	botToken = strings.TrimSpace(botToken)
	chatID = strings.TrimSpace(chatID)
	text = strings.TrimSpace(text)
	if botToken == "" {
		return ErrTelegramBotTokenRequired
	}
	if chatID == "" {
		return ErrTelegramChatIDRequired
	}
	if text == "" {
		return ErrMessageRequired
	}

	endpoint := "https://api.telegram.org/bot" + botToken + "/sendMessage"
	body, err := json.Marshal(telegramSendMessageRequest{
		ChatID:                chatID,
		Text:                  text,
		ParseMode:             "HTML",
		DisableWebPagePreview: true,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	httpClient, err := c.client(ctx)
	if err != nil {
		return err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return fmt.Errorf("telegram sendMessage failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(respBody)))
}

func (c *TelegramClient) client(ctx context.Context) (*http.Client, error) {
	if c.httpClientProvider != nil {
		return c.httpClientProvider.HTTPClient(ctx)
	}
	if c.httpClient != nil {
		return c.httpClient, nil
	}
	return &http.Client{Timeout: 30 * time.Second}, nil
}
