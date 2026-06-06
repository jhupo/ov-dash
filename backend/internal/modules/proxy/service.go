package proxy

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var (
	ErrHostRequired = errors.New("proxy host is required")
	ErrInvalidPort  = errors.New("proxy port must be between 1 and 65535")
)

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Get(ctx context.Context) (Settings, error) {
	return s.repo.Get(ctx)
}

func (s *Service) Update(ctx context.Context, input UpdateSettingsInput) (Settings, error) {
	input.Host = strings.TrimSpace(input.Host)
	input.Username = strings.TrimSpace(input.Username)

	if input.Enabled && input.Host == "" {
		return Settings{}, ErrHostRequired
	}
	if input.Port < 1 || input.Port > 65535 {
		return Settings{}, ErrInvalidPort
	}

	return s.repo.Update(ctx, input)
}

func (s *Service) HTTPClient(ctx context.Context) (*http.Client, error) {
	settings, err := s.Get(ctx)
	if err != nil {
		return nil, err
	}
	return NewHTTPClient(settings)
}

func NewHTTPClient(settings Settings) (*http.Client, error) {
	transport, err := NewTransport(settings)
	if err != nil {
		return nil, err
	}

	return &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}, nil
}

func NewTransport(settings Settings) (*http.Transport, error) {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
	}

	if !settings.Enabled {
		return transport, nil
	}

	proxyURL, err := url.Parse(settings.URL())
	if err != nil {
		return nil, err
	}
	transport.Proxy = http.ProxyURL(proxyURL)
	return transport, nil
}

func (s Settings) URL() string {
	if !s.Enabled || s.Host == "" || s.Port == 0 {
		return ""
	}

	u := url.URL{
		Scheme: "socks5",
		Host:   s.hostPort(),
	}
	if s.Username != "" {
		if s.Password != "" {
			u.User = url.UserPassword(s.Username, s.Password)
		} else {
			u.User = url.User(s.Username)
		}
	}
	return u.String()
}

func (s Settings) hostPort() string {
	if strings.Contains(s.Host, ":") {
		return "[" + strings.Trim(s.Host, "[]") + "]:" + strconv.Itoa(s.Port)
	}
	return s.Host + ":" + strconv.Itoa(s.Port)
}
