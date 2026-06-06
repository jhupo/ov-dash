package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"

	"ov-dash/backend/internal/config"
	"ov-dash/backend/internal/queue"

	"go.uber.org/zap"
)

type PythonScriptHandler struct {
	cfg    config.PythonConfig
	logger *zap.Logger
}

type pythonScriptPayload struct {
	Script string         `json:"script"`
	Args   []string       `json:"args"`
	Input  map[string]any `json:"input"`
}

func NewPythonScriptHandler(cfg config.PythonConfig, logger *zap.Logger) *PythonScriptHandler {
	return &PythonScriptHandler{cfg: cfg, logger: logger}
}

func (h *PythonScriptHandler) Handle(ctx context.Context, job queue.Job) error {
	var payload pythonScriptPayload
	raw, err := json.Marshal(job.Payload)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return err
	}
	if payload.Script == "" {
		return errors.New("script is required")
	}
	if !filepath.IsLocal(payload.Script) {
		return fmt.Errorf("script path must be local: %s", payload.Script)
	}
	scriptPath := filepath.Clean(filepath.Join(h.cfg.ScriptsDir, payload.Script))

	args := append([]string{scriptPath}, payload.Args...)
	cmd := exec.CommandContext(ctx, h.cfg.Bin, args...)
	output, err := cmd.CombinedOutput()
	if len(output) > 0 {
		h.logger.Info("python script output", zap.String("job_id", job.ID), zap.ByteString("output", output))
	}
	if err != nil {
		return fmt.Errorf("run python script: %w", err)
	}
	return nil
}

type NoopHandler struct{}

func (NoopHandler) Handle(context.Context, queue.Job) error {
	return nil
}
