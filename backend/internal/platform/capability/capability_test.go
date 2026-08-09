package capability

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5"
)

type fakeRow struct {
	allowed bool
	err     error
}

func (r fakeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	*(dest[0].(*bool)) = r.allowed
	return nil
}

type fakeQueryRower struct {
	row  pgx.Row
	args []any
}

func (q *fakeQueryRower) QueryRow(_ context.Context, _ string, args ...any) pgx.Row {
	q.args = args
	return q.row
}

func TestPostgresAuthorizerAdminAlwaysAllowed(t *testing.T) {
	authorizer := NewPostgresAuthorizer(nil)

	allowed, err := authorizer.Authorize(context.Background(), User{Role: " ADMIN "}, UpdatesApply)
	if err != nil || !allowed {
		t.Fatalf("expected admin to be allowed, allowed=%v err=%v", allowed, err)
	}
}

func TestPostgresAuthorizerReadsRoleGrant(t *testing.T) {
	db := &fakeQueryRower{row: fakeRow{allowed: true}}
	authorizer := NewPostgresAuthorizer(db)

	allowed, err := authorizer.Authorize(context.Background(), User{Role: " Operator "}, JobsManage)
	if err != nil || !allowed {
		t.Fatalf("expected persisted grant to allow capability, allowed=%v err=%v", allowed, err)
	}
	if got := db.args[0]; got != "operator" {
		t.Fatalf("expected normalized role, got %#v", got)
	}
	if got := db.args[1]; got != JobsManage {
		t.Fatalf("expected capability argument, got %#v", got)
	}
}

func TestPostgresAuthorizerFailsClosedOnDatabaseError(t *testing.T) {
	authorizer := NewPostgresAuthorizer(&fakeQueryRower{row: fakeRow{err: errors.New("database unavailable")}})

	allowed, err := authorizer.Authorize(context.Background(), User{Role: "viewer"}, DashboardRead)
	if err == nil || allowed {
		t.Fatalf("expected database error to fail closed, allowed=%v err=%v", allowed, err)
	}
}

type stubAuthorizer struct {
	allowed bool
	err     error
}

func (a stubAuthorizer) Authorize(context.Context, User, Capability) (bool, error) {
	return a.allowed, a.err
}

func TestRequireCapabilityUsesRequestContextAuthorizer(t *testing.T) {
	contextKey := struct{}{}
	resolver := func(ctx context.Context) (User, bool) {
		role, ok := ctx.Value(contextKey).(string)
		return User{Role: role}, ok
	}
	handler := RequireCapability(stubAuthorizer{allowed: true}, resolver, JobsRead)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodGet, "/jobs", nil)
	req = req.WithContext(context.WithValue(req.Context(), contextKey, "viewer"))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, req)

	if response.Code != http.StatusNoContent {
		t.Fatalf("expected request to pass, got status %d", response.Code)
	}
}

func TestRequireCapabilityRejectsAuthorizationFailure(t *testing.T) {
	resolver := func(context.Context) (User, bool) { return User{Role: "viewer"}, true }
	handler := RequireCapability(stubAuthorizer{err: errors.New("database unavailable")}, resolver, JobsRead)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("next handler must not run")
	}))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/jobs", nil))

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", response.Code)
	}
}
