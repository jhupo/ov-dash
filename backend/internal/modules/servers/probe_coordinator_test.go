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
	return r.monitorActive, nil
}

type serverProbeStub struct {
	installErr     error
	collectErr     error
	collectOnceErr error
	metric         Metric
	onceMetric     Metric
	status         AgentStatus
	statusErr      error
}

func (p *serverProbeStub) Install(ctx context.Context, item Connection) error {
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

func (p *serverProbeStub) Dial(ctx context.Context, item Connection) (net.Conn, error) {
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
	probe := &serverProbeStub{installErr: errors.New("install failed"), collectOnceErr: errors.New("once failed"), statusErr: errors.New("status unavailable")}
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
		installErr:     errors.New("install failed"),
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
