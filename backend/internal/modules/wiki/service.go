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
	ErrFileRequired  = errors.New("wiki attachment file is required")
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

func (s *Service) CreateAttachment(ctx context.Context, input SaveAttachmentInput) (Attachment, error) {
	var err error
	input.ID = strings.TrimSpace(input.ID)
	input.PageID = strings.TrimSpace(input.PageID)
	input.OriginalName = strings.TrimSpace(input.OriginalName)
	input.StoragePath = strings.TrimSpace(input.StoragePath)
	input.ContentType = strings.TrimSpace(input.ContentType)
	input.ActorID = strings.TrimSpace(input.ActorID)

	if input.ID == "" {
		input.ID, err = randomID()
		if err != nil {
			return Attachment{}, err
		}
	}
	if input.StoragePath == "" {
		return Attachment{}, ErrFileRequired
	}

	return s.repository.CreateAttachment(ctx, input)
}

func (s *Service) GetAttachment(ctx context.Context, id string) (Attachment, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Attachment{}, ErrIDRequired
	}
	return s.repository.GetAttachment(ctx, id)
}

func normalizeInput(input SavePageInput) (SavePageInput, error) {
	input.ParentID = strings.TrimSpace(input.ParentID)
	input.Title = strings.TrimSpace(input.Title)
	input.PageType = strings.TrimSpace(input.PageType)
	input.Category = strings.TrimSpace(input.Category)
	input.Summary = strings.TrimSpace(input.Summary)
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

	resources := make([]SaveResourceInput, 0, len(input.Resources))
	for index, resource := range input.Resources {
		resource.ID = strings.TrimSpace(resource.ID)
		resource.ResourceType = strings.TrimSpace(resource.ResourceType)
		resource.Title = strings.TrimSpace(resource.Title)
		resource.Host = strings.TrimSpace(resource.Host)
		resource.Port = strings.TrimSpace(resource.Port)
		resource.URL = strings.TrimSpace(resource.URL)
		resource.Username = strings.TrimSpace(resource.Username)
		resource.Password = strings.TrimSpace(resource.Password)
		resource.Note = strings.TrimSpace(resource.Note)
		if !isSupportedResourceType(resource.ResourceType) {
			resource.ResourceType = ResourceTypeNote
		}
		if emptyResource(resource) {
			continue
		}
		if resource.Title == "" {
			resource.Title = defaultResourceTitle(resource)
		}
		if resource.SortOrder == 0 {
			resource.SortOrder = index + 1
		}
		resources = append(resources, resource)
	}
	input.Resources = resources

	return input, nil
}

func emptyResource(resource SaveResourceInput) bool {
	return resource.Title == "" &&
		resource.Host == "" &&
		resource.Port == "" &&
		resource.URL == "" &&
		resource.Username == "" &&
		resource.Password == "" &&
		resource.Note == ""
}

func defaultResourceTitle(resource SaveResourceInput) string {
	switch resource.ResourceType {
	case ResourceTypeMachine:
		if resource.Host != "" {
			return resource.Host
		}
		return "机器信息"
	case ResourceTypeLink:
		if resource.URL != "" {
			return resource.URL
		}
		return "链接"
	case ResourceTypeCredential:
		if resource.Username != "" {
			return resource.Username
		}
		return "账号密码"
	default:
		return "备注"
	}
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

func isSupportedResourceType(resourceType string) bool {
	switch resourceType {
	case ResourceTypeMachine,
		ResourceTypeLink,
		ResourceTypeCredential,
		ResourceTypeNote:
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
