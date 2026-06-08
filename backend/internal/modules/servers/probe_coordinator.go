package servers

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"time"
)

type probeRepository interface {
	List(ctx context.Context) ([]Connection, error)
	Get(ctx context.Context, id string) (Connection, error)
	MarkCollecting(ctx context.Context, id string) error
	MarkCollectFailed(ctx context.Context, id string, message string) error
	SaveMetric(ctx context.Context, metric Metric) error
	SaveAgentMetric(ctx context.Context, metric Metric) error
	TouchMonitorActivity(ctx context.Context, ttl time.Duration) error
	MonitorActive(ctx context.Context) (bool, error)
}

type serverProbe interface {
	Install(ctx context.Context, item Connection) error
	EnsureCurrent(ctx context.Context, item Connection) error
	Collect(ctx context.Context, item Connection) (Metric, error)
	CollectOnce(ctx context.Context, item Connection) (Metric, error)
	Status(ctx context.Context, item Connection) (AgentStatus, error)
	StatusOnce(ctx context.Context, item Connection) (AgentStatus, error)
	Dial(ctx context.Context, item Connection) (net.Conn, error)
	CollectConn(ctx context.Context, item Connection, conn net.Conn, reader *bufio.Reader) (Metric, error)
}

type ProbeCoordinator struct {
	repository probeRepository
	probe      serverProbe
}

func NewProbeCoordinator(repository probeRepository, probe serverProbe) *ProbeCoordinator {
	return &ProbeCoordinator{
		repository: repository,
		probe:      probe,
	}
}

func (c *ProbeCoordinator) CollectAll(ctx context.Context) {
	items, err := c.repository.List(ctx)
	if err != nil {
		return
	}
	for _, item := range items {
		select {
		case <-ctx.Done():
			return
		default:
		}
		_ = c.Collect(ctx, item.ID)
	}
}

func (c *ProbeCoordinator) Collect(ctx context.Context, id string) error {
	item, err := c.repository.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := c.repository.MarkCollecting(ctx, item.ID); err != nil {
		return err
	}

	metric, err := c.collect(ctx, item)
	if err != nil {
		_ = c.repository.MarkCollectFailed(ctx, item.ID, trimError(c.annotateCollectError(ctx, item, err)))
		return err
	}
	return c.repository.SaveMetric(ctx, metric)
}

func (c *ProbeCoordinator) Install(ctx context.Context, id string) error {
	item, err := c.repository.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := c.repository.MarkCollecting(ctx, item.ID); err != nil {
		return err
	}
	if err := c.probe.Install(ctx, item); err != nil {
		_ = c.repository.MarkCollectFailed(ctx, item.ID, trimError(err))
		return err
	}
	metric, err := c.collectAfterInstall(ctx, item)
	if err != nil {
		_ = c.repository.MarkCollectFailed(ctx, item.ID, trimError(err))
		return err
	}
	return c.repository.SaveAgentMetric(ctx, metric)
}

func (c *ProbeCoordinator) Status(ctx context.Context, id string) (AgentStatus, error) {
	item, err := c.repository.Get(ctx, id)
	if err != nil {
		return AgentStatus{}, err
	}
	return c.probe.Status(ctx, item)
}

func (c *ProbeCoordinator) TouchMonitor(ctx context.Context, ttl time.Duration) error {
	return c.repository.TouchMonitorActivity(ctx, ttl)
}

func (c *ProbeCoordinator) CollectAgentLoop(ctx context.Context, id string) error {
	item, err := c.repository.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := c.repository.MarkCollecting(ctx, item.ID); err != nil {
		return err
	}
	if err := c.probe.EnsureCurrent(ctx, item); err != nil {
		_ = c.repository.MarkCollectFailed(ctx, item.ID, trimError(c.annotateCollectError(ctx, item, err)))
		return err
	}

	conn, err := c.probe.Dial(ctx, item)
	if err != nil {
		return c.collectAgentFallbackLoop(ctx, item, err)
	}
	defer conn.Close()
	reader := bufio.NewReader(conn)

	return c.collectAgentTCPStream(ctx, item, conn, reader)
}

func (c *ProbeCoordinator) collectAgentTCPStream(ctx context.Context, item Connection, conn net.Conn, reader *bufio.Reader) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		active, err := c.repository.MonitorActive(ctx)
		if err != nil {
			return err
		}
		if !active {
			return nil
		}

		metric, err := c.probe.CollectConn(ctx, item, conn, reader)
		if err != nil {
			_ = c.repository.MarkCollectFailed(ctx, item.ID, trimError(c.annotateCollectError(ctx, item, err)))
			return err
		}
		if err := c.repository.SaveAgentMetric(ctx, metric); err != nil {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (c *ProbeCoordinator) collectAgentFallbackLoop(ctx context.Context, item Connection, dialErr error) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		active, err := c.repository.MonitorActive(ctx)
		if err != nil {
			return err
		}
		if !active {
			return nil
		}

		metric, err := c.probe.CollectOnce(ctx, item)
		if err != nil {
			combined := fmt.Errorf("%w; ssh once fallback failed: %v", dialErr, err)
			_ = c.repository.MarkCollectFailed(ctx, item.ID, trimError(c.annotateCollectError(ctx, item, combined)))
			return err
		}
		if err := c.repository.SaveAgentMetric(ctx, metric); err != nil {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (c *ProbeCoordinator) collect(ctx context.Context, item Connection) (Metric, error) {
	if err := c.probe.EnsureCurrent(ctx, item); err != nil {
		return Metric{}, err
	}
	return c.collectAfterInstall(ctx, item)
}

func (c *ProbeCoordinator) collectAfterInstall(ctx context.Context, item Connection) (Metric, error) {
	metric, err := c.probe.Collect(ctx, item)
	if err == nil {
		return metric, nil
	}
	fallbackMetric, fallbackErr := c.probe.CollectOnce(ctx, item)
	if fallbackErr == nil {
		return fallbackMetric, nil
	}
	return Metric{}, fmt.Errorf("%w; ssh once fallback failed: %v", err, fallbackErr)
}

func (c *ProbeCoordinator) annotateCollectError(ctx context.Context, item Connection, err error) error {
	status, statusErr := c.probe.Status(ctx, item)
	if statusErr != nil {
		return err
	}
	return fmt.Errorf("%w; agent version=%s service_active=%t socat=%t nc=%t port=%d", err, status.Version, status.ServiceActive, status.Socat, status.NC, status.Port)
}
