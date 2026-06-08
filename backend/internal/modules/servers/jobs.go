package servers

import (
	"context"
	"fmt"

	platformmodule "ov-dash/backend/internal/platform/module"
	"ov-dash/backend/internal/queue"

	"go.uber.org/zap"
)

type CollectJobHandler struct {
	collector *Collector
	logger    *zap.Logger
}

func NewCollectJobHandler(collector *Collector, logger *zap.Logger) *CollectJobHandler {
	return &CollectJobHandler{
		collector: collector,
		logger:    logger,
	}
}

func (Module) RegisterJobs(ctx platformmodule.Context, reg *platformmodule.JobRegistry) error {
	return reg.Register(platformmodule.JobDefinition{
		Type:        "server.collect",
		Description: "Open a server agent collection session.",
		MaxAttempts: 1,
		Handler: NewCollectJobHandler(
			NewCollector(NewRepositoryWithSecrets(ctx.DB, ctx.Secrets)),
			ctx.Logger,
		),
	})
}

func (h *CollectJobHandler) Handle(ctx context.Context, job queue.Job) error {
	serverID, _ := job.Payload["server_id"].(string)
	if serverID == "" {
		return fmt.Errorf("server_id is required")
	}

	if err := h.collector.CollectAgentLoop(ctx, serverID); err != nil {
		return err
	}

	if h.logger != nil {
		h.logger.Info("server agent session ended", zap.String("server_id", serverID), zap.String("job_id", job.ID))
	}
	return nil
}
