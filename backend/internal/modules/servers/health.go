package servers

import (
	"context"
	"fmt"

	platformmodule "ov-dash/backend/internal/platform/module"
)

type collectionHealthSnapshot struct {
	Total           int
	ActiveMonitor   bool
	CollectingStale int
	ErrorCount      int
	AgentStale      int
}

func (Module) HealthChecks(ctx platformmodule.Context) []platformmodule.HealthCheck {
	repository := NewRepositoryWithSecrets(ctx.DB, ctx.Secrets)
	return []platformmodule.HealthCheck{
		{
			ID:   "servers.collection",
			Name: "Server collection",
			Check: func(checkCtx context.Context) error {
				snapshot, err := repository.CollectionHealth(checkCtx)
				if err != nil {
					return err
				}
				return evaluateCollectionHealth(snapshot)
			},
		},
	}
}

func (r *Repository) CollectionHealth(ctx context.Context) (collectionHealthSnapshot, error) {
	var snapshot collectionHealthSnapshot
	err := r.db.QueryRow(ctx, `
		SELECT
			count(*)::int,
			COALESCE((SELECT max(active_until) > now() FROM server_monitor_activity), false),
			count(*) FILTER (
				WHERE collect_status = 'collecting'
				  AND updated_at < now() - interval '2 minutes'
			)::int,
			count(*) FILTER (WHERE collect_status = 'error')::int,
			count(*) FILTER (
				WHERE collector_installed = true
				  AND (agent_last_seen_at IS NULL OR agent_last_seen_at < now() - interval '10 minutes')
			)::int
		FROM server_connections
	`).Scan(
		&snapshot.Total,
		&snapshot.ActiveMonitor,
		&snapshot.CollectingStale,
		&snapshot.ErrorCount,
		&snapshot.AgentStale,
	)
	if err != nil {
		return collectionHealthSnapshot{}, err
	}
	return snapshot, nil
}

func evaluateCollectionHealth(snapshot collectionHealthSnapshot) error {
	if snapshot.Total == 0 || !snapshot.ActiveMonitor {
		return nil
	}
	if snapshot.CollectingStale > 0 {
		return fmt.Errorf("%d server collections stuck for more than 2 minutes", snapshot.CollectingStale)
	}
	if snapshot.ErrorCount >= snapshot.Total {
		return fmt.Errorf("all %d server collections are failing", snapshot.Total)
	}
	if snapshot.AgentStale >= snapshot.Total {
		return fmt.Errorf("all %d installed server agents are stale", snapshot.Total)
	}
	return nil
}
