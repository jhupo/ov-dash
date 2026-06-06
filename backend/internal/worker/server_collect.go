package worker

import (
	"context"
	"fmt"

	"ov-dash/backend/internal/modules/servers"
	"ov-dash/backend/internal/modules/tasks"
	"ov-dash/backend/internal/queue"

	"go.uber.org/zap"
)

type ServerCollectHandler struct {
	collector *servers.Collector
	tasks     *tasks.Repository
	logger    *zap.Logger
}

func NewServerCollectHandler(collector *servers.Collector, tasks *tasks.Repository, logger *zap.Logger) *ServerCollectHandler {
	return &ServerCollectHandler{
		collector: collector,
		tasks:     tasks,
		logger:    logger,
	}
}

func (h *ServerCollectHandler) Handle(ctx context.Context, job queue.Job) error {
	serverID, _ := job.Payload["server_id"].(string)
	taskID, _ := job.Payload["task_id"].(string)
	if serverID == "" {
		return fmt.Errorf("server_id is required")
	}
	if taskID != "" {
		_ = h.tasks.UpdateStatus(ctx, taskID, "in progress", "正在安装或连接服务器 Agent")
	}

	if err := h.collector.CollectAgentLoop(ctx, serverID); err != nil {
		if taskID != "" {
			_ = h.tasks.UpdateStatus(ctx, taskID, "failed", "Agent 采集连接异常: "+err.Error())
		}
		return err
	}

	if taskID != "" {
		_ = h.tasks.UpdateStatus(ctx, taskID, "done", "Agent 采集会话已结束")
	}
	h.logger.Info("server agent session ended", zap.String("server_id", serverID), zap.String("job_id", job.ID))
	return nil
}
