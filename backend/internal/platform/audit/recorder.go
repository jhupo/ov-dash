package audit

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"ov-dash/backend/internal/db"
	"ov-dash/backend/internal/platform/redact"
)

type Actor struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Role  string `json:"role"`
}

type Entry struct {
	Actor      Actor
	Action     string
	Resource   string
	ResourceID string
	Result     string
	Message    string
	Metadata   map[string]any
	IP         string
	UserAgent  string
}

type Log struct {
	ID         string         `json:"id"`
	Actor      Actor          `json:"actor"`
	Action     string         `json:"action"`
	Resource   string         `json:"resource"`
	ResourceID string         `json:"resource_id"`
	Result     string         `json:"result"`
	Message    string         `json:"message"`
	Metadata   map[string]any `json:"metadata"`
	IP         string         `json:"ip_address"`
	UserAgent  string         `json:"user_agent"`
	CreatedAt  time.Time      `json:"created_at"`
}

type ListFilter struct {
	Resource string
	Action   string
	Result   string
	ActorID  string
	Limit    int
}

type Recorder struct {
	db *db.Pool
}

func NewRecorder(db *db.Pool) *Recorder {
	return &Recorder{db: db}
}

func (r *Recorder) Record(ctx context.Context, entry Entry) error {
	if r == nil {
		return nil
	}
	id, err := randomID()
	if err != nil {
		return err
	}
	metadata, err := json.Marshal(redact.Metadata(entry.Metadata))
	if err != nil {
		return err
	}
	_, err = r.db.Exec(ctx, `
		INSERT INTO audit_logs (
			id, actor_id, actor_email, actor_role, action, resource, resource_id,
			result, message, metadata, ip_address, user_agent
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10::jsonb, $11, $12)
	`, id, entry.Actor.ID, entry.Actor.Email, entry.Actor.Role, entry.Action, entry.Resource, entry.ResourceID,
		entry.Result, entry.Message, string(metadata), entry.IP, entry.UserAgent)
	return err
}

func (r *Recorder) List(ctx context.Context, limit int) ([]Log, error) {
	return r.ListFiltered(ctx, ListFilter{Limit: limit})
}

func (r *Recorder) ListFiltered(ctx context.Context, filter ListFilter) ([]Log, error) {
	if r == nil {
		return []Log{}, nil
	}
	filter = normalizeListFilter(filter)
	rows, err := r.db.Query(ctx, `
		SELECT id, actor_id, actor_email, actor_role, action, resource, resource_id,
		       result, message, metadata, ip_address, user_agent, created_at
		FROM audit_logs
		WHERE ($1 = '' OR resource = $1)
		  AND ($2 = '' OR action = $2)
		  AND ($3 = '' OR result = $3)
		  AND ($4 = '' OR actor_id = $4)
		ORDER BY created_at DESC
		LIMIT $5
	`, filter.Resource, filter.Action, filter.Result, filter.ActorID, filter.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanLogs(rows)
}

func scanLogs(rows logRows) ([]Log, error) {
	items := make([]Log, 0)
	for rows.Next() {
		var item Log
		var metadata []byte
		if err := rows.Scan(
			&item.ID,
			&item.Actor.ID,
			&item.Actor.Email,
			&item.Actor.Role,
			&item.Action,
			&item.Resource,
			&item.ResourceID,
			&item.Result,
			&item.Message,
			&metadata,
			&item.IP,
			&item.UserAgent,
			&item.CreatedAt,
		); err != nil {
			return nil, err
		}
		if len(metadata) > 0 {
			_ = json.Unmarshal(metadata, &item.Metadata)
		}
		if item.Metadata == nil {
			item.Metadata = map[string]any{}
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

type logRows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}

func normalizeListFilter(filter ListFilter) ListFilter {
	filter.Resource = strings.TrimSpace(filter.Resource)
	filter.Action = strings.TrimSpace(filter.Action)
	filter.Result = strings.TrimSpace(filter.Result)
	filter.ActorID = strings.TrimSpace(filter.ActorID)
	if filter.Limit < 1 || filter.Limit > 200 {
		filter.Limit = 50
	}
	return filter
}

func FilterFromQuery(r *http.Request) ListFilter {
	filter := ListFilter{
		Resource: r.URL.Query().Get("resource"),
		Action:   r.URL.Query().Get("action"),
		Result:   r.URL.Query().Get("result"),
		ActorID:  r.URL.Query().Get("actor_id"),
		Limit:    LimitFromQuery(r),
	}
	return normalizeListFilter(filter)
}

func LimitFromQuery(r *http.Request) int {
	limit := 50
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil {
			limit = value
		}
	}
	return limit
}

func RequestIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err != nil {
		return strings.TrimSpace(r.RemoteAddr)
	}
	return host
}

func randomID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
