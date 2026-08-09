package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type fakeAuthRepository struct {
	serviceRepository
	passwordUser      userWithPassword
	sessionUser       User
	capabilities      []string
	capabilityRoles   []string
	createdSession    Session
	lastLoginUserID   string
	findSessionErr    error
	capabilitiesError error
}

func (r *fakeAuthRepository) FindUserByEmail(context.Context, string) (userWithPassword, error) {
	return r.passwordUser, nil
}

func (r *fakeAuthRepository) CreateSession(_ context.Context, session Session, _, _ string) error {
	r.createdSession = session
	return nil
}

func (r *fakeAuthRepository) TouchLastLogin(_ context.Context, userID string) error {
	r.lastLoginUserID = userID
	return nil
}

func (r *fakeAuthRepository) FindSessionUser(context.Context, string, time.Time) (User, error) {
	return r.sessionUser, r.findSessionErr
}

func (r *fakeAuthRepository) ListCapabilitiesByRole(_ context.Context, role string) ([]string, error) {
	r.capabilityRoles = append(r.capabilityRoles, role)
	return append([]string(nil), r.capabilities...), r.capabilitiesError
}

func TestNormalizeBootstrapAdminInput(t *testing.T) {
	input, err := normalizeBootstrapAdminInput(BootstrapAdminInput{
		Email:    " Admin@Example.COM ",
		Password: "a-secure-password",
		Username: "admin.user",
	})
	if err != nil {
		t.Fatalf("normalize input: %v", err)
	}
	if input.Email != "admin@example.com" {
		t.Fatalf("expected normalized email, got %q", input.Email)
	}
	if input.FirstName != "Admin" {
		t.Fatalf("expected default first name, got %q", input.FirstName)
	}
}

func TestNormalizeBootstrapAdminInputRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name  string
		input BootstrapAdminInput
		want  error
	}{
		{name: "email", input: BootstrapAdminInput{Email: "invalid", Username: "admin", Password: "a-secure-password"}, want: ErrBootstrapEmail},
		{name: "username", input: BootstrapAdminInput{Email: "admin@example.com", Username: "a", Password: "a-secure-password"}, want: ErrBootstrapUsername},
		{name: "password", input: BootstrapAdminInput{Email: "admin@example.com", Username: "admin", Password: "short"}, want: ErrBootstrapPassword},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := normalizeBootstrapAdminInput(tt.input)
			if !errors.Is(err, tt.want) {
				t.Fatalf("expected %v, got %v", tt.want, err)
			}
		})
	}
}

func TestLoginIncludesRoleCapabilities(t *testing.T) {
	passwordHash, err := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	repository := &fakeAuthRepository{
		passwordUser: userWithPassword{
			User:         User{ID: "user-1", Email: "user@example.com", Status: "active", Role: "operator"},
			PasswordHash: string(passwordHash),
		},
		capabilities: []string{"jobs:manage", "servers:ssh"},
	}
	service := NewService(repository)
	service.now = func() time.Time { return time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC) }

	result, err := service.Login(context.Background(), LoginInput{
		Email:    "user@example.com",
		Password: "correct-password",
	})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if len(repository.capabilityRoles) != 1 || repository.capabilityRoles[0] != "operator" {
		t.Fatalf("capability roles = %#v", repository.capabilityRoles)
	}
	if len(result.User.Capabilities) != 2 || result.User.Capabilities[0] != "jobs:manage" {
		t.Fatalf("capabilities = %#v", result.User.Capabilities)
	}
	if repository.createdSession.UserID != "user-1" || repository.lastLoginUserID != "user-1" {
		t.Fatalf("session or last login was not persisted")
	}
}

func TestCurrentUserOnlyResolvesSessionIdentity(t *testing.T) {
	repository := &fakeAuthRepository{
		sessionUser: User{ID: "user-2", Role: "viewer"},
	}
	service := NewService(repository)

	user, err := service.CurrentUser(context.Background(), "session-token")
	if err != nil {
		t.Fatalf("current user: %v", err)
	}
	if len(repository.capabilityRoles) != 0 {
		t.Fatalf("CurrentUser queried capabilities for roles %#v", repository.capabilityRoles)
	}
	if user.ID != "user-2" || user.Role != "viewer" {
		t.Fatalf("user = %#v", user)
	}
}

func TestResolvePublicUserReturnsCapabilityQueryError(t *testing.T) {
	repository := &fakeAuthRepository{capabilitiesError: errors.New("database unavailable")}
	service := NewService(repository)

	_, err := service.ResolvePublicUser(context.Background(), User{Role: "viewer"})
	if err == nil || !strings.Contains(err.Error(), "list capabilities for role") {
		t.Fatalf("expected capability query error, got %v", err)
	}
}
