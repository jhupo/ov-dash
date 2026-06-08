package servers

import (
	"context"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

type Collector struct {
	repository  *Repository
	ssh         *SSHExecutor
	coordinator *ProbeCoordinator
}

func NewCollector(repository *Repository) *Collector {
	sshExecutor := NewSSHExecutor(20 * time.Second)
	probe := NewAgentProbe(sshExecutor)
	return &Collector{
		repository:  repository,
		ssh:         sshExecutor,
		coordinator: NewProbeCoordinator(repository, probe),
	}
}

func (c *Collector) CollectAll(ctx context.Context) {
	c.coordinator.CollectAll(ctx)
}

func (c *Collector) Collect(ctx context.Context, id string) error {
	return c.coordinator.Collect(ctx, id)
}

func (c *Collector) Install(ctx context.Context, id string) error {
	return c.coordinator.Install(ctx, id)
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
	return c.coordinator.TouchMonitor(ctx, ttl)
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

func (c *Collector) CollectAgentLoop(ctx context.Context, id string) error {
	return c.coordinator.CollectAgentLoop(ctx, id)
}

func trimError(err error) string {
	message := strings.TrimSpace(err.Error())
	if len(message) > 400 {
		return message[:400]
	}
	return message
}
