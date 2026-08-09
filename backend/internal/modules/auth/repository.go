package auth

import (
	"context"
	"strings"
	"time"

	"ov-dash/backend/internal/db"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type repositoryDB interface {
	Begin(context.Context) (pgx.Tx, error)
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type Repository struct {
	db repositoryDB
}

func NewRepository(db *db.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) CreateInitialAdmin(ctx context.Context, user User, passwordHash string) (User, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(6363710373251581815)`); err != nil {
		return User{}, err
	}

	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE role = 'admin')`).Scan(&exists); err != nil {
		return User{}, err
	}
	if exists {
		return User{}, ErrAdminAlreadyExists
	}

	row := tx.QueryRow(ctx, `
		INSERT INTO users (
			id, first_name, last_name, username, email, phone_number, status, role, password_hash
		)
		VALUES ($1, $2, $3, $4, $5, '', 'active', 'admin', $6)
		RETURNING id, first_name, last_name, username, email, phone_number, status, role,
		          last_login_at, created_at, updated_at
	`, user.ID, user.FirstName, user.LastName, user.Username, user.Email, passwordHash)
	created, err := scanUser(row)
	if err != nil {
		return User{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return User{}, err
	}
	return created, nil
}

func (r *Repository) FindUserByEmail(ctx context.Context, email string) (userWithPassword, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, first_name, last_name, username, email, phone_number, status, role,
		       password_hash, last_login_at, created_at, updated_at
		FROM users
		WHERE lower(email) = lower($1)
	`, email)
	return scanUserWithPassword(row)
}

func (r *Repository) FindUserByID(ctx context.Context, id string) (User, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, first_name, last_name, username, email, phone_number, status, role,
		       last_login_at, created_at, updated_at
		FROM users
		WHERE id = $1
	`, id)
	return scanUser(row)
}

func (r *Repository) CreateSession(ctx context.Context, session Session, userAgent string, ipAddress string) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO user_sessions (id, user_id, token_hash, user_agent, ip_address, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, session.ID, session.UserID, session.TokenHash, userAgent, ipAddress, session.ExpiresAt)
	return err
}

func (r *Repository) FindSessionUser(ctx context.Context, tokenHash string, now time.Time) (User, error) {
	row := r.db.QueryRow(ctx, `
		SELECT u.id, u.first_name, u.last_name, u.username, u.email, u.phone_number,
		       u.status, u.role, u.last_login_at, u.created_at, u.updated_at
		FROM user_sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = $1
		  AND s.revoked_at IS NULL
		  AND s.expires_at > $2
		  AND u.status = 'active'
	`, tokenHash, now)
	return scanUser(row)
}

func (r *Repository) ListCapabilitiesByRole(ctx context.Context, role string) ([]string, error) {
	capabilities := make([]string, 0)
	err := r.db.QueryRow(ctx, `
		SELECT COALESCE(array_agg(capability ORDER BY capability), ARRAY[]::text[])
		FROM role_capabilities
		WHERE role = $1
	`, normalizeRole(role)).Scan(&capabilities)
	return capabilities, err
}

func (r *Repository) RevokeSession(ctx context.Context, tokenHash string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE user_sessions
		SET revoked_at = now(), updated_at = now()
		WHERE token_hash = $1 AND revoked_at IS NULL
	`, tokenHash)
	return err
}

func (r *Repository) UpdatePassword(ctx context.Context, userID string, passwordHash string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE users
		SET password_hash = $2, updated_at = now()
		WHERE id = $1
	`, userID, passwordHash)
	return err
}

func (r *Repository) TouchLastLogin(ctx context.Context, userID string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE users
		SET last_login_at = now(), updated_at = now()
		WHERE id = $1
	`, userID)
	return err
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanUser(row rowScanner) (User, error) {
	var user User
	err := row.Scan(
		&user.ID,
		&user.FirstName,
		&user.LastName,
		&user.Username,
		&user.Email,
		&user.PhoneNumber,
		&user.Status,
		&user.Role,
		&user.LastLoginAt,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	return user, err
}

func scanUserWithPassword(row rowScanner) (userWithPassword, error) {
	var user userWithPassword
	err := row.Scan(
		&user.ID,
		&user.FirstName,
		&user.LastName,
		&user.Username,
		&user.Email,
		&user.PhoneNumber,
		&user.Status,
		&user.Role,
		&user.PasswordHash,
		&user.LastLoginAt,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	return user, err
}

func normalizeRole(role string) string {
	return strings.ToLower(strings.TrimSpace(role))
}
