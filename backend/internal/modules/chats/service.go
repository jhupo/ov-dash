package chats

import "context"

type Service struct {
	repository *Repository
}

func NewService(repository *Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) ListConversations(ctx context.Context) ([]Conversation, error) {
	return s.repository.ListConversations(ctx)
}
