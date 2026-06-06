package servers

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"
)

var (
	ErrNameRequired     = errors.New("server name is required")
	ErrHostRequired     = errors.New("server host is required")
	ErrUsernameRequired = errors.New("server username is required")
	ErrInvalidPort      = errors.New("server port must be between 1 and 65535")
	ErrInvalidAuthType  = errors.New("server auth type must be password or key")
)

type Service struct {
	repository *Repository
}

func NewService(repository *Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) List(ctx context.Context) ([]PublicConnection, error) {
	items, err := s.repository.List(ctx)
	if err != nil {
		return nil, err
	}

	public := make([]PublicConnection, 0, len(items))
	for _, item := range items {
		public = append(public, item.Public())
	}
	return public, nil
}

func (s *Service) Save(ctx context.Context, input SaveInput) (PublicConnection, error) {
	input.ID = strings.TrimSpace(input.ID)
	input.Name = strings.TrimSpace(input.Name)
	input.GroupName = strings.TrimSpace(input.GroupName)
	input.Region = strings.TrimSpace(input.Region)
	input.Host = strings.TrimSpace(input.Host)
	input.Username = strings.TrimSpace(input.Username)
	input.AuthType = strings.TrimSpace(input.AuthType)

	if input.ID == "" {
		input.ID = "srv_" + strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	if input.Port == 0 {
		input.Port = 22
	}
	if input.AuthType == "" {
		input.AuthType = "password"
	}
	if input.CollectInterval == 0 {
		input.CollectInterval = 60
	}
	if input.Name == "" {
		return PublicConnection{}, ErrNameRequired
	}
	if input.Host == "" {
		return PublicConnection{}, ErrHostRequired
	}
	if input.Username == "" {
		return PublicConnection{}, ErrUsernameRequired
	}
	if input.Port < 1 || input.Port > 65535 {
		return PublicConnection{}, ErrInvalidPort
	}
	if input.AuthType != "password" && input.AuthType != "key" {
		return PublicConnection{}, ErrInvalidAuthType
	}
	if input.CollectInterval < 10 {
		input.CollectInterval = 10
	}

	item, err := s.repository.Upsert(ctx, input)
	if err != nil {
		return PublicConnection{}, err
	}
	return item.Public(), nil
}

func (s *Service) Delete(ctx context.Context, id string) error {
	return s.repository.Delete(ctx, strings.TrimSpace(id))
}

func (s *Service) Samples(ctx context.Context, id string, since time.Time) ([]Metric, error) {
	return s.repository.Samples(ctx, strings.TrimSpace(id), since)
}
