package servers

import (
	"context"
	"time"

	"golang.org/x/crypto/ssh"
)

type SSHAccessService struct {
	repository *Repository
	executor   *SSHExecutor
}

func NewSSHAccessService(repository *Repository, executor *SSHExecutor) *SSHAccessService {
	if executor == nil {
		executor = NewSSHExecutor(20 * time.Second)
	}
	return &SSHAccessService{
		repository: repository,
		executor:   executor,
	}
}

func (s *SSHAccessService) RunCommand(ctx context.Context, id string, command string) (string, error) {
	item, err := s.repository.Get(ctx, id)
	if err != nil {
		return "", err
	}
	client, err := s.executor.Connect(ctx, item)
	if err != nil {
		return "", err
	}
	defer client.Close()
	return s.executor.Output(ctx, client, command)
}

func (s *SSHAccessService) OpenShell(ctx context.Context, id string) (*ssh.Client, *ssh.Session, error) {
	item, err := s.repository.Get(ctx, id)
	if err != nil {
		return nil, nil, err
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

func requestDefaultPTY(session *ssh.Session) error {
	return session.RequestPty("xterm-256color", 40, 120, ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	})
}
