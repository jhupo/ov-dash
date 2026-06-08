package servers

import (
	"context"
	"strings"
	"time"
)

type Service struct {
	inventory *InventoryService
	metrics   *MetricsStore
}

func NewService(repository *Repository) *Service {
	return &Service{
		inventory: NewInventoryService(repository),
		metrics:   repository.metricStore(),
	}
}

func (s *Service) List(ctx context.Context) ([]PublicConnection, error) {
	return s.inventory.List(ctx)
}

func (s *Service) Save(ctx context.Context, input SaveInput) (PublicConnection, error) {
	return s.inventory.Save(ctx, input)
}

func (s *Service) Delete(ctx context.Context, id string) error {
	return s.inventory.Delete(ctx, id)
}

func (s *Service) Samples(ctx context.Context, id string, since time.Time) ([]Metric, error) {
	return s.metrics.Samples(ctx, strings.TrimSpace(id), since)
}
