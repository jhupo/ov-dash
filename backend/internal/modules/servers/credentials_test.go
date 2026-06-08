package servers

import (
	"context"
	"errors"
	"testing"
)

type memoryCredentialStore struct {
	values  map[string]string
	deletes []string
	err     error
}

func newMemoryCredentialStore() *memoryCredentialStore {
	return &memoryCredentialStore{values: map[string]string{}}
}

func (s *memoryCredentialStore) Put(ctx context.Context, scope string, name string, plaintext string) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	id := scope + ":" + name
	s.values[id] = plaintext
	return id, nil
}

func (s *memoryCredentialStore) Get(ctx context.Context, id string) (string, error) {
	value, ok := s.values[id]
	if !ok {
		return "", errors.New("missing secret")
	}
	return value, nil
}

func (s *memoryCredentialStore) Delete(ctx context.Context, id string) error {
	s.deletes = append(s.deletes, id)
	delete(s.values, id)
	return nil
}

func (s *memoryCredentialStore) DeleteNamed(ctx context.Context, scope string, name string) error {
	return s.Delete(ctx, scope+":"+name)
}

func TestCredentialResolverPrepareSaveStoresSecrets(t *testing.T) {
	store := newMemoryCredentialStore()
	resolver := NewCredentialResolver(store)
	password := "secret-password"
	privateKey := "secret-key"

	prepared, err := resolver.PrepareSave(context.Background(), SaveInput{
		ID:         "srv_1",
		Password:   &password,
		PrivateKey: &privateKey,
	})
	if err != nil {
		t.Fatalf("PrepareSave returned error: %v", err)
	}

	if prepared.Password == nil || *prepared.Password != "" {
		t.Fatalf("password field was not cleared: %#v", prepared.Password)
	}
	if prepared.PrivateKey == nil || *prepared.PrivateKey != "" {
		t.Fatalf("private key field was not cleared: %#v", prepared.PrivateKey)
	}
	if prepared.PasswordSecretID != "server_connections:srv_1:password" {
		t.Fatalf("password secret id = %q", prepared.PasswordSecretID)
	}
	if prepared.PrivateKeySecretID != "server_connections:srv_1:private_key" {
		t.Fatalf("private key secret id = %q", prepared.PrivateKeySecretID)
	}
}

func TestCredentialResolverPrepareSaveKeepsPlainFieldsWithoutStore(t *testing.T) {
	password := "plain"
	prepared, err := NewCredentialResolver(nil).PrepareSave(context.Background(), SaveInput{
		ID:       "srv_1",
		Password: &password,
	})
	if err != nil {
		t.Fatalf("PrepareSave returned error: %v", err)
	}
	if prepared.Password != &password || prepared.PasswordSecretID != "" {
		t.Fatalf("plain credential was not preserved without store: %+v", prepared)
	}
}

func TestCredentialResolverResolveAndDelete(t *testing.T) {
	store := newMemoryCredentialStore()
	store.values["sec_password"] = "resolved-password"
	store.values["sec_key"] = "resolved-key"
	resolver := NewCredentialResolver(store)

	item := resolver.Resolve(context.Background(), Connection{
		ID:                 "srv_1",
		PasswordSecretID:   "sec_password",
		PrivateKeySecretID: "sec_key",
	})
	if item.Password != "resolved-password" || item.PrivateKey != "resolved-key" {
		t.Fatalf("credentials were not resolved: %+v", item)
	}

	if err := resolver.Delete(context.Background(), item); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	if len(store.deletes) != 2 {
		t.Fatalf("delete count = %d, want 2", len(store.deletes))
	}
}
