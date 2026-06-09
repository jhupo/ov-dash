package servers

import (
	"context"
	"time"

	"ov-dash/backend/internal/queue"
)

const (
	ServerCollectJobType     = "server.collect"
	ServerAgentUpdateJobType = "server.agent.update"
)

type AgentActivity struct {
	ServerID            string             `json:"server_id"`
	CollectStatus       string             `json:"collect_status"`
	CollectError        string             `json:"collect_error"`
	CollectFailureCount int                `json:"collect_failure_count"`
	LastCollectedAt     *time.Time         `json:"last_collected_at,omitempty"`
	AgentLastSeenAt     *time.Time         `json:"agent_last_seen_at,omitempty"`
	CurrentMetric       *Metric            `json:"current_metric,omitempty"`
	Jobs                []AgentActivityJob `json:"jobs"`
}

type AgentActivityJob struct {
	Job       queue.JobRecord `json:"job"`
	Stage     string          `json:"stage"`
	Logs      []queue.JobLog  `json:"logs"`
	UpdatedAt time.Time       `json:"updated_at"`
}

type agentActivityQueue interface {
	ListJobsFiltered(ctx context.Context, filter queue.JobListFilter) ([]queue.JobRecord, error)
	ListJobLogs(ctx context.Context, id string) ([]queue.JobLog, error)
}

func BuildAgentActivity(ctx context.Context, item Connection, jobs agentActivityQueue) (AgentActivity, error) {
	activity := AgentActivity{
		ServerID:            item.ID,
		CollectStatus:       item.CollectStatus,
		CollectError:        item.CollectError,
		CollectFailureCount: item.CollectFailureCount,
		LastCollectedAt:     item.LastCollectedAt,
		AgentLastSeenAt:     item.AgentLastSeenAt,
		CurrentMetric:       item.Metric,
		Jobs:                []AgentActivityJob{},
	}
	if jobs == nil {
		return activity, nil
	}

	records, err := jobs.ListJobsFiltered(ctx, queue.JobListFilter{
		Types:    []string{ServerCollectJobType, ServerAgentUpdateJobType},
		ServerID: item.ID,
		Limit:    10,
	})
	if err != nil {
		return AgentActivity{}, err
	}
	for _, record := range records {
		logs, err := jobs.ListJobLogs(ctx, record.ID)
		if err != nil {
			return AgentActivity{}, err
		}
		activity.Jobs = append(activity.Jobs, AgentActivityJob{
			Job:       record,
			Stage:     latestJobStage(logs),
			Logs:      tailJobLogs(logs, 20),
			UpdatedAt: record.UpdatedAt,
		})
	}
	return activity, nil
}

func latestJobStage(logs []queue.JobLog) string {
	for i := len(logs) - 1; i >= 0; i-- {
		if stage, _ := logs[i].Metadata["stage"].(string); stage != "" {
			return stage
		}
	}
	return ""
}

func tailJobLogs(logs []queue.JobLog, limit int) []queue.JobLog {
	if limit < 1 || len(logs) <= limit {
		return logs
	}
	return logs[len(logs)-limit:]
}
