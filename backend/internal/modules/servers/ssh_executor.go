package servers

import (
	"bytes"
	"context"
	"errors"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

var errMissingServerCredential = errors.New("missing server credential")

var (
	ErrSSHHostKeyChanged            = errors.New("ssh_host_key_changed")
	ErrSSHOutputLimitExceeded       = errors.New("ssh_output_limit_exceeded")
	errSSHHostKeyVerificationFailed = errors.New("ssh_host_key_verification_failed")
)

const sshCommandOutputLimit = 4 << 20

type SSHHostKey struct {
	ServerID    string
	Algorithm   string
	PublicKey   string
	Fingerprint string
}

type sshHostKeyStore interface {
	VerifyOrRememberSSHHostKey(ctx context.Context, key SSHHostKey) error
}

type SSHExecutor struct {
	timeout  time.Duration
	hostKeys sshHostKeyStore
}

type SSHCommandResult struct {
	Stdout string
	Stderr string
}

func NewSSHExecutor(timeout time.Duration, hostKeys sshHostKeyStore) *SSHExecutor {
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	return &SSHExecutor{timeout: timeout, hostKeys: hostKeys}
}

func (e *SSHExecutor) Connect(ctx context.Context, item Connection) (*ssh.Client, error) {
	if err := keyAuthError(item); err != nil {
		return nil, err
	}
	config := &ssh.ClientConfig{
		User:            item.Username,
		Auth:            authMethods(item),
		HostKeyCallback: e.hostKeyCallback(ctx, item.ID),
		Timeout:         e.timeout,
	}
	if len(config.Auth) == 0 {
		return nil, errMissingServerCredential
	}
	if e.hostKeys == nil {
		return nil, errSSHHostKeyVerificationFailed
	}

	dialer := net.Dialer{Timeout: e.timeout}
	address := net.JoinHostPort(item.Host, strconv.Itoa(item.Port))
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, err
	}

	done := make(chan resultClient, 1)
	go func() {
		sshConn, chans, reqs, err := ssh.NewClientConn(conn, address, config)
		if err != nil {
			done <- resultClient{err: err}
			return
		}
		done <- resultClient{client: ssh.NewClient(sshConn, chans, reqs)}
	}()

	select {
	case <-ctx.Done():
		_ = conn.Close()
		return nil, ctx.Err()
	case result := <-done:
		return result.client, result.err
	}
}

func (e *SSHExecutor) hostKeyCallback(ctx context.Context, serverID string) ssh.HostKeyCallback {
	return func(_ string, _ net.Addr, key ssh.PublicKey) error {
		if e.hostKeys == nil || strings.TrimSpace(serverID) == "" {
			return errSSHHostKeyVerificationFailed
		}
		record := SSHHostKey{
			ServerID:    serverID,
			Algorithm:   key.Type(),
			PublicKey:   strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key))),
			Fingerprint: ssh.FingerprintSHA256(key),
		}
		if err := e.hostKeys.VerifyOrRememberSSHHostKey(ctx, record); err != nil {
			if errors.Is(err, ErrSSHHostKeyChanged) {
				return ErrSSHHostKeyChanged
			}
			return errSSHHostKeyVerificationFailed
		}
		return nil
	}
}

func (e *SSHExecutor) Run(ctx context.Context, client *ssh.Client, command string) error {
	_, err := e.RunResult(ctx, client, command)
	return err
}

func (e *SSHExecutor) Output(ctx context.Context, client *ssh.Client, command string) (string, error) {
	result, err := e.RunResult(ctx, client, command)
	return result.Stdout, err
}

func (e *SSHExecutor) RunResult(ctx context.Context, client *ssh.Client, command string) (SSHCommandResult, error) {
	session, err := client.NewSession()
	if err != nil {
		return SSHCommandResult{}, err
	}
	defer session.Close()

	output := newSSHCommandOutput(sshCommandOutputLimit, func() {
		_ = session.Close()
	})
	session.Stdout = output.stdoutWriter()
	session.Stderr = output.stderrWriter()

	done := make(chan error, 1)
	go func() {
		done <- session.Run(command)
	}()

	select {
	case <-ctx.Done():
		_ = session.Close()
		<-done
		result, limitExceeded := output.result()
		if limitExceeded {
			return result, ErrSSHOutputLimitExceeded
		}
		return result, ctx.Err()
	case err := <-done:
		result, limitExceeded := output.result()
		if limitExceeded {
			return result, ErrSSHOutputLimitExceeded
		}
		if err == nil {
			return result, nil
		}
		return result, err
	}
}

type sshCommandOutput struct {
	mu        sync.Mutex
	limit     int
	used      int
	exceeded  bool
	closeOnce sync.Once
	close     func()
	stdout    bytes.Buffer
	stderr    bytes.Buffer
}

type sshCommandOutputWriter struct {
	output *sshCommandOutput
	stderr bool
}

func newSSHCommandOutput(limit int, closeSession func()) *sshCommandOutput {
	return &sshCommandOutput{limit: limit, close: closeSession}
}

func (o *sshCommandOutput) stdoutWriter() *sshCommandOutputWriter {
	return &sshCommandOutputWriter{output: o}
}

func (o *sshCommandOutput) stderrWriter() *sshCommandOutputWriter {
	return &sshCommandOutputWriter{output: o, stderr: true}
}

func (w *sshCommandOutputWriter) Write(payload []byte) (int, error) {
	o := w.output
	o.mu.Lock()
	remaining := o.limit - o.used
	if remaining < 0 {
		remaining = 0
	}
	written := len(payload)
	if written > remaining {
		written = remaining
	}
	if written > 0 {
		target := &o.stdout
		if w.stderr {
			target = &o.stderr
		}
		_, _ = target.Write(payload[:written])
		o.used += written
	}
	limitReached := len(payload) > 0 && o.used >= o.limit
	if limitReached {
		o.exceeded = true
	}
	o.mu.Unlock()

	if limitReached {
		o.closeOnce.Do(func() {
			if o.close != nil {
				o.close()
			}
		})
		return written, ErrSSHOutputLimitExceeded
	}
	return written, nil
}

func (o *sshCommandOutput) result() (SSHCommandResult, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return SSHCommandResult{
		Stdout: o.stdout.String(),
		Stderr: o.stderr.String(),
	}, o.exceeded
}

type resultClient struct {
	client *ssh.Client
	err    error
}

func authMethods(item Connection) []ssh.AuthMethod {
	methods := make([]ssh.AuthMethod, 0, 2)
	if item.AuthType == "key" && strings.TrimSpace(item.PrivateKey) != "" {
		if signer, err := ssh.ParsePrivateKey([]byte(item.PrivateKey)); err == nil {
			methods = append(methods, ssh.PublicKeys(signer))
		}
	}
	if strings.TrimSpace(item.Password) != "" {
		methods = append(methods, ssh.Password(item.Password))
	}
	return methods
}

func keyAuthError(item Connection) error {
	if item.AuthType != "key" || strings.TrimSpace(item.PrivateKey) == "" {
		return nil
	}
	_, err := ssh.ParsePrivateKey([]byte(item.PrivateKey))
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "passphrase") {
		return errors.New("private key passphrase is not supported yet")
	}
	return errors.New("invalid private key")
}

func sshFailureCode(err error) string {
	switch {
	case IsNotFound(err):
		return "server_connection_not_found"
	case errors.Is(err, ErrSSHHostKeyChanged):
		return "ssh_host_key_changed"
	case errors.Is(err, ErrServerConnectionExpired):
		return "server_connection_expired"
	case errors.Is(err, ErrSSHOutputLimitExceeded):
		return "ssh_output_limit_exceeded"
	case errors.Is(err, errSSHHostKeyVerificationFailed):
		return "ssh_host_key_verification_failed"
	default:
		return "ssh_connection_failed"
	}
}
