package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

const sessionDuration = 7 * 24 * time.Hour

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrInactiveUser       = errors.New("inactive user")
	ErrInvalidSession     = errors.New("invalid session")
	ErrPasswordTooShort   = errors.New("password must be at least 8 characters")
)

type Service struct {
	repository *Repository
	now        func() time.Time
}

type LoginInput struct {
	Email     string
	Password  string
	UserAgent string
	IPAddress string
}

type LoginResult struct {
	User      User      `json:"user"`
	Token     string    `json:"-"`
	ExpiresAt time.Time `json:"expiresAt"`
}

func NewService(repository *Repository) *Service {
	return &Service{repository: repository, now: time.Now}
}

func (s *Service) EnsureDefaultAdmin(ctx context.Context) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(DefaultAdminPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return s.repository.EnsureDefaultAdmin(ctx, string(hash))
}

func (s *Service) Login(ctx context.Context, input LoginInput) (LoginResult, error) {
	email := strings.TrimSpace(input.Email)
	password := input.Password
	if email == "" || password == "" {
		return LoginResult{}, ErrInvalidCredentials
	}

	user, err := s.repository.FindUserByEmail(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		return LoginResult{}, ErrInvalidCredentials
	}
	if err != nil {
		return LoginResult{}, err
	}
	if user.Status != "active" {
		return LoginResult{}, ErrInactiveUser
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return LoginResult{}, ErrInvalidCredentials
	}

	token, err := randomToken()
	if err != nil {
		return LoginResult{}, err
	}
	expiresAt := s.now().Add(sessionDuration)
	session := Session{
		ID:        "sess_" + strings.ReplaceAll(token[:22], "-", ""),
		UserID:    user.ID,
		TokenHash: hashToken(token),
		ExpiresAt: expiresAt,
	}
	if err := s.repository.CreateSession(ctx, session, input.UserAgent, input.IPAddress); err != nil {
		return LoginResult{}, err
	}
	if err := s.repository.TouchLastLogin(ctx, user.ID); err != nil {
		return LoginResult{}, err
	}
	user.User.LastLoginAt = ptrTime(s.now())

	return LoginResult{User: user.User, Token: token, ExpiresAt: expiresAt}, nil
}

func (s *Service) CurrentUser(ctx context.Context, token string) (User, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return User{}, ErrInvalidSession
	}
	user, err := s.repository.FindSessionUser(ctx, hashToken(token), s.now())
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrInvalidSession
	}
	return user, err
}

func (s *Service) Logout(ctx context.Context, token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil
	}
	return s.repository.RevokeSession(ctx, hashToken(token))
}

func (s *Service) ChangePassword(ctx context.Context, userID string, currentPassword string, newPassword string) error {
	if len(newPassword) < 8 {
		return ErrPasswordTooShort
	}
	user, err := s.repository.FindUserByID(ctx, userID)
	if err != nil {
		return err
	}
	withPassword, err := s.repository.FindUserByEmail(ctx, user.Email)
	if err != nil {
		return err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(withPassword.PasswordHash), []byte(currentPassword)); err != nil {
		return ErrInvalidCredentials
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return s.repository.UpdatePassword(ctx, userID, string(hash))
}

func SessionDuration() time.Duration {
	return sessionDuration
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func randomToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

func ptrTime(value time.Time) *time.Time {
	return &value
}
