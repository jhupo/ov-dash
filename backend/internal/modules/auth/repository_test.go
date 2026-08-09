package auth

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

type capabilityRepositoryDB struct {
	repositoryDB
	query string
	args  []any
	row   pgx.Row
}

func (db *capabilityRepositoryDB) QueryRow(_ context.Context, query string, args ...any) pgx.Row {
	db.query = query
	db.args = args
	return db.row
}

type capabilityRow struct {
	capabilities []string
}

func (row capabilityRow) Scan(dest ...any) error {
	capabilities := dest[0].(*[]string)
	*capabilities = append(*capabilities, row.capabilities...)
	return nil
}

func TestListCapabilitiesByRoleNormalizesRoleAndRequestsSortedCapabilities(t *testing.T) {
	database := &capabilityRepositoryDB{
		row: capabilityRow{capabilities: []string{"jobs:manage", "servers:ssh"}},
	}
	repository := &Repository{db: database}

	capabilities, err := repository.ListCapabilitiesByRole(context.Background(), " Operator ")
	if err != nil {
		t.Fatalf("list capabilities: %v", err)
	}
	if len(database.args) != 1 || database.args[0] != "operator" {
		t.Fatalf("query args = %#v, want normalized operator role", database.args)
	}
	if !strings.Contains(database.query, "array_agg(capability ORDER BY capability)") {
		t.Fatalf("query does not order capabilities: %s", database.query)
	}
	if len(capabilities) != 2 || capabilities[0] != "jobs:manage" || capabilities[1] != "servers:ssh" {
		t.Fatalf("capabilities = %#v", capabilities)
	}
}
