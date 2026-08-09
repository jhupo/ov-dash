package servers

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func TestNewSSHExecutorUsesDefaultTimeout(t *testing.T) {
	executor := NewSSHExecutor(0, &hostKeyStoreStub{})
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
	_, err := NewSSHExecutor(0, nil).Connect(context.Background(), Connection{
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

func TestHostKeyCallbackRecordsTOFUKey(t *testing.T) {
	store := &hostKeyStoreStub{}
	executor := NewSSHExecutor(time.Second, store)
	key := testSSHPublicKey(t)

	if err := executor.hostKeyCallback(context.Background(), "srv_1")("host:22", &net.TCPAddr{}, key); err != nil {
		t.Fatalf("host key callback returned error: %v", err)
	}
	if len(store.keys) != 1 {
		t.Fatalf("stored keys = %d, want 1", len(store.keys))
	}
	stored := store.keys[0]
	if stored.ServerID != "srv_1" || stored.Algorithm != key.Type() {
		t.Fatalf("stored host key identity = %+v", stored)
	}
	if stored.Fingerprint != ssh.FingerprintSHA256(key) || stored.PublicKey == "" {
		t.Fatalf("stored host key material = %+v", stored)
	}
}

func TestHostKeyCallbackRejectsChangedKeyWithoutLeakingStoreError(t *testing.T) {
	changed := NewSSHExecutor(time.Second, &hostKeyStoreStub{err: ErrSSHHostKeyChanged})
	err := changed.hostKeyCallback(context.Background(), "srv_1")("host:22", &net.TCPAddr{}, testSSHPublicKey(t))
	if !errors.Is(err, ErrSSHHostKeyChanged) {
		t.Fatalf("changed key error = %v", err)
	}

	storeFailure := NewSSHExecutor(time.Second, &hostKeyStoreStub{err: errors.New("password=database-secret")})
	err = storeFailure.hostKeyCallback(context.Background(), "srv_1")("host:22", &net.TCPAddr{}, testSSHPublicKey(t))
	if !errors.Is(err, errSSHHostKeyVerificationFailed) || strings.Contains(err.Error(), "database-secret") {
		t.Fatalf("store failure was not sanitized: %v", err)
	}
}

func TestSSHCommandOutputUsesOneConcurrentLimit(t *testing.T) {
	const limit = 4096
	var closeCalls atomic.Int32
	output := newSSHCommandOutput(limit, func() {
		closeCalls.Add(1)
	})
	writers := []*sshCommandOutputWriter{output.stdoutWriter(), output.stderrWriter()}
	payload := bytes.Repeat([]byte("x"), 256)

	var group sync.WaitGroup
	for index := 0; index < 64; index++ {
		group.Add(1)
		go func(writer *sshCommandOutputWriter) {
			defer group.Done()
			_, _ = writer.Write(payload)
		}(writers[index%len(writers)])
	}
	group.Wait()

	result, exceeded := output.result()
	if !exceeded {
		t.Fatal("combined output limit was not marked exceeded")
	}
	if size := len(result.Stdout) + len(result.Stderr); size != limit {
		t.Fatalf("retained output = %d bytes, want %d", size, limit)
	}
	if closeCalls.Load() != 1 {
		t.Fatalf("session close calls = %d, want 1", closeCalls.Load())
	}
}

func TestSSHCommandOutputReturnsStableLimitError(t *testing.T) {
	output := newSSHCommandOutput(4, nil)
	written, err := output.stdoutWriter().Write([]byte("12345"))
	if written != 4 {
		t.Fatalf("written = %d, want 4", written)
	}
	if !errors.Is(err, ErrSSHOutputLimitExceeded) {
		t.Fatalf("error = %v, want ErrSSHOutputLimitExceeded", err)
	}
	if code := sshFailureCode(err); code != "ssh_output_limit_exceeded" {
		t.Fatalf("failure code = %q", code)
	}
}

func TestSSHFailureCodeMapsExpiredConnection(t *testing.T) {
	if code := sshFailureCode(ErrServerConnectionExpired); code != "server_connection_expired" {
		t.Fatalf("failure code = %q", code)
	}
}

type hostKeyStoreStub struct {
	keys []SSHHostKey
	err  error
}

func (s *hostKeyStoreStub) VerifyOrRememberSSHHostKey(_ context.Context, key SSHHostKey) error {
	s.keys = append(s.keys, key)
	return s.err
}

func testSSHPublicKey(t *testing.T) ssh.PublicKey {
	t.Helper()
	publicKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	key, err := ssh.NewPublicKey(publicKey)
	if err != nil {
		t.Fatalf("new SSH public key: %v", err)
	}
	return key
}
