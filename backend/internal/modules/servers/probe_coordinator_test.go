package servers

import (
	"bufio"
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"
)

type probeRepositoryStub struct {
	items          []Connection
	item           Connection
	collecting     []string
	failed         map[string]string
	saved          []Metric
	savedAgent     []Metric
	monitorActive  bool
	monitorChecks  int
	activeChecks   int
	touchedMonitor bool
}

func newProbeRepositoryStub(item Connection) *probeRepositoryStub {
	return &probeRepositoryStub{
		items:         []Connection{item},
		item:          item,
		failed:        map[string]string{},
		monitorActive: false,
	}
}

func (r *probeRepositoryStub) List(ctx context.Context) ([]Connection, error) {
	return r.items, nil
}

func (r *probeRepositoryStub) Get(ctx context.Context, id string) (Connection, error) {
	return r.item, nil
}

func (r *probeRepositoryStub) MarkCollecting(ctx context.Context, id string) error {
	r.collecting = append(r.collecting, id)
	return nil
}

func (r *probeRepositoryStub) MarkCollectFailed(ctx context.Context, id string, message string) error {
	r.failed[id] = message
	return nil
}

func (r *probeRepositoryStub) SaveMetric(ctx context.Context, metric Metric) error {
	r.saved = append(r.saved, metric)
	return nil
}

func (r *probeRepositoryStub) SaveAgentMetric(ctx context.Context, metric Metric) error {
	r.savedAgent = append(r.savedAgent, metric)
	return nil
}

func (r *probeRepositoryStub) TouchMonitorActivity(ctx context.Context, ttl time.Duration) error {
	r.touchedMonitor = true
	return nil
}

func (r *probeRepositoryStub) MonitorActive(ctx context.Context) (bool, error) {
	r.monitorChecks++
	if r.activeChecks > 0 {
		return r.monitorChecks <= r.activeChecks, nil
	}
	return r.monitorActive, nil
}

type serverProbeStub struct {
	installErr          error
	ensureErr           error
	ensureCalls         int
	installCalls        int
	installWithObserver bool
	collectErr          error
	collectOnceErr      error
	dialErr             error
	metric              Metric
	onceMetric          Metric
	status              AgentStatus
	statusErr           error
}

type collectionObserverStub struct {
	events []CollectionEvent
}

func (o *collectionObserverStub) ObserveCollection(ctx context.Context, event CollectionEvent) {
	o.events = append(o.events, event)
}

func (p *serverProbeStub) Install(ctx context.Context, item Connection) error {
	p.installCalls++
	return p.installErr
}

func (p *serverProbeStub) InstallWithObserver(ctx context.Context, item Connection, observer CollectionObserver) error {
	p.installCalls++
	if p.installWithObserver {
		observeCollection(ctx, observer, item.ID, "agent.install.start", "", "installing server agent", nil)
		observeCollection(ctx, observer, item.ID, "agent.install.ready", "", "server agent is installed and current", map[string]any{"version": currentAgentVersion})
	}
	return p.installErr
}

func (p *serverProbeStub) EnsureCurrent(ctx context.Context, item Connection) error {
	p.ensureCalls++
	if p.ensureErr != nil {
		return p.ensureErr
	}
	return p.installErr
}

func (p *serverProbeStub) Collect(ctx context.Context, item Connection) (Metric, error) {
	if p.collectErr != nil {
		return Metric{}, p.collectErr
	}
	return p.metric, nil
}

func (p *serverProbeStub) CollectOnce(ctx context.Context, item Connection) (Metric, error) {
	if p.collectOnceErr != nil {
		return Metric{}, p.collectOnceErr
	}
	return p.onceMetric, nil
}

func (p *serverProbeStub) Status(ctx context.Context, item Connection) (AgentStatus, error) {
	if p.statusErr != nil {
		return AgentStatus{}, p.statusErr
	}
	return p.status, nil
}

func (p *serverProbeStub) StatusOnce(ctx context.Context, item Connection) (AgentStatus, error) {
	return p.Status(ctx, item)
}

func (p *serverProbeStub) Diagnostics(ctx context.Context, item Connection) AgentDiagnostics {
	return AgentDiagnostics{
		ServerID:        item.ID,
		Status:          DiagnosticOK,
		ExpectedVersion: currentAgentVersion,
		AgentPort:       agentPort(item),
		CheckedAt:       time.Now().UTC(),
	}
}

func (p *serverProbeStub) Dial(ctx context.Context, item Connection) (net.Conn, error) {
	if p.dialErr != nil {
		return nil, p.dialErr
	}
	return nil, errors.New("dial unavailable in unit test")
}

func (p *serverProbeStub) CollectConn(ctx context.Context, item Connection, conn net.Conn, reader *bufio.Reader) (Metric, error) {
	return Metric{}, errors.New("collect conn unavailable in unit test")
}

func TestProbeCoordinatorCollectMarksAndSavesMetric(t *testing.T) {
	repository := newProbeRepositoryStub(Connection{ID: "srv_1"})
	probe := &serverProbeStub{metric: Metric{ServerID: "srv_1", CPUPercent: 12}, onceMetric: Metric{ServerID: "srv_1", CPUPercent: 99}}
	coordinator := NewProbeCoordinator(repository, probe)

	if err := coordinator.Collect(context.Background(), "srv_1"); err != nil {
		t.Fatalf("Collect returned error: %v", err)
	}
	if len(repository.collecting) != 1 || repository.collecting[0] != "srv_1" {
		t.Fatalf("collecting marks = %#v", repository.collecting)
	}
	if len(repository.saved) != 1 || repository.saved[0].CPUPercent != 12 {
		t.Fatalf("saved metrics = %#v", repository.saved)
	}
	if len(repository.failed) != 0 {
		t.Fatalf("unexpected failed marks: %#v", repository.failed)
	}
}

func TestProbeCoordinatorCollectFallsBackToSSHOnce(t *testing.T) {
	repository := newProbeRepositoryStub(Connection{ID: "srv_1"})
	probe := &serverProbeStub{
		collectErr: errors.New("tcp blocked"),
		onceMetric: Metric{
			ServerID:   "srv_1",
			CPUPercent: 55,
		},
	}
	coordinator := NewProbeCoordinator(repository, probe)

	if err := coordinator.Collect(context.Background(), "srv_1"); err != nil {
		t.Fatalf("Collect returned error: %v", err)
	}
	if len(repository.saved) != 1 || repository.saved[0].CPUPercent != 55 {
		t.Fatalf("ssh once metric was not saved: %#v", repository.saved)
	}
}

func TestProbeCoordinatorCollectMarksFailure(t *testing.T) {
	repository := newProbeRepositoryStub(Connection{ID: "srv_1"})
	probe := &serverProbeStub{ensureErr: errors.New("install failed"), collectOnceErr: errors.New("once failed"), statusErr: errors.New("status unavailable")}
	coordinator := NewProbeCoordinator(repository, probe)

	if err := coordinator.Collect(context.Background(), "srv_1"); err == nil {
		t.Fatal("Collect returned nil error")
	}
	if repository.failed["srv_1"] != "install failed" {
		t.Fatalf("failure was not recorded: %#v", repository.failed)
	}
}

func TestProbeCoordinatorCollectFailureIncludesAgentStatusWhenAvailable(t *testing.T) {
	repository := newProbeRepositoryStub(Connection{ID: "srv_1"})
	probe := &serverProbeStub{
		ensureErr:      errors.New("install failed"),
		collectOnceErr: errors.New("once failed"),
		status: AgentStatus{
			Version:       currentAgentVersion,
			Port:          19087,
			Socat:         true,
			NC:            false,
			ServiceActive: true,
		},
	}
	coordinator := NewProbeCoordinator(repository, probe)

	if err := coordinator.Collect(context.Background(), "srv_1"); err == nil {
		t.Fatal("Collect returned nil error")
	}
	message := repository.failed["srv_1"]
	for _, snippet := range []string{"install failed", "agent version=", "service_active=true", "socat=true", "port=19087"} {
		if !strings.Contains(message, snippet) {
			t.Fatalf("failure message missing %q: %s", snippet, message)
		}
	}
}

func TestProbeCoordinatorInstallSavesAgentMetric(t *testing.T) {
	repository := newProbeRepositoryStub(Connection{ID: "srv_1"})
	probe := &serverProbeStub{metric: Metric{ServerID: "srv_1", CPUPercent: 34}}
	coordinator := NewProbeCoordinator(repository, probe)

	if err := coordinator.Install(context.Background(), "srv_1"); err != nil {
		t.Fatalf("Install returned error: %v", err)
	}
	if len(repository.savedAgent) != 1 || repository.savedAgent[0].CPUPercent != 34 {
		t.Fatalf("saved agent metrics = %#v", repository.savedAgent)
	}
}

func TestProbeCoordinatorInstallFallsBackToSSHOnceMetric(t *testing.T) {
	repository := newProbeRepositoryStub(Connection{ID: "srv_1"})
	probe := &serverProbeStub{
		collectErr: errors.New("tcp blocked"),
		onceMetric: Metric{
			ServerID:   "srv_1",
			CPUPercent: 77,
		},
	}
	coordinator := NewProbeCoordinator(repository, probe)

	if err := coordinator.Install(context.Background(), "srv_1"); err != nil {
		t.Fatalf("Install returned error: %v", err)
	}
	if len(repository.savedAgent) != 1 || repository.savedAgent[0].CPUPercent != 77 {
		t.Fatalf("ssh once agent metric was not saved: %#v", repository.savedAgent)
	}
}

func TestProbeCoordinatorStatusDelegates(t *testing.T) {
	repository := newProbeRepositoryStub(Connection{ID: "srv_1"})
	probe := &serverProbeStub{status: AgentStatus{ServerID: "srv_1", Version: currentAgentVersion}}
	coordinator := NewProbeCoordinator(repository, probe)

	status, err := coordinator.Status(context.Background(), "srv_1")
	if err != nil {
		t.Fatalf("Status returned error: %v", err)
	}
	if status.Version != currentAgentVersion {
		t.Fatalf("status = %+v", status)
	}
}

func TestProbeCoordinatorCollectAgentLoopFallsBackToSSHOnce(t *testing.T) {
	repository := newProbeRepositoryStub(Connection{ID: "srv_1"})
	repository.activeChecks = 1
	probe := &serverProbeStub{
		dialErr: errors.New("tcp blocked"),
		onceMetric: Metric{
			ServerID:   "srv_1",
			CPUPercent: 88,
		},
	}
	coordinator := NewProbeCoordinator(repository, probe)

	if err := coordinator.CollectAgentLoop(context.Background(), "srv_1"); err != nil {
		t.Fatalf("CollectAgentLoop returned error: %v", err)
	}
	if len(repository.savedAgent) != 1 || repository.savedAgent[0].CPUPercent != 88 {
		t.Fatalf("fallback agent metric was not saved: %#v", repository.savedAgent)
	}
	if len(repository.failed) != 0 {
		t.Fatalf("unexpected failed marks: %#v", repository.failed)
	}
	if probe.installCalls != 0 {
		t.Fatalf("unexpected install calls before ssh fallback: %d", probe.installCalls)
	}
	if probe.ensureCalls != 1 {
		t.Fatalf("EnsureCurrent calls = %d, want 1", probe.ensureCalls)
	}
}

func TestProbeCoordinatorCollectAgentLoopEmitsFallbackEvents(t *testing.T) {
	repository := newProbeRepositoryStub(Connection{ID: "srv_1"})
	repository.activeChecks = 1
	probe := &serverProbeStub{
		dialErr: errors.New("tcp blocked"),
		onceMetric: Metric{
			ServerID:  "srv_1",
			LatencyMS: 12.5,
		},
	}
	observer := &collectionObserverStub{}
	coordinator := NewProbeCoordinator(repository, probe).WithObserver(observer)

	if err := coordinator.CollectAgentLoop(context.Background(), "srv_1"); err != nil {
		t.Fatalf("CollectAgentLoop returned error: %v", err)
	}

	stages := make([]string, 0, len(observer.events))
	for _, event := range observer.events {
		stages = append(stages, event.Stage)
		if event.ServerID != "srv_1" {
			t.Fatalf("event server id = %q, want srv_1", event.ServerID)
		}
	}
	want := []string{"start", "agent.ensure", "agent.ready", "fallback", "collect.ok", "monitor.inactive"}
	if strings.Join(stages, ",") != strings.Join(want, ",") {
		t.Fatalf("stages = %#v, want %#v", stages, want)
	}
	if observer.events[3].Mode != "ssh_once" {
		t.Fatalf("fallback mode = %q, want ssh_once", observer.events[3].Mode)
	}
	if observer.events[4].Metadata["latency_ms"] != 12.5 {
		t.Fatalf("collect latency metadata = %#v", observer.events[4].Metadata)
	}
}

func TestProbeCoordinatorCollectAgentLoopFailsWhenEnsureCurrentFails(t *testing.T) {
	repository := newProbeRepositoryStub(Connection{ID: "srv_1"})
	probe := &serverProbeStub{
		ensureErr: errors.New("agent update failed"),
		statusErr: errors.New("status unavailable"),
	}
	coordinator := NewProbeCoordinator(repository, probe)

	if err := coordinator.CollectAgentLoop(context.Background(), "srv_1"); err == nil {
		t.Fatal("CollectAgentLoop returned nil error")
	}
	if probe.ensureCalls != 1 {
		t.Fatalf("EnsureCurrent calls = %d, want 1", probe.ensureCalls)
	}
	if repository.failed["srv_1"] != "agent update failed" {
		t.Fatalf("failure was not recorded: %#v", repository.failed)
	}
}

func TestProbeCoordinatorCollectAgentLoopMarksFallbackFailure(t *testing.T) {
	repository := newProbeRepositoryStub(Connection{ID: "srv_1"})
	repository.activeChecks = 1
	probe := &serverProbeStub{
		dialErr:        errors.New("tcp blocked"),
		collectOnceErr: errors.New("ssh failed"),
		statusErr:      errors.New("status unavailable"),
	}
	coordinator := NewProbeCoordinator(repository, probe)

	if err := coordinator.CollectAgentLoop(context.Background(), "srv_1"); err == nil {
		t.Fatal("CollectAgentLoop returned nil error")
	}
	message := repository.failed["srv_1"]
	if !strings.Contains(message, "tcp blocked") || !strings.Contains(message, "ssh once fallback failed") {
		t.Fatalf("fallback failure was not recorded with both causes: %s", message)
	}
}

func TestProbeCoordinatorTouchMonitorDelegates(t *testing.T) {
	repository := newProbeRepositoryStub(Connection{ID: "srv_1"})
	coordinator := NewProbeCoordinator(repository, &serverProbeStub{})

	if err := coordinator.TouchMonitor(context.Background(), time.Second); err != nil {
		t.Fatalf("TouchMonitor returned error: %v", err)
	}
	if !repository.touchedMonitor {
		t.Fatal("monitor activity was not touched")
	}
}
