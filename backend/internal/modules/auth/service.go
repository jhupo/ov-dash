package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

const sessionDuration = 7 * 24 * time.Hour

var usernamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{2,63}$`)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrInactiveUser       = errors.New("inactive user")
	ErrInvalidSession     = errors.New("invalid session")
	ErrPasswordTooShort   = errors.New("password must be at least 8 characters")
	ErrAdminAlreadyExists = errors.New("an administrator already exists")
	ErrBootstrapEmail     = errors.New("bootstrap admin email is invalid")
	ErrBootstrapUsername  = errors.New("bootstrap admin username must be 3-64 letters, numbers, dots, underscores, or hyphens")
	ErrBootstrapPassword  = errors.New("bootstrap admin password must be at least 12 characters")
)

type Service struct {
	repository serviceRepository
	now        func() time.Time
}

type serviceRepository interface {
	CreateInitialAdmin(context.Context, User, string) (User, error)
	FindUserByEmail(context.Context, string) (userWithPassword, error)
	FindUserByID(context.Context, string) (User, error)
	CreateSession(context.Context, Session, string, string) error
	FindSessionUser(context.Context, string, time.Time) (User, error)
	ListCapabilitiesByRole(context.Context, string) ([]string, error)
	RevokeSession(context.Context, string) error
	UpdatePassword(context.Context, string, string) error
	TouchLastLogin(context.Context, string) error
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

func NewService(repository serviceRepository) *Service {
	return &Service{repository: repository, now: time.Now}
}

func (s *Service) BootstrapAdmin(ctx context.Context, input BootstrapAdminInput) (User, error) {
	input, err := normalizeBootstrapAdminInput(input)
	if err != nil {
		return User{}, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return User{}, fmt.Errorf("hash bootstrap admin password: %w", err)
	}
	token, err := randomToken()
	if err != nil {
		return User{}, fmt.Errorf("generate bootstrap admin id: %w", err)
	}

	user := User{
		ID:        "user_" + token[:22],
		FirstName: input.FirstName,
		LastName:  input.LastName,
		Username:  input.Username,
		Email:     input.Email,
		Status:    "active",
		Role:      "admin",
	}
	return s.repository.CreateInitialAdmin(ctx, user, string(hash))
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
	publicUser, err := s.ResolvePublicUser(ctx, user.User)
	if err != nil {
		return LoginResult{}, err
	}

	return LoginResult{User: publicUser, Token: token, ExpiresAt: expiresAt}, nil
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

func (s *Service) ResolvePublicUser(ctx context.Context, user User) (User, error) {
	capabilities, err := s.repository.ListCapabilitiesByRole(ctx, user.Role)
	if err != nil {
		return User{}, fmt.Errorf("list capabilities for role %q: %w", normalizeRole(user.Role), err)
	}
	user.Capabilities = capabilities
	return user, nil
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

func normalizeBootstrapAdminInput(input BootstrapAdminInput) (BootstrapAdminInput, error) {
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	input.Username = strings.TrimSpace(input.Username)
	input.FirstName = strings.TrimSpace(input.FirstName)
	input.LastName = strings.TrimSpace(input.LastName)

	address, err := mail.ParseAddress(input.Email)
	if err != nil || !strings.EqualFold(address.Address, input.Email) {
		return BootstrapAdminInput{}, ErrBootstrapEmail
	}
	if !usernamePattern.MatchString(input.Username) {
		return BootstrapAdminInput{}, ErrBootstrapUsername
	}
	if len(input.Password) < 12 {
		return BootstrapAdminInput{}, ErrBootstrapPassword
	}
	if input.FirstName == "" {
		input.FirstName = "Admin"
	}
	return input, nil
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
