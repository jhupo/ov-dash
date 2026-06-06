package worker

import (
	"context"
	"fmt"

	"ov-dash/backend/internal/modules/servers"
	"ov-dash/backend/internal/queue"

	"go.uber.org/zap"
)

type ServerCollectHandler struct {
	collector *servers.Collector
	logger    *zap.Logger
}

func NewServerCollectHandler(collector *servers.Collector, logger *zap.Logger) *ServerCollectHandler {
	return &ServerCollectHandler{
		collector: collector,
		logger:    logger,
	}
}

func (h *ServerCollectHandler) Handle(ctx context.Context, job queue.Job) error {
	serverID, _ := job.Payload["server_id"].(string)
	if serverID == "" {
		return fmt.Errorf("server_id is required")
	}

	if err := h.collector.CollectAgentLoop(ctx, serverID); err != nil {
		return err
	}

	h.logger.Info("server agent session ended", zap.String("server_id", serverID), zap.String("job_id", job.ID))
	return nil
}
