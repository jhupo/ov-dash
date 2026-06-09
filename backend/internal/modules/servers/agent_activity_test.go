package servers

import (
	"context"
	"testing"
	"time"

	"ov-dash/backend/internal/queue"
)

func TestBuildAgentActivityIncludesRecentJobsAndStages(t *testing.T) {
	now := time.Date(2026, 6, 9, 18, 0, 0, 0, time.UTC)
	jobs := &agentActivityQueueStub{
		records: []queue.JobRecord{
			{ID: "job-update", Type: ServerAgentUpdateJobType, Payload: map[string]any{"server_id": "srv_1"}, Status: queue.JobStatusRunning, UpdatedAt: now},
			{ID: "job-collect", Type: ServerCollectJobType, Payload: map[string]any{"server_id": "srv_1"}, Status: queue.JobStatusCompleted, UpdatedAt: now.Add(-time.Minute)},
		},
		logs: map[string][]queue.JobLog{
			"job-update": {
				{JobID: "job-update", Message: "start", Metadata: map[string]any{"stage": "agent.install.start"}},
				{JobID: "job-update", Message: "ready", Metadata: map[string]any{"stage": "agent.install.ready"}},
			},
			"job-collect": {
				{JobID: "job-collect", Message: "ok", Metadata: map[string]any{"stage": "collect.ok"}},
			},
		},
	}

	activity, err := BuildAgentActivity(context.Background(), Connection{
		ID:                  "srv_1",
		CollectStatus:       "collecting",
		CollectFailureCount: 2,
		Metric:              &Metric{ServerID: "srv_1", CPUPercent: 12},
	}, jobs)
	if err != nil {
		t.Fatalf("BuildAgentActivity returned error: %v", err)
	}

	if activity.ServerID != "srv_1" || activity.CollectStatus != "collecting" || activity.CollectFailureCount != 2 {
		t.Fatalf("activity fields = %+v", activity)
	}
	if len(activity.Jobs) != 2 {
		t.Fatalf("jobs = %d, want 2", len(activity.Jobs))
	}
	if activity.Jobs[0].Stage != "agent.install.ready" || activity.Jobs[1].Stage != "collect.ok" {
		t.Fatalf("stages = %#v", []string{activity.Jobs[0].Stage, activity.Jobs[1].Stage})
	}
	if jobs.filter.ServerID != "srv_1" || jobs.filter.Limit != 10 {
		t.Fatalf("filter = %#v", jobs.filter)
	}
	if len(jobs.filter.Types) != 2 || jobs.filter.Types[0] != ServerCollectJobType || jobs.filter.Types[1] != ServerAgentUpdateJobType {
		t.Fatalf("filter types = %#v", jobs.filter.Types)
	}
}

func TestBuildAgentActivityTailsLogs(t *testing.T) {
	logs := make([]queue.JobLog, 25)
	for i := range logs {
		logs[i] = queue.JobLog{ID: int64(i + 1), JobID: "job-1", Metadata: map[string]any{"stage": "stage"}}
	}
	activity, err := BuildAgentActivity(context.Background(), Connection{ID: "srv_1"}, &agentActivityQueueStub{
		records: []queue.JobRecord{{ID: "job-1", Type: ServerCollectJobType}},
		logs:    map[string][]queue.JobLog{"job-1": logs},
	})
	if err != nil {
		t.Fatalf("BuildAgentActivity returned error: %v", err)
	}
	if len(activity.Jobs) != 1 || len(activity.Jobs[0].Logs) != 20 {
		t.Fatalf("logs = %#v", activity.Jobs)
	}
	if activity.Jobs[0].Logs[0].ID != 6 {
		t.Fatalf("first tailed log id = %d, want 6", activity.Jobs[0].Logs[0].ID)
	}
}

type agentActivityQueueStub struct {
	filter  queue.JobListFilter
	records []queue.JobRecord
	logs    map[string][]queue.JobLog
}

func (s *agentActivityQueueStub) ListJobsFiltered(ctx context.Context, filter queue.JobListFilter) ([]queue.JobRecord, error) {
	s.filter = filter
	return s.records, nil
}

func (s *agentActivityQueueStub) ListJobLogs(ctx context.Context, id string) ([]queue.JobLog, error) {
	return s.logs[id], nil
}
