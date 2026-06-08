package servers

import (
	"bufio"
	"context"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

type Collector struct {
	repository *Repository
	ssh        *SSHExecutor
	probe      *AgentProbe
}

func NewCollector(repository *Repository) *Collector {
	sshExecutor := NewSSHExecutor(20 * time.Second)
	return &Collector{
		repository: repository,
		ssh:        sshExecutor,
		probe:      NewAgentProbe(sshExecutor),
	}
}

func (c *Collector) CollectAll(ctx context.Context) {
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

func (c *Collector) Collect(ctx context.Context, id string) error {
	item, err := c.repository.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := c.repository.MarkCollecting(ctx, item.ID); err != nil {
		return err
	}

	metric, err := c.collect(ctx, item)
	if err != nil {
		_ = c.repository.MarkCollectFailed(ctx, item.ID, trimError(err))
		return err
	}
	return c.repository.SaveMetric(ctx, metric)
}

func (c *Collector) Install(ctx context.Context, id string) error {
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
	metric, err := c.probe.Collect(ctx, item)
	if err != nil {
		_ = c.repository.MarkCollectFailed(ctx, item.ID, trimError(err))
		return err
	}
	return c.repository.SaveAgentMetric(ctx, metric)
}

func (c *Collector) RunCommand(ctx context.Context, id string, command string) (string, error) {
	item, err := c.repository.Get(ctx, id)
	if err != nil {
		return "", err
	}
	client, err := c.ssh.Connect(ctx, item)
	if err != nil {
		return "", err
	}
	defer client.Close()
	return c.ssh.Output(ctx, client, command)
}

func (c *Collector) TouchMonitor(ctx context.Context, ttl time.Duration) error {
	return c.repository.TouchMonitorActivity(ctx, ttl)
}

func (c *Collector) Shell(ctx context.Context, id string) (*ssh.Client, *ssh.Session, error) {
	item, err := c.repository.Get(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	client, err := c.ssh.Connect(ctx, item)
	if err != nil {
		return nil, nil, err
	}
	session, err := client.NewSession()
	if err != nil {
		client.Close()
		return nil, nil, err
	}
	if err := session.RequestPty("xterm-256color", 40, 120, ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}); err != nil {
		session.Close()
		client.Close()
		return nil, nil, err
	}
	return client, session, nil
}

func (c *Collector) collect(ctx context.Context, item Connection) (Metric, error) {
	if err := c.probe.Install(ctx, item); err != nil {
		return Metric{}, err
	}
	return c.probe.Collect(ctx, item)
}

func (c *Collector) CollectAgentLoop(ctx context.Context, id string) error {
	item, err := c.repository.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := c.repository.MarkCollecting(ctx, item.ID); err != nil {
		return err
	}

	conn, err := c.probe.Dial(ctx, item)
	if err != nil {
		if err := c.probe.Install(ctx, item); err != nil {
			_ = c.repository.MarkCollectFailed(ctx, item.ID, trimError(err))
			return err
		}
		conn, err = c.probe.Dial(ctx, item)
	}
	if err != nil {
		_ = c.repository.MarkCollectFailed(ctx, item.ID, trimError(err))
		return err
	}
	defer conn.Close()
	reader := bufio.NewReader(conn)

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
			_ = c.repository.MarkCollectFailed(ctx, item.ID, trimError(err))
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

func trimError(err error) string {
	message := strings.TrimSpace(err.Error())
	if len(message) > 400 {
		return message[:400]
	}
	return message
}
