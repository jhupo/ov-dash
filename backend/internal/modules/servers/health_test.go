package servers

import (
	"strings"
	"testing"
)

func TestEvaluateCollectionHealthIgnoresInactiveMonitor(t *testing.T) {
	err := evaluateCollectionHealth(collectionHealthSnapshot{
		Total:           3,
		ActiveMonitor:   false,
		CollectingStale: 3,
		ErrorCount:      3,
		AgentStale:      3,
	})
	if err != nil {
		t.Fatalf("evaluateCollectionHealth returned error for inactive monitor: %v", err)
	}
}

func TestEvaluateCollectionHealthDetectsStuckCollections(t *testing.T) {
	err := evaluateCollectionHealth(collectionHealthSnapshot{
		Total:           3,
		ActiveMonitor:   true,
		CollectingStale: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "stuck") {
		t.Fatalf("expected stuck collection error, got %v", err)
	}
}

func TestEvaluateCollectionHealthDetectsAllFailing(t *testing.T) {
	err := evaluateCollectionHealth(collectionHealthSnapshot{
		Total:         2,
		ActiveMonitor: true,
		ErrorCount:    2,
	})
	if err == nil || !strings.Contains(err.Error(), "all 2 server collections are failing") {
		t.Fatalf("expected all failing error, got %v", err)
	}
}

func TestEvaluateCollectionHealthDetectsAllInstalledAgentsStale(t *testing.T) {
	err := evaluateCollectionHealth(collectionHealthSnapshot{
		Total:         2,
		ActiveMonitor: true,
		AgentStale:    2,
	})
	if err == nil || !strings.Contains(err.Error(), "all 2 installed server agents are stale") {
		t.Fatalf("expected stale agent error, got %v", err)
	}
}

func TestEvaluateCollectionHealthAllowsPartialFailures(t *testing.T) {
	err := evaluateCollectionHealth(collectionHealthSnapshot{
		Total:         3,
		ActiveMonitor: true,
		ErrorCount:    1,
		AgentStale:    1,
	})
	if err != nil {
		t.Fatalf("evaluateCollectionHealth returned error for partial failures: %v", err)
	}
}
