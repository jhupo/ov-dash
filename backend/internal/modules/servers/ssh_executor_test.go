package servers

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestNewSSHExecutorUsesDefaultTimeout(t *testing.T) {
	executor := NewSSHExecutor(0)
	if executor.timeout == 0 {
		t.Fatal("default timeout was not applied")
	}
}

func TestAuthMethodsRequiresCredential(t *testing.T) {
	if got := authMethods(Connection{}); len(got) != 0 {
		t.Fatalf("auth methods = %d, want 0", len(got))
	}
	if got := authMethods(Connection{Password: "secret"}); len(got) != 1 {
		t.Fatalf("password auth methods = %d, want 1", len(got))
	}
}

func TestKeyAuthErrorRejectsInvalidPrivateKey(t *testing.T) {
	err := keyAuthError(Connection{AuthType: "key", PrivateKey: "not-a-key"})
	if err == nil {
		t.Fatal("keyAuthError returned nil")
	}
	if !strings.Contains(err.Error(), "invalid private key") {
		t.Fatalf("error = %v", err)
	}
}

func TestConnectRejectsMissingCredentialBeforeDial(t *testing.T) {
	_, err := NewSSHExecutor(0).Connect(context.Background(), Connection{
		Host:     "127.0.0.1",
		Port:     22,
		Username: "root",
	})
	if err == nil {
		t.Fatal("Connect returned nil error")
	}
	if !errors.Is(err, errMissingServerCredential) {
		t.Fatalf("error = %v, want missing server credential", err)
	}
}
