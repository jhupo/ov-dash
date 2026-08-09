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

type InventoryService struct {
	repository *Repository
}

func NewInventoryService(repository *Repository) *InventoryService {
	return &InventoryService{repository: repository}
}

func (s *InventoryService) List(ctx context.Context) ([]PublicConnection, error) {
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

func (s *InventoryService) Get(ctx context.Context, id string) (Connection, error) {
	return s.repository.Get(ctx, strings.TrimSpace(id))
}

func (s *InventoryService) Save(ctx context.Context, input SaveInput) (PublicConnection, error) {
	input, err := normalizeSaveInput(input)
	if err != nil {
		return PublicConnection{}, err
	}

	item, err := s.repository.Upsert(ctx, input)
	if err != nil {
		return PublicConnection{}, err
	}
	return item.Public(), nil
}

func (s *InventoryService) Delete(ctx context.Context, id string) error {
	return s.repository.Delete(ctx, strings.TrimSpace(id))
}

func normalizeSaveInput(input SaveInput) (SaveInput, error) {
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
	if input.Name == "" {
		return SaveInput{}, ErrNameRequired
	}
	if input.Host == "" {
		return SaveInput{}, ErrHostRequired
	}
	if input.Username == "" {
		return SaveInput{}, ErrUsernameRequired
	}
	if input.Port < 1 || input.Port > 65535 {
		return SaveInput{}, ErrInvalidPort
	}
	if input.AuthType != "password" && input.AuthType != "key" {
		return SaveInput{}, ErrInvalidAuthType
	}
	return input, nil
}
