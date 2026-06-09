package servers

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

var errMissingServerCredential = errors.New("missing server credential")

type SSHExecutor struct {
	timeout time.Duration
}

type SSHCommandResult struct {
	Stdout string
	Stderr string
}

func NewSSHExecutor(timeout time.Duration) *SSHExecutor {
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	return &SSHExecutor{timeout: timeout}
}

func (e *SSHExecutor) Connect(ctx context.Context, item Connection) (*ssh.Client, error) {
	if err := keyAuthError(item); err != nil {
		return nil, err
	}
	config := &ssh.ClientConfig{
		User:            item.Username,
		Auth:            authMethods(item),
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         e.timeout,
	}
	if len(config.Auth) == 0 {
		return nil, errMissingServerCredential
	}

	dialer := net.Dialer{Timeout: e.timeout}
	address := fmt.Sprintf("%s:%d", item.Host, item.Port)
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

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	done := make(chan error, 1)
	go func() {
		done <- session.Run(command)
	}()

	select {
	case <-ctx.Done():
		_ = session.Close()
		return SSHCommandResult{
			Stdout: stdout.String(),
			Stderr: stderr.String(),
		}, ctx.Err()
	case err := <-done:
		result := SSHCommandResult{
			Stdout: stdout.String(),
			Stderr: stderr.String(),
		}
		if err == nil {
			return result, nil
		}
		message := strings.TrimSpace(result.Stderr)
		if message != "" {
			return result, fmt.Errorf("%w: %s", err, message)
		}
		return result, err
	}
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
	return fmt.Errorf("invalid private key: %w", err)
}
