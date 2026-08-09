package queue

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"ov-dash/backend/internal/events"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	riverqueue "github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

func TestEnvelopeStrictValidation(t *testing.T) {
	job, err := NewJob("noop", map[string]any{"ok": true})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(Envelope{Version: EnvelopeVersion, Job: job})
	if err != nil {
		t.Fatal(err)
	}
	var envelope Envelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("decode valid envelope: %v", err)
	}
	if envelope.Job.ID != job.ID {
		t.Fatalf("job ID = %q, want %q", envelope.Job.ID, job.ID)
	}

	invalid := strings.Replace(string(raw), `"version":1`, `"version":1,"unexpected":true`, 1)
	if err := json.Unmarshal([]byte(invalid), &envelope); err == nil {
		t.Fatal("expected unknown envelope field to be rejected")
	}
}

func TestEnvelopeRejectsUnsupportedVersion(t *testing.T) {
	job, err := NewJob("noop", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if err := (Envelope{Version: 2, Job: job}).Validate(); err == nil {
		t.Fatal("expected unsupported version to be rejected")
	}
}

func TestNewRequeuedJobUsesNewIDAndPreservesDefinition(t *testing.T) {
	record := JobRecord{
		ID:          "old-job",
		Type:        "noop",
		Payload:     map[string]any{"ok": true},
		Status:      JobStatusDead,
		MaxAttempts: 7,
	}
	job, err := newRequeuedJob(record, "new-key")
	if err != nil {
		t.Fatal(err)
	}
	if job.ID == "" || job.ID == record.ID {
		t.Fatalf("requeued job ID = %q", job.ID)
	}
	if job.MaxAttempts != 7 || job.IdempotencyKey != "new-key" {
		t.Fatalf("requeued job = %#v", job)
	}
}

func TestRetryBackoff(t *testing.T) {
	tests := map[int]time.Duration{
		0: 5 * time.Second,
		1: 5 * time.Second,
		2: 10 * time.Second,
		3: 20 * time.Second,
		5: time.Minute,
	}
	for attempt, want := range tests {
		if got := RetryBackoff(attempt); got != want {
			t.Fatalf("RetryBackoff(%d) = %s, want %s", attempt, got, want)
		}
	}
}

func TestNormalizeReleaseID(t *testing.T) {
	tests := map[string]string{
		"":                    "",
		"  v1.2.3  ":          "ov-dash-1.2.3",
		"ov-dash-1.2.3":       "ov-dash-1.2.3",
		"ov-dash-v1.2.3-rc.1": "ov-dash-1.2.3-rc.1",
		"local":               "ov-dash-local",
	}
	for input, want := range tests {
		if got := NormalizeReleaseID(input); got != want {
			t.Fatalf("NormalizeReleaseID(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestRunWorkerClientSignalsOnlyAfterStartSucceeds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	client := &fakeWorkerClient{}
	started := 0

	err := runWorkerClient(ctx, WorkerOptions{
		ShutdownTimeout: time.Second,
		Started: func() {
			started++
			cancel()
		},
	}, client)
	if err != nil {
		t.Fatalf("run worker client: %v", err)
	}
	if started != 1 {
		t.Fatalf("started callbacks = %d, want 1", started)
	}
	if client.startCalls != 1 || client.stopCalls != 1 || client.cancelCalls != 0 {
		t.Fatalf("worker calls = start:%d stop:%d cancel:%d", client.startCalls, client.stopCalls, client.cancelCalls)
	}
}

func TestRunWorkerClientDoesNotSignalWhenStartFails(t *testing.T) {
	client := &fakeWorkerClient{startErr: errors.New("river unavailable")}
	started := 0

	err := runWorkerClient(context.Background(), WorkerOptions{
		ShutdownTimeout: time.Second,
		Started:         func() { started++ },
	}, client)
	if err == nil || !strings.Contains(err.Error(), "start River worker client") {
		t.Fatalf("error = %v, want start failure", err)
	}
	if started != 0 {
		t.Fatalf("started callbacks = %d, want 0", started)
	}
	if client.stopCalls != 0 || client.cancelCalls != 0 {
		t.Fatalf("failed start must not stop worker: stop:%d cancel:%d", client.stopCalls, client.cancelCalls)
	}
}

func TestWorkerHeartbeatPersistsNormalizedReleaseID(t *testing.T) {
	database := &fakeHeartbeatAuditDB{}
	store := &PostgresAuditStore{db: database}

	if err := store.RecordWorkerHeartbeat(context.Background(), "worker-1", "jobs:test", " v1.2.3 ", "host-1", 42); err != nil {
		t.Fatalf("record heartbeat: %v", err)
	}
	if !strings.Contains(database.execSQL, "release_id") {
		t.Fatalf("heartbeat SQL does not persist release_id: %s", database.execSQL)
	}
	if got := database.execArgs[2]; got != "ov-dash-1.2.3" {
		t.Fatalf("release argument = %#v, want ov-dash-1.2.3", got)
	}
}

func TestWorkerHeartbeatRejectsEmptyReleaseID(t *testing.T) {
	database := &fakeHeartbeatAuditDB{}
	store := &PostgresAuditStore{db: database}

	err := store.RecordWorkerHeartbeat(context.Background(), "worker-1", "jobs:test", " ", "host-1", 42)
	if err == nil || err.Error() != "worker heartbeat release ID is required" {
		t.Fatalf("error = %v", err)
	}
	if database.execSQL != "" {
		t.Fatal("empty release ID must not write heartbeat")
	}
}

func TestListWorkerHeartbeatsScansReleaseID(t *testing.T) {
	now := time.Now().UTC()
	database := &fakeHeartbeatAuditDB{rows: &fakeHeartbeatRows{items: []WorkerHeartbeat{{
		ID: "worker-1", QueueName: "jobs:test", ReleaseID: "ov-dash-1.2.3",
		Hostname: "host-1", PID: 42, StartedAt: now.Add(-time.Minute), LastSeenAt: now,
	}}}}
	store := &PostgresAuditStore{db: database}

	items, err := store.ListWorkerHeartbeats(context.Background(), 30*time.Second)
	if err != nil {
		t.Fatalf("list heartbeat: %v", err)
	}
	if len(items) != 1 || items[0].ReleaseID != "ov-dash-1.2.3" {
		t.Fatalf("heartbeats = %#v", items)
	}
	if !strings.Contains(database.querySQL, "release_id") {
		t.Fatalf("heartbeat query does not select release_id: %s", database.querySQL)
	}
}

func TestEnqueueCommitsAuditRiverAndOutboxTogether(t *testing.T) {
	tx := &lifecycleTx{}
	riverClient := &fakeRiverTransactions{
		insertResult: &rivertype.JobInsertResult{Job: &rivertype.JobRow{ID: 41}},
	}
	client := newLifecycleClient(tx, riverClient)
	job, err := NewJob("noop", map[string]any{"ok": true})
	if err != nil {
		t.Fatal(err)
	}

	if err := client.Enqueue(context.Background(), "jobs:test", job); err != nil {
		t.Fatalf("enqueue job: %v", err)
	}
	if !tx.committed || tx.rolledBack {
		t.Fatalf("transaction state = committed %t, rolled back %t", tx.committed, tx.rolledBack)
	}
	if riverClient.insertCalls != 1 {
		t.Fatalf("River inserts = %d, want 1", riverClient.insertCalls)
	}
	if got := tx.outboxEventTypes(); len(got) != 1 || got[0] != "job.enqueued" {
		t.Fatalf("outbox events = %#v, want job.enqueued", got)
	}
	if !tx.hasSQL("INSERT INTO jobs_audit") || !tx.hasSQL("river_job_id = $2") {
		t.Fatalf("enqueue did not write audit and River link in transaction: %#v", tx.execCalls)
	}
}

func TestEnqueueRollsBackWhenOutboxAppendFails(t *testing.T) {
	tx := &lifecycleTx{failOutboxType: "job.enqueued"}
	riverClient := &fakeRiverTransactions{
		insertResult: &rivertype.JobInsertResult{Job: &rivertype.JobRow{ID: 42}},
	}
	client := newLifecycleClient(tx, riverClient)
	job, err := NewJob("noop", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}

	err = client.Enqueue(context.Background(), "jobs:test", job)
	if err == nil || !strings.Contains(err.Error(), "append enqueue outbox") {
		t.Fatalf("error = %v, want outbox append failure", err)
	}
	if tx.committed || !tx.rolledBack {
		t.Fatalf("transaction state = committed %t, rolled back %t", tx.committed, tx.rolledBack)
	}
	if riverClient.insertCalls != 1 {
		t.Fatalf("River inserts = %d, want insertion before transactional rollback", riverClient.insertCalls)
	}
}

func TestCompletedStatusAndOutboxShareTransaction(t *testing.T) {
	tx := &lifecycleTx{}
	store := &PostgresAuditStore{
		begin:  func(context.Context) (pgx.Tx, error) { return tx, nil },
		outbox: events.NewOutbox(nil),
	}
	job := Job{ID: strings.Repeat("a", 32), Type: "noop", Payload: map[string]any{}, Attempts: 1, MaxAttempts: 3}

	if err := store.RecordJobCompleted(context.Background(), "jobs:test", job); err != nil {
		t.Fatalf("record completed job: %v", err)
	}
	if !tx.committed || tx.rolledBack {
		t.Fatalf("transaction state = committed %t, rolled back %t", tx.committed, tx.rolledBack)
	}
	if got := tx.outboxEventTypes(); len(got) != 1 || got[0] != "job.completed" {
		t.Fatalf("outbox events = %#v, want job.completed", got)
	}
}

func TestCompletedStatusRollsBackWhenOutboxAppendFails(t *testing.T) {
	tx := &lifecycleTx{failOutboxType: "job.completed"}
	store := &PostgresAuditStore{
		begin:  func(context.Context) (pgx.Tx, error) { return tx, nil },
		outbox: events.NewOutbox(nil),
	}
	job := Job{ID: strings.Repeat("b", 32), Type: "noop", Payload: map[string]any{}, Attempts: 1, MaxAttempts: 3}

	if err := store.RecordJobCompleted(context.Background(), "jobs:test", job); err == nil {
		t.Fatal("expected outbox append failure")
	}
	if tx.committed || !tx.rolledBack {
		t.Fatalf("transaction state = committed %t, rolled back %t", tx.committed, tx.rolledBack)
	}
}

func TestLifecycleTransitionsAppendExpectedOutboxEvents(t *testing.T) {
	job := Job{ID: strings.Repeat("e", 32), Type: "noop", Payload: map[string]any{}, Attempts: 2, MaxAttempts: 3}
	retryAt := time.Now().UTC().Add(time.Minute)
	tests := []struct {
		name string
		want []string
		run  func(*PostgresAuditStore) error
	}{
		{
			name: "started",
			want: []string{"job.started"},
			run: func(store *PostgresAuditStore) error {
				return store.RecordJobRunning(context.Background(), "jobs:test", job)
			},
		},
		{
			name: "failed",
			want: []string{"job.failed"},
			run: func(store *PostgresAuditStore) error {
				return store.RecordJobFailed(context.Background(), "jobs:test", job, "handler failed", &retryAt)
			},
		},
		{
			name: "dead",
			want: []string{"job.failed", "job.dead"},
			run: func(store *PostgresAuditStore) error {
				return store.RecordJobDead(context.Background(), "jobs:test", job, "handler failed")
			},
		},
		{
			name: "canceled",
			want: []string{"job.canceled"},
			run: func(store *PostgresAuditStore) error {
				return store.RecordJobCanceled(context.Background(), "jobs:test", job, "cancel requested")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx := &lifecycleTx{}
			store := &PostgresAuditStore{
				begin:  func(context.Context) (pgx.Tx, error) { return tx, nil },
				outbox: events.NewOutbox(nil),
			}
			if err := tt.run(store); err != nil {
				t.Fatalf("record transition: %v", err)
			}
			if !tx.committed || tx.rolledBack {
				t.Fatalf("transaction state = committed %t, rolled back %t", tx.committed, tx.rolledBack)
			}
			if got := tx.outboxEventTypes(); !equalStrings(got, tt.want) {
				t.Fatalf("outbox events = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestLifecycleStateConflictsReturnExplicitError(t *testing.T) {
	job := Job{ID: strings.Repeat("f", 32), Type: "noop", Payload: map[string]any{}, Attempts: 1, MaxAttempts: 3}
	tests := []struct {
		name string
		run  func(*PostgresAuditStore) error
	}{
		{name: "running", run: func(store *PostgresAuditStore) error {
			return store.RecordJobRunning(context.Background(), "jobs:test", job)
		}},
		{name: "completed", run: func(store *PostgresAuditStore) error {
			return store.RecordJobCompleted(context.Background(), "jobs:test", job)
		}},
		{name: "failed", run: func(store *PostgresAuditStore) error {
			return store.RecordJobFailed(context.Background(), "jobs:test", job, "failed", nil)
		}},
		{name: "dead", run: func(store *PostgresAuditStore) error {
			return store.RecordJobDead(context.Background(), "jobs:test", job, "dead")
		}},
		{name: "canceled", run: func(store *PostgresAuditStore) error {
			return store.RecordJobCanceled(context.Background(), "jobs:test", job, "canceled")
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx := &lifecycleTx{zeroTransitionRows: true}
			store := &PostgresAuditStore{
				begin:  func(context.Context) (pgx.Tx, error) { return tx, nil },
				outbox: events.NewOutbox(nil),
			}

			err := tt.run(store)
			if !errors.Is(err, ErrJobStateConflict) {
				t.Fatalf("error = %v, want ErrJobStateConflict", err)
			}
			if tx.committed || !tx.rolledBack {
				t.Fatalf("transaction state = committed %t, rolled back %t", tx.committed, tx.rolledBack)
			}
			if got := tx.outboxEventTypes(); len(got) != 0 {
				t.Fatalf("outbox events = %#v, want none", got)
			}
		})
	}
}

func TestCancelCommitsRequestedAndImmediateCanceledEvents(t *testing.T) {
	tx := &lifecycleTx{row: scanRow(func(dest ...any) error {
		*(dest[0].(*int64)) = 51
		*(dest[1].(*string)) = "noop"
		*(dest[2].(*string)) = "jobs:test"
		return nil
	})}
	riverClient := &fakeRiverTransactions{
		cancelResult: &rivertype.JobRow{ID: 51, State: rivertype.JobStateCancelled},
	}
	client := newLifecycleClient(tx, riverClient)

	if err := client.RequestJobCancel(context.Background(), strings.Repeat("c", 32)); err != nil {
		t.Fatalf("cancel job: %v", err)
	}
	if !tx.committed || tx.rolledBack {
		t.Fatalf("transaction state = committed %t, rolled back %t", tx.committed, tx.rolledBack)
	}
	want := []string{"job.cancel_requested", "job.canceled"}
	if got := tx.outboxEventTypes(); !equalStrings(got, want) {
		t.Fatalf("outbox events = %#v, want %#v", got, want)
	}
	if !tx.hasSQL("'cancel_requested'") || !tx.hasSQL("'canceled'") {
		t.Fatalf("cancel audit events missing: %#v", tx.execCalls)
	}
}

func TestRequeueCommitsNewEnqueueAndRequeuedEvents(t *testing.T) {
	now := time.Now().UTC()
	tx := &lifecycleTx{row: jobRecordRow(JobRecord{
		ID:          strings.Repeat("d", 32),
		QueueName:   "jobs:test",
		Type:        "noop",
		Payload:     map[string]any{"ok": true},
		Status:      JobStatusDead,
		Attempts:    3,
		MaxAttempts: 3,
		CreatedAt:   now,
		UpdatedAt:   now,
		RiverJobID:  61,
	})}
	riverClient := &fakeRiverTransactions{
		insertResult: &rivertype.JobInsertResult{Job: &rivertype.JobRow{ID: 62}},
	}
	client := newLifecycleClient(tx, riverClient)

	job, err := client.RequeueJob(context.Background(), "jobs:test", strings.Repeat("d", 32), "retry-once")
	if err != nil {
		t.Fatalf("requeue job: %v", err)
	}
	if job.ID == "" || !tx.committed || tx.rolledBack {
		t.Fatalf("job = %#v, transaction state = committed %t, rolled back %t", job, tx.committed, tx.rolledBack)
	}
	want := []string{"job.enqueued", "job.requeued"}
	if got := tx.outboxEventTypes(); !equalStrings(got, want) {
		t.Fatalf("outbox events = %#v, want %#v", got, want)
	}
	if !tx.hasSQL("job requeued from previous job") || !tx.hasSQL("INSERT INTO job_logs") {
		t.Fatalf("requeue audit events or logs missing: %#v", tx.execCalls)
	}
}

func TestRequeueCancelsFailedRiverJobBeforeCreatingReplacement(t *testing.T) {
	now := time.Now().UTC()
	tx := &lifecycleTx{row: jobRecordRow(JobRecord{
		ID:          strings.Repeat("e", 32),
		QueueName:   "jobs:test",
		Type:        "noop",
		Payload:     map[string]any{"ok": true},
		Status:      JobStatusFailed,
		MaxAttempts: 3,
		CreatedAt:   now,
		UpdatedAt:   now,
		RiverJobID:  71,
	})}
	riverClient := &fakeRiverTransactions{
		insertResult: &rivertype.JobInsertResult{Job: &rivertype.JobRow{ID: 72}},
		cancelResult: &rivertype.JobRow{ID: 71, State: rivertype.JobStateCancelled},
	}
	client := newLifecycleClient(tx, riverClient)

	if _, err := client.RequeueJob(context.Background(), "jobs:test", strings.Repeat("e", 32), "replacement"); err != nil {
		t.Fatalf("requeue failed job: %v", err)
	}
	if riverClient.cancelCalls != 1 || riverClient.canceledJobID != 71 {
		t.Fatalf("River cancellation = calls %d job %d", riverClient.cancelCalls, riverClient.canceledJobID)
	}
	if riverClient.insertCalls != 1 || !tx.committed {
		t.Fatalf("replacement insert calls = %d, committed = %t", riverClient.insertCalls, tx.committed)
	}
}

type lifecycleExecCall struct {
	sql  string
	args []any
}

type lifecycleTx struct {
	pgx.Tx
	row                pgx.Row
	execCalls          []lifecycleExecCall
	committed          bool
	rolledBack         bool
	failOutboxType     string
	zeroTransitionRows bool
}

func (tx *lifecycleTx) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	tx.execCalls = append(tx.execCalls, lifecycleExecCall{sql: sql, args: append([]any(nil), args...)})
	if strings.Contains(sql, "INSERT INTO event_outbox") && len(args) > 1 && args[1] == tx.failOutboxType {
		return pgconn.CommandTag{}, errors.New("outbox unavailable")
	}
	if tx.zeroTransitionRows && !strings.Contains(sql, "INSERT INTO event_outbox") {
		return pgconn.NewCommandTag("INSERT 0 0"), nil
	}
	return pgconn.NewCommandTag("INSERT 0 1"), nil
}

func (tx *lifecycleTx) QueryRow(context.Context, string, ...any) pgx.Row {
	return tx.row
}

func (tx *lifecycleTx) Commit(context.Context) error {
	tx.committed = true
	return nil
}

func (tx *lifecycleTx) Rollback(context.Context) error {
	if !tx.committed {
		tx.rolledBack = true
	}
	return nil
}

func (tx *lifecycleTx) outboxEventTypes() []string {
	types := []string{}
	for _, call := range tx.execCalls {
		if strings.Contains(call.sql, "INSERT INTO event_outbox") {
			types = append(types, call.args[1].(string))
		}
	}
	return types
}

func (tx *lifecycleTx) hasSQL(fragment string) bool {
	for _, call := range tx.execCalls {
		if strings.Contains(call.sql, fragment) {
			return true
		}
	}
	return false
}

type fakeRiverTransactions struct {
	insertResult  *rivertype.JobInsertResult
	insertErr     error
	insertCalls   int
	cancelResult  *rivertype.JobRow
	cancelErr     error
	cancelCalls   int
	canceledJobID int64
}

type fakeWorkerClient struct {
	startErr    error
	stopErr     error
	cancelErr   error
	startCalls  int
	stopCalls   int
	cancelCalls int
}

func (c *fakeWorkerClient) Start(context.Context) error {
	c.startCalls++
	return c.startErr
}

func (c *fakeWorkerClient) Stop(context.Context) error {
	c.stopCalls++
	return c.stopErr
}

func (c *fakeWorkerClient) StopAndCancel(context.Context) error {
	c.cancelCalls++
	return c.cancelErr
}

type fakeHeartbeatAuditDB struct {
	execSQL   string
	execArgs  []any
	querySQL  string
	queryArgs []any
	rows      pgx.Rows
}

func (db *fakeHeartbeatAuditDB) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	db.execSQL = sql
	db.execArgs = append([]any(nil), args...)
	return pgconn.NewCommandTag("INSERT 0 1"), nil
}

func (db *fakeHeartbeatAuditDB) Query(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
	db.querySQL = sql
	db.queryArgs = append([]any(nil), args...)
	return db.rows, nil
}

func (db *fakeHeartbeatAuditDB) QueryRow(context.Context, string, ...any) pgx.Row {
	return scanRow(func(...any) error { return pgx.ErrNoRows })
}

type fakeHeartbeatRows struct {
	items []WorkerHeartbeat
	index int
}

func (r *fakeHeartbeatRows) Close()                                       {}
func (r *fakeHeartbeatRows) Err() error                                   { return nil }
func (r *fakeHeartbeatRows) CommandTag() pgconn.CommandTag                { return pgconn.NewCommandTag("SELECT") }
func (r *fakeHeartbeatRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *fakeHeartbeatRows) Next() bool {
	if r.index >= len(r.items) {
		return false
	}
	r.index++
	return true
}
func (r *fakeHeartbeatRows) Scan(dest ...any) error {
	item := r.items[r.index-1]
	*(dest[0].(*string)) = item.ID
	*(dest[1].(*string)) = item.QueueName
	*(dest[2].(*string)) = item.ReleaseID
	*(dest[3].(*string)) = item.Hostname
	*(dest[4].(*int)) = item.PID
	*(dest[5].(*time.Time)) = item.StartedAt
	*(dest[6].(*time.Time)) = item.LastSeenAt
	return nil
}
func (r *fakeHeartbeatRows) Values() ([]any, error) { return nil, nil }
func (r *fakeHeartbeatRows) RawValues() [][]byte    { return nil }
func (r *fakeHeartbeatRows) Conn() *pgx.Conn        { return nil }

func (r *fakeRiverTransactions) InsertTx(context.Context, pgx.Tx, riverqueue.JobArgs, *riverqueue.InsertOpts) (*rivertype.JobInsertResult, error) {
	r.insertCalls++
	return r.insertResult, r.insertErr
}

func (r *fakeRiverTransactions) JobCancelTx(_ context.Context, _ pgx.Tx, id int64) (*rivertype.JobRow, error) {
	r.cancelCalls++
	r.canceledJobID = id
	return r.cancelResult, r.cancelErr
}

func newLifecycleClient(tx *lifecycleTx, riverClient riverTransactionClient) *Client {
	return &Client{
		begin: func(context.Context) (pgx.Tx, error) { return tx, nil },
		river: riverClient,
		audit: &PostgresAuditStore{outbox: events.NewOutbox(nil)},
	}
}

type scanRow func(...any) error

func (row scanRow) Scan(dest ...any) error {
	return row(dest...)
}

func jobRecordRow(record JobRecord) pgx.Row {
	payload, _ := json.Marshal(record.Payload)
	return scanRow(func(dest ...any) error {
		*(dest[0].(*string)) = record.ID
		*(dest[1].(*string)) = record.QueueName
		*(dest[2].(*string)) = record.Type
		*(dest[3].(*[]byte)) = payload
		*(dest[4].(*string)) = record.IdempotencyKey
		*(dest[5].(*string)) = record.Status
		*(dest[6].(*int)) = record.Attempts
		*(dest[7].(*int)) = record.MaxAttempts
		*(dest[8].(*string)) = record.LastError
		*(dest[9].(*bool)) = record.CancelRequested
		*(dest[16].(*time.Time)) = record.CreatedAt
		*(dest[17].(*time.Time)) = record.UpdatedAt
		*(dest[18].(*int64)) = record.RiverJobID
		return nil
	})
}

func equalStrings(got []string, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
