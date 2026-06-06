package tasks

import "context"

type Service struct {
	repository *Repository
}

func NewService(repository *Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) List(ctx context.Context) ([]Task, error) {
	return s.repository.List(ctx)
}

func (s *Service) Delete(ctx context.Context, ids []string) error {
	return s.repository.Delete(ctx, ids)
}
