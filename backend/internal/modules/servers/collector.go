package servers

import (
	"context"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

type Collector struct {
	repository  *Repository
	ssh         *SSHAccessService
	coordinator *ProbeCoordinator
}

func NewCollector(repository *Repository) *Collector {
	sshExecutor := NewSSHExecutor(20 * time.Second)
	probe := NewAgentProbe(sshExecutor)
	return &Collector{
		repository:  repository,
		ssh:         NewSSHAccessService(repository, sshExecutor),
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

func (c *Collector) AgentStatus(ctx context.Context, id string) (AgentStatus, error) {
	return c.coordinator.Status(ctx, id)
}

func (c *Collector) AgentDiagnostics(ctx context.Context, id string) (AgentDiagnostics, error) {
	return c.coordinator.Diagnostics(ctx, id)
}

func (c *Collector) RunCommand(ctx context.Context, id string, command string) (string, error) {
	return c.ssh.RunCommand(ctx, id, command)
}

func (c *Collector) TouchMonitor(ctx context.Context, ttl time.Duration) error {
	return c.coordinator.TouchMonitor(ctx, ttl)
}

func (c *Collector) Shell(ctx context.Context, id string) (*ssh.Client, *ssh.Session, error) {
	return c.ssh.OpenShell(ctx, id)
}

func (c *Collector) CollectAgentLoop(ctx context.Context, id string, observers ...CollectionObserver) error {
	coordinator := c.coordinator
	if len(observers) > 0 && observers[0] != nil {
		coordinator = coordinator.WithObserver(observers[0])
	}
	return coordinator.CollectAgentLoop(ctx, id)
}

func trimError(err error) string {
	message := strings.TrimSpace(err.Error())
	if len(message) > 400 {
		return message[:400]
	}
	return message
}
