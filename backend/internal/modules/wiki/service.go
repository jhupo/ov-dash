package wiki

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
)

var (
	ErrTitleRequired = errors.New("wiki page title is required")
	ErrIDRequired    = errors.New("wiki page id is required")
)

type Service struct {
	repository *Repository
}

func NewService(repository *Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) List(ctx context.Context) ([]Page, error) {
	return s.repository.List(ctx)
}

func (s *Service) Get(ctx context.Context, id string) (Page, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Page{}, ErrIDRequired
	}
	return s.repository.Get(ctx, id)
}

func (s *Service) Create(ctx context.Context, input SavePageInput) (Page, error) {
	var err error
	input, err = normalizeInput(input)
	if err != nil {
		return Page{}, err
	}
	input.ID, err = randomID()
	if err != nil {
		return Page{}, err
	}
	return s.repository.Create(ctx, input)
}

func (s *Service) Update(ctx context.Context, input SavePageInput) (Page, error) {
	var err error
	input.ID = strings.TrimSpace(input.ID)
	if input.ID == "" {
		return Page{}, ErrIDRequired
	}
	input, err = normalizeInput(input)
	if err != nil {
		return Page{}, err
	}
	return s.repository.Update(ctx, input)
}

func (s *Service) Delete(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return ErrIDRequired
	}
	return s.repository.Delete(ctx, id)
}

func (s *Service) ListRevisions(ctx context.Context, pageID string) ([]Revision, error) {
	pageID = strings.TrimSpace(pageID)
	if pageID == "" {
		return nil, ErrIDRequired
	}
	return s.repository.ListRevisions(ctx, pageID)
}

func normalizeInput(input SavePageInput) (SavePageInput, error) {
	input.ParentID = strings.TrimSpace(input.ParentID)
	input.Title = strings.TrimSpace(input.Title)
	input.PageType = strings.TrimSpace(input.PageType)
	input.Category = strings.TrimSpace(input.Category)
	input.Summary = strings.TrimSpace(input.Summary)
	input.LinkLabel = strings.TrimSpace(input.LinkLabel)
	input.LinkURL = strings.TrimSpace(input.LinkURL)
	input.MachineHost = strings.TrimSpace(input.MachineHost)
	input.MachinePort = strings.TrimSpace(input.MachinePort)
	input.MachineUsername = strings.TrimSpace(input.MachineUsername)
	input.Tags = strings.TrimSpace(input.Tags)
	input.ActorID = strings.TrimSpace(input.ActorID)

	if input.Title == "" {
		return SavePageInput{}, ErrTitleRequired
	}
	if input.PageType == "" {
		input.PageType = PageTypeDocument
	}
	if !isSupportedPageType(input.PageType) {
		input.PageType = PageTypeDocument
	}
	if input.Category == "" {
		input.Category = "资料库"
	}
	return input, nil
}

func isSupportedPageType(pageType string) bool {
	switch pageType {
	case PageTypeDocument,
		PageTypeMachine,
		PageTypeLink,
		PageTypeRunbook,
		PageTypeTroubleshooting:
		return true
	default:
		return false
	}
}

func randomID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
