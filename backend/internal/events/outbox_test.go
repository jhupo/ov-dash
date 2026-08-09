package events

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.uber.org/zap"
)

type outboxRow struct {
	value []byte
	err   error
}

func (r outboxRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	*(dest[0].(*[]byte)) = append([]byte(nil), r.value...)
	return nil
}

type execCall struct {
	sql  string
	args []any
}

type fakeOutboxDB struct {
	row       pgx.Row
	rows      int64
	execErr   error
	querySQL  string
	queryArgs []any
	execCalls []execCall
}

func (db *fakeOutboxDB) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	db.querySQL = sql
	db.queryArgs = append([]any(nil), args...)
	return db.row
}

func (db *fakeOutboxDB) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	db.execCalls = append(db.execCalls, execCall{sql: sql, args: append([]any(nil), args...)})
	if db.rows == 1 {
		return pgconn.NewCommandTag("UPDATE 1"), db.execErr
	}
	return pgconn.NewCommandTag("UPDATE 0"), db.execErr
}

type fakeTransaction struct {
	call execCall
	err  error
}

func (*fakeTransaction) Commit(context.Context) error {
	return nil
}

func (*fakeTransaction) Rollback(context.Context) error {
	return nil
}

func (tx *fakeTransaction) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	tx.call = execCall{sql: sql, args: append([]any(nil), args...)}
	return pgconn.NewCommandTag("INSERT 0 1"), tx.err
}

func TestAppendRedactsPayloadInsideCallerTransaction(t *testing.T) {
	tx := &fakeTransaction{}
	outbox := NewOutbox(nil)
	event := New("user.updated", "test", map[string]any{
		"password": "plain-secret",
		"nested": map[string]string{
			"api_token": "token-secret",
			"visible":   "kept",
		},
	})

	if err := outbox.Append(context.Background(), tx, event); err != nil {
		t.Fatalf("append event: %v", err)
	}
	payload := string(tx.call.args[3].([]byte))
	for _, secret := range []string{"plain-secret", "token-secret"} {
		if strings.Contains(payload, secret) {
			t.Fatalf("expected %q to be redacted from %s", secret, payload)
		}
	}
	if !strings.Contains(payload, `"visible":"kept"`) {
		t.Fatalf("expected public payload field to be preserved, got %s", payload)
	}
}

func TestClaimUsesSkipLockedAndExpiredLeases(t *testing.T) {
	leaseExpiresAt := time.Now().UTC().Add(time.Minute).Format(time.RFC3339Nano)
	db := &fakeOutboxDB{row: outboxRow{value: []byte(`[{"id":"event-1","type":"user.updated","source":"test","payload":{"ok":true},"created_at":"2026-08-09T00:00:00Z","attempts":2,"max_attempts":10,"lease_owner":"worker-1","lease_expires_at":"` + leaseExpiresAt + `"}]`)}}
	outbox := NewOutbox(db)

	records, err := outbox.Claim(context.Background(), "worker-1", 25, 20*time.Second)
	if err != nil {
		t.Fatalf("claim events: %v", err)
	}
	if len(records) != 1 || records[0].ID != "event-1" || records[0].Attempts != 2 {
		t.Fatalf("unexpected records: %#v", records)
	}
	for _, fragment := range []string{"FOR UPDATE SKIP LOCKED", "lease_expires_at <= now()", "attempts = outbox.attempts + 1"} {
		if !strings.Contains(db.querySQL, fragment) {
			t.Fatalf("claim query must contain %q", fragment)
		}
	}
	if db.queryArgs[0] != "worker-1" || db.queryArgs[1] != 25 || db.queryArgs[2] != int64(20000) {
		t.Fatalf("unexpected claim arguments: %#v", db.queryArgs)
	}
}

func TestAckRejectsLostLease(t *testing.T) {
	db := &fakeOutboxDB{rows: 0}
	err := NewOutbox(db).Ack(context.Background(), "event-1", "worker-1")
	if !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("expected ErrLeaseLost, got %v", err)
	}
}

func TestFailSchedulesBackoffAndRedactsError(t *testing.T) {
	db := &fakeOutboxDB{rows: 1}
	record := OutboxRecord{Event: Event{ID: "event-1"}, Attempts: 3, MaxAttempts: 10}

	if err := NewOutbox(db).Fail(context.Background(), record, "worker-1", errors.New("password=do-not-store")); err != nil {
		t.Fatalf("fail event: %v", err)
	}
	call := db.execCalls[0]
	if strings.Contains(call.args[2].(string), "do-not-store") {
		t.Fatalf("expected failure message to be redacted, got %q", call.args[2])
	}
	if got := call.args[3]; got != int64(4000) {
		t.Fatalf("expected exponential backoff of 4000ms, got %#v", got)
	}
}

func TestBusReturnsHandlerFailures(t *testing.T) {
	bus := NewBus(zap.NewNop())
	bus.Subscribe("user.updated", func(context.Context, Event) error {
		return errors.New("handler failed")
	})

	if err := bus.Publish(context.Background(), New("user.updated", "test", nil)); err == nil {
		t.Fatal("expected publish error")
	}
}

type fakePublisher struct {
	published []Event
	err       error
}

func (p *fakePublisher) Publish(_ context.Context, event Event) error {
	p.published = append(p.published, event)
	return p.err
}

func TestDispatcherPublishesAndAcknowledgesClaimedEvents(t *testing.T) {
	db := &fakeOutboxDB{
		row:  outboxRow{value: []byte(`[{"id":"event-1","type":"user.updated","source":"test","payload":{},"created_at":"2026-08-09T00:00:00Z","attempts":1,"max_attempts":10,"lease_owner":"worker-1","lease_expires_at":"2026-08-09T00:01:00Z"}]`)},
		rows: 1,
	}
	publisher := &fakePublisher{}
	dispatcher, err := NewDispatcher(NewOutbox(db), publisher, "worker-1")
	if err != nil {
		t.Fatalf("create dispatcher: %v", err)
	}

	result, err := dispatcher.DispatchBatch(context.Background())
	if err != nil {
		t.Fatalf("dispatch batch: %v", err)
	}
	if result.Claimed != 1 || result.Acked != 1 || result.Failed != 0 {
		t.Fatalf("unexpected dispatch result: %#v", result)
	}
	if len(publisher.published) != 1 || publisher.published[0].ID != "event-1" {
		t.Fatalf("unexpected published events: %#v", publisher.published)
	}
	if len(db.execCalls) != 1 || !strings.Contains(db.execCalls[0].sql, "delivered_at = now()") {
		t.Fatalf("expected event acknowledgement, calls=%#v", db.execCalls)
	}
}
