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
	logs      jobLogAppender
}

const ServerCollectMaxAttempts = queue.DefaultMaxAttempts

type jobLogAppender interface {
	AppendJobLog(ctx context.Context, id string, stream string, message string, metadata map[string]any) error
}

func NewCollectJobHandler(collector *Collector, logger *zap.Logger, logs ...jobLogAppender) *CollectJobHandler {
	handler := &CollectJobHandler{
		collector: collector,
		logger:    logger,
	}
	if len(logs) > 0 {
		handler.logs = logs[0]
	}
	return handler
}

type collectJobObserver struct {
	jobID string
	logs  jobLogAppender
}

func (o collectJobObserver) ObserveCollection(ctx context.Context, event CollectionEvent) {
	if o.logs == nil {
		return
	}
	stream := event.Stream
	if stream == "" {
		stream = "system"
	}
	metadata := map[string]any{
		"server_id": event.ServerID,
		"stage":     event.Stage,
	}
	if event.Mode != "" {
		metadata["mode"] = event.Mode
	}
	for key, value := range event.Metadata {
		metadata[key] = value
	}
	_ = o.logs.AppendJobLog(ctx, o.jobID, stream, event.Message, metadata)
}

func (Module) RegisterJobs(ctx platformmodule.Context, reg *platformmodule.JobRegistry) error {
	var logs []jobLogAppender
	if ctx.Queue != nil {
		logs = append(logs, ctx.Queue)
	}
	return reg.Register(platformmodule.JobDefinition{
		Type:        "server.collect",
		Description: "Open a server agent collection session.",
		MaxAttempts: ServerCollectMaxAttempts,
		Handler: NewCollectJobHandler(
			NewCollector(NewRepositoryWithSecrets(ctx.DB, ctx.Secrets)),
			ctx.Logger,
			logs...,
		),
	})
}

func (h *CollectJobHandler) Handle(ctx context.Context, job queue.Job) error {
	serverID, _ := job.Payload["server_id"].(string)
	if serverID == "" {
		return fmt.Errorf("server_id is required")
	}

	observer := CollectionObserver(nil)
	if h.logs != nil {
		observer = collectJobObserver{jobID: job.ID, logs: h.logs}
	}

	if err := h.collector.CollectAgentLoop(ctx, serverID, observer); err != nil {
		return err
	}

	if h.logger != nil {
		h.logger.Info("server agent session ended", zap.String("server_id", serverID), zap.String("job_id", job.ID))
	}
	return nil
}
