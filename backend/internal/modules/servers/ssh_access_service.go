package servers

import (
	"context"
	"errors"
	"time"

	"golang.org/x/crypto/ssh"
)

var ErrServerConnectionExpired = errors.New("server_connection_expired")

type sshAccessRepository interface {
	GetWithCredentials(context.Context, string) (Connection, error)
	VerifyOrRememberSSHHostKey(context.Context, SSHHostKey) error
}

type SSHAccessService struct {
	repository sshAccessRepository
	executor   *SSHExecutor
	now        func() time.Time
}

func NewSSHAccessService(repository sshAccessRepository, executor *SSHExecutor, now func() time.Time) *SSHAccessService {
	if executor == nil {
		executor = NewSSHExecutor(20*time.Second, repository)
	}
	if now == nil {
		now = time.Now
	}
	return &SSHAccessService{
		repository: repository,
		executor:   executor,
		now:        now,
	}
}

func (s *SSHAccessService) RunCommand(ctx context.Context, id string, command string) (string, error) {
	item, err := s.repository.GetWithCredentials(ctx, id)
	if err != nil {
		return "", err
	}
	if connectionExpired(item, s.now()) {
		return "", ErrServerConnectionExpired
	}
	client, err := s.executor.Connect(ctx, item)
	if err != nil {
		return "", err
	}
	defer client.Close()
	return s.executor.Output(ctx, client, command)
}

func (s *SSHAccessService) OpenShell(ctx context.Context, id string) (*ssh.Client, *ssh.Session, error) {
	item, err := s.repository.GetWithCredentials(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	if connectionExpired(item, s.now()) {
		return nil, nil, ErrServerConnectionExpired
	}
	client, err := s.executor.Connect(ctx, item)
	if err != nil {
		return nil, nil, err
	}
	session, err := client.NewSession()
	if err != nil {
		client.Close()
		return nil, nil, err
	}
	if err := requestDefaultPTY(session); err != nil {
		session.Close()
		client.Close()
		return nil, nil, err
	}
	return client, session, nil
}

func connectionExpired(item Connection, now time.Time) bool {
	return item.ExpiresAt != nil && !item.ExpiresAt.After(now)
}

func requestDefaultPTY(session *ssh.Session) error {
	return session.RequestPty("xterm-256color", 40, 120, ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	})
}
