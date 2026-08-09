package servers

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSSHAccessServiceRejectsExpiredConnection(t *testing.T) {
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	repository := &sshAccessRepositoryStub{connection: Connection{
		ID:        "srv_expired",
		ExpiresAt: &now,
	}}
	service := NewSSHAccessService(repository, nil, func() time.Time { return now })

	if _, err := service.RunCommand(context.Background(), "srv_expired", "uptime"); !errors.Is(err, ErrServerConnectionExpired) {
		t.Fatalf("RunCommand error = %v, want ErrServerConnectionExpired", err)
	}
	if _, _, err := service.OpenShell(context.Background(), "srv_expired"); !errors.Is(err, ErrServerConnectionExpired) {
		t.Fatalf("OpenShell error = %v, want ErrServerConnectionExpired", err)
	}
	if repository.getCalls != 2 {
		t.Fatalf("credential reads = %d, want 2", repository.getCalls)
	}
}

func TestConnectionExpiredUsesInclusiveBoundary(t *testing.T) {
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	past := now.Add(-time.Nanosecond)
	future := now.Add(time.Nanosecond)

	if !connectionExpired(Connection{ExpiresAt: &past}, now) {
		t.Fatal("past connection was not expired")
	}
	if !connectionExpired(Connection{ExpiresAt: &now}, now) {
		t.Fatal("connection expiring at now was not expired")
	}
	if connectionExpired(Connection{ExpiresAt: &future}, now) {
		t.Fatal("future connection was expired")
	}
	if connectionExpired(Connection{}, now) {
		t.Fatal("connection without expiration was expired")
	}
}

type sshAccessRepositoryStub struct {
	connection Connection
	err        error
	getCalls   int
}

func (s *sshAccessRepositoryStub) GetWithCredentials(context.Context, string) (Connection, error) {
	s.getCalls++
	return s.connection, s.err
}

func (s *sshAccessRepositoryStub) VerifyOrRememberSSHHostKey(context.Context, SSHHostKey) error {
	return nil
}
