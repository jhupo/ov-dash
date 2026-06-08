package secret

import (
	"context"
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

type fakeCredentialDB struct {
	proxies []fakeProxyRow
	servers []fakeServerRow
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
	default:
		return &fakeCredentialRows{}, nil
	}
}

func (db *fakeCredentialDB) Exec(_ context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
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
}

func (w *fakeSecretWriter) Put(_ context.Context, scope string, name string, plaintext string) (string, error) {
	w.puts = append(w.puts, fakeSecretPut{scope: scope, name: name, plaintext: plaintext})
	return "sec_" + strings.ReplaceAll(scope+"_"+name, ":", "_"), nil
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
