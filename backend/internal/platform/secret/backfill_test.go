package secret

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestBackfillLegacyCredentialsNilInputsAreNoop(t *testing.T) {
	summary, err := BackfillLegacyCredentials(context.Background(), nil, nil)

	if err != nil {
		t.Fatalf("expected nil inputs to be no-op: %v", err)
	}
	if summary != (BackfillSummary{}) {
		t.Fatalf("summary = %#v, want zero", summary)
	}
}

func TestBackfillProxyPasswordsIsIdempotent(t *testing.T) {
	db := &fakeCredentialDB{
		proxies: []fakeProxyRow{
			{id: "proxy-1", password: "secret"},
			{id: "proxy-2", password: "", passwordSecretID: "sec_existing"},
		},
	}
	store := &fakeSecretWriter{}

	first, err := backfillProxyPasswords(context.Background(), db, store)
	if err != nil {
		t.Fatalf("first backfill returned error: %v", err)
	}
	second, err := backfillProxyPasswords(context.Background(), db, store)
	if err != nil {
		t.Fatalf("second backfill returned error: %v", err)
	}

	if first != 1 || second != 0 {
		t.Fatalf("counts = (%d, %d), want (1, 0)", first, second)
	}
	if len(store.puts) != 1 {
		t.Fatalf("Put calls = %d, want 1", len(store.puts))
	}
	if db.proxies[0].password != "" {
		t.Fatalf("legacy password was not cleared: %#v", db.proxies[0])
	}
	if db.proxies[0].passwordSecretID == "" {
		t.Fatalf("password secret id was not set: %#v", db.proxies[0])
	}
}

func TestBackfillServerCredentialsIsIdempotent(t *testing.T) {
	db := &fakeCredentialDB{
		servers: []fakeServerRow{
			{id: "server-1", password: "password", privateKey: "key"},
			{id: "server-2", password: "", passwordSecretID: "sec_password", privateKey: "key2"},
		},
	}
	store := &fakeSecretWriter{}

	firstPasswords, firstKeys, err := backfillServerCredentials(context.Background(), db, store)
	if err != nil {
		t.Fatalf("first backfill returned error: %v", err)
	}
	secondPasswords, secondKeys, err := backfillServerCredentials(context.Background(), db, store)
	if err != nil {
		t.Fatalf("second backfill returned error: %v", err)
	}

	if firstPasswords != 1 || firstKeys != 2 {
		t.Fatalf("first counts = (%d, %d), want (1, 2)", firstPasswords, firstKeys)
	}
	if secondPasswords != 0 || secondKeys != 0 {
		t.Fatalf("second counts = (%d, %d), want (0, 0)", secondPasswords, secondKeys)
	}
	if len(store.puts) != 3 {
		t.Fatalf("Put calls = %d, want 3", len(store.puts))
	}
	for _, server := range db.servers {
		if server.password != "" || server.privateKey != "" {
			t.Fatalf("legacy credentials were not cleared: %#v", server)
		}
	}
}

func TestBackfillProxyPasswordsRollsBackSecretAndReferenceTogether(t *testing.T) {
	writeErr := errors.New("proxy reference update failed")
	db := &fakeCredentialDB{
		proxies:          []fakeProxyRow{{id: "proxy-1", password: "secret"}},
		failExecContains: "UPDATE proxy_settings",
		failExecErr:      writeErr,
	}
	store := &fakeSecretWriter{}

	_, err := backfillProxyPasswords(context.Background(), db, store)
	if !errors.Is(err, writeErr) {
		t.Fatalf("backfill error = %v, want %v", err, writeErr)
	}
	if db.lastTx == nil || db.lastTx.committed || !db.lastTx.rolledBack {
		t.Fatalf("transaction state = %#v, want rollback without commit", db.lastTx)
	}
	if len(store.puts) != 1 || store.puts[0].tx != db.lastTx {
		t.Fatal("secret write did not use the rolled-back business transaction")
	}
	if db.proxies[0].password != "secret" || db.proxies[0].passwordSecretID != "" {
		t.Fatalf("business row changed after failed update: %#v", db.proxies[0])
	}
}

func TestBackfillTelegramTokensIsIdempotentAndClearsPlaintext(t *testing.T) {
	db := &fakeCredentialDB{
		telegram: []fakeTelegramRow{
			{id: "default", botToken: "bot", inboundToken: "inbound"},
			{id: "existing", botToken: "stale", botTokenSecretID: "sec_existing", inboundToken: "incoming"},
		},
	}
	store := &fakeSecretWriter{}

	firstBotTokens, firstInboundTokens, err := backfillTelegramTokens(context.Background(), db, store)
	if err != nil {
		t.Fatalf("first backfill returned error: %v", err)
	}
	secondBotTokens, secondInboundTokens, err := backfillTelegramTokens(context.Background(), db, store)
	if err != nil {
		t.Fatalf("second backfill returned error: %v", err)
	}

	if firstBotTokens != 1 || firstInboundTokens != 2 {
		t.Fatalf("first counts = (%d, %d), want (1, 2)", firstBotTokens, firstInboundTokens)
	}
	if secondBotTokens != 0 || secondInboundTokens != 0 {
		t.Fatalf("second counts = (%d, %d), want (0, 0)", secondBotTokens, secondInboundTokens)
	}
	if len(store.puts) != 3 {
		t.Fatalf("Put calls = %d, want 3", len(store.puts))
	}
	for _, settings := range db.telegram {
		if settings.botToken != "" || settings.inboundToken != "" {
			t.Fatalf("legacy telegram tokens were not cleared: %#v", settings)
		}
	}
	if db.telegram[1].botTokenSecretID != "sec_existing" {
		t.Fatalf("existing secret reference changed: %#v", db.telegram[1])
	}
}

func TestBackfillWikiResourcePasswordsIsIdempotentAndClearsPlaintext(t *testing.T) {
	db := &fakeCredentialDB{
		wikiResources: []fakeWikiResourceRow{
			{id: "resource-1", password: "password"},
			{id: "resource-2", password: "stale", passwordSecretID: "sec_existing"},
		},
	}
	store := &fakeSecretWriter{}

	first, err := backfillWikiResourcePasswords(context.Background(), db, store)
	if err != nil {
		t.Fatalf("first backfill returned error: %v", err)
	}
	second, err := backfillWikiResourcePasswords(context.Background(), db, store)
	if err != nil {
		t.Fatalf("second backfill returned error: %v", err)
	}

	if first != 1 || second != 0 {
		t.Fatalf("counts = (%d, %d), want (1, 0)", first, second)
	}
	if len(store.puts) != 1 {
		t.Fatalf("Put calls = %d, want 1", len(store.puts))
	}
	for _, resource := range db.wikiResources {
		if resource.password != "" {
			t.Fatalf("legacy wiki password was not cleared: %#v", resource)
		}
	}
	if db.wikiResources[1].passwordSecretID != "sec_existing" {
		t.Fatalf("existing secret reference changed: %#v", db.wikiResources[1])
	}
}

type fakeProxyRow struct {
	id               string
	password         string
	passwordSecretID string
}

type fakeServerRow struct {
	id                 string
	password           string
	passwordSecretID   string
	privateKey         string
	privateKeySecretID string
}

type fakeTelegramRow struct {
	id                   string
	botToken             string
	botTokenSecretID     string
	inboundToken         string
	inboundTokenSecretID string
}

type fakeWikiResourceRow struct {
	id               string
	password         string
	passwordSecretID string
}

type fakeCredentialDB struct {
	proxies          []fakeProxyRow
	servers          []fakeServerRow
	telegram         []fakeTelegramRow
	wikiResources    []fakeWikiResourceRow
	failExecContains string
	failExecErr      error
	lastTx           *fakeCredentialTx
}

func (db *fakeCredentialDB) Begin(context.Context) (pgx.Tx, error) {
	tx := &fakeCredentialTx{db: db}
	db.lastTx = tx
	return tx, nil
}

func (db *fakeCredentialDB) Query(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
	switch {
	case strings.Contains(sql, "FROM proxy_settings"):
		rows := [][]any{}
		for _, proxy := range db.proxies {
			if proxy.password != "" && proxy.passwordSecretID == "" {
				rows = append(rows, []any{proxy.id, proxy.password})
			}
		}
		return &fakeCredentialRows{rows: rows}, nil
	case strings.Contains(sql, "FROM server_connections"):
		rows := [][]any{}
		for _, server := range db.servers {
			password := ""
			privateKey := ""
			if server.password != "" && server.passwordSecretID == "" {
				password = server.password
			}
			if server.privateKey != "" && server.privateKeySecretID == "" {
				privateKey = server.privateKey
			}
			if password != "" || privateKey != "" {
				rows = append(rows, []any{server.id, password, privateKey})
			}
		}
		return &fakeCredentialRows{rows: rows}, nil
	case strings.Contains(sql, "FROM telegram_notification_settings"):
		rows := [][]any{}
		for _, settings := range db.telegram {
			if settings.botToken != "" || settings.inboundToken != "" {
				rows = append(rows, []any{
					settings.id,
					settings.botToken,
					settings.botTokenSecretID,
					settings.inboundToken,
					settings.inboundTokenSecretID,
				})
			}
		}
		return &fakeCredentialRows{rows: rows}, nil
	case strings.Contains(sql, "FROM wiki_page_resources"):
		rows := [][]any{}
		for _, resource := range db.wikiResources {
			if resource.password != "" {
				rows = append(rows, []any{resource.id, resource.password, resource.passwordSecretID})
			}
		}
		return &fakeCredentialRows{rows: rows}, nil
	default:
		return &fakeCredentialRows{}, nil
	}
}

func (db *fakeCredentialDB) Exec(_ context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
	if db.failExecContains != "" && strings.Contains(sql, db.failExecContains) {
		return pgconn.CommandTag{}, db.failExecErr
	}
	switch {
	case strings.Contains(sql, "UPDATE proxy_settings"):
		id := arguments[0].(string)
		secretID := arguments[1].(string)
		for i := range db.proxies {
			if db.proxies[i].id == id && db.proxies[i].passwordSecretID == "" {
				db.proxies[i].password = ""
				db.proxies[i].passwordSecretID = secretID
			}
		}
	case strings.Contains(sql, "UPDATE server_connections"):
		id := arguments[0].(string)
		passwordSecretID := arguments[1].(string)
		privateKeySecretID := arguments[2].(string)
		for i := range db.servers {
			if db.servers[i].id != id {
				continue
			}
			if passwordSecretID != "" {
				db.servers[i].password = ""
				db.servers[i].passwordSecretID = passwordSecretID
			}
			if privateKeySecretID != "" {
				db.servers[i].privateKey = ""
				db.servers[i].privateKeySecretID = privateKeySecretID
			}
		}
	case strings.Contains(sql, "UPDATE telegram_notification_settings"):
		id := arguments[0].(string)
		botTokenSecretID := arguments[1].(string)
		inboundTokenSecretID := arguments[2].(string)
		for i := range db.telegram {
			if db.telegram[i].id != id {
				continue
			}
			if botTokenSecretID != "" {
				db.telegram[i].botToken = ""
				db.telegram[i].botTokenSecretID = botTokenSecretID
			}
			if inboundTokenSecretID != "" {
				db.telegram[i].inboundToken = ""
				db.telegram[i].inboundTokenSecretID = inboundTokenSecretID
			}
		}
	case strings.Contains(sql, "UPDATE wiki_page_resources"):
		id := arguments[0].(string)
		passwordSecretID := arguments[1].(string)
		for i := range db.wikiResources {
			if db.wikiResources[i].id == id {
				db.wikiResources[i].password = ""
				db.wikiResources[i].passwordSecretID = passwordSecretID
			}
		}
	}
	return pgconn.NewCommandTag("UPDATE 1"), nil
}

type fakeSecretWriter struct {
	puts []fakeSecretPut
}

type fakeSecretPut struct {
	scope     string
	name      string
	plaintext string
	tx        pgx.Tx
}

func (w *fakeSecretWriter) Put(_ context.Context, scope string, name string, plaintext string) (string, error) {
	w.puts = append(w.puts, fakeSecretPut{scope: scope, name: name, plaintext: plaintext})
	return "sec_" + strings.ReplaceAll(scope+"_"+name, ":", "_"), nil
}

func (w *fakeSecretWriter) PutTx(ctx context.Context, tx pgx.Tx, scope string, name string, plaintext string) (string, error) {
	id, err := w.Put(ctx, scope, name, plaintext)
	w.puts[len(w.puts)-1].tx = tx
	return id, err
}

type fakeCredentialTx struct {
	pgx.Tx
	db         *fakeCredentialDB
	committed  bool
	rolledBack bool
}

func (tx *fakeCredentialTx) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return tx.db.Query(ctx, sql, args...)
}

func (tx *fakeCredentialTx) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return tx.db.Exec(ctx, sql, args...)
}

func (tx *fakeCredentialTx) Commit(context.Context) error {
	tx.committed = true
	return nil
}

func (tx *fakeCredentialTx) Rollback(context.Context) error {
	tx.rolledBack = true
	return nil
}

type fakeCredentialRows struct {
	rows   [][]any
	index  int
	closed bool
}

func (r *fakeCredentialRows) Close() {
	r.closed = true
}

func (r *fakeCredentialRows) Err() error {
	return nil
}

func (r *fakeCredentialRows) CommandTag() pgconn.CommandTag {
	return pgconn.NewCommandTag("SELECT")
}

func (r *fakeCredentialRows) FieldDescriptions() []pgconn.FieldDescription {
	return nil
}

func (r *fakeCredentialRows) Next() bool {
	if r.index >= len(r.rows) {
		r.closed = true
		return false
	}
	r.index++
	return true
}

func (r *fakeCredentialRows) Scan(dest ...any) error {
	row := r.rows[r.index-1]
	for i, value := range row {
		*(dest[i].(*string)) = value.(string)
	}
	return nil
}

func (r *fakeCredentialRows) Values() ([]any, error) {
	return r.rows[r.index-1], nil
}

func (r *fakeCredentialRows) RawValues() [][]byte {
	return nil
}

func (r *fakeCredentialRows) Conn() *pgx.Conn {
	return nil
}
