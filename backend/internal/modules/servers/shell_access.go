package servers

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/crypto/ssh"
)

type shellTicket struct {
	ServerID string `json:"server_id"`
	UserID   string `json:"user_id"`
}

func (h *Handler) consumeShellTicket(ctx context.Context, serverID string, userID string, ticket string) error {
	if h.cache == nil {
		return errors.New("ssh_ticket_store_unavailable")
	}
	ticket = strings.TrimSpace(ticket)
	if ticket == "" {
		return errors.New("missing_ssh_ticket")
	}
	value, err := h.cache.Take(ctx, shellTicketKey(ticket))
	if err != nil {
		return errors.New("invalid_ssh_ticket")
	}
	stored, err := decodeShellTicket(value)
	if err != nil || stored.ServerID != serverID || stored.UserID != userID {
		return errors.New("invalid_ssh_ticket")
	}
	return nil
}

func encodeShellTicket(ticket shellTicket) (string, error) {
	value, err := json.Marshal(ticket)
	return string(value), err
}

func decodeShellTicket(value string) (shellTicket, error) {
	var ticket shellTicket
	if err := json.Unmarshal([]byte(value), &ticket); err != nil {
		return shellTicket{}, err
	}
	if strings.TrimSpace(ticket.ServerID) == "" || strings.TrimSpace(ticket.UserID) == "" {
		return shellTicket{}, errors.New("invalid ssh ticket")
	}
	return ticket, nil
}

func shellTicketKey(ticket string) string {
	return "servers:ssh-ticket:" + ticket
}

func randomTicket() (string, error) {
	buffer := make([]byte, 32)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

func handleTerminalResize(session *ssh.Session, text string) (bool, error) {
	const prefix = "\x1b]ovdash-resize;"
	const suffix = "\x07"
	if !strings.HasPrefix(text, prefix) || !strings.HasSuffix(text, suffix) {
		return false, nil
	}
	size := strings.TrimSuffix(strings.TrimPrefix(text, prefix), suffix)
	parts := strings.Split(size, ";")
	if len(parts) != 2 {
		return true, nil
	}
	cols, err := strconv.Atoi(parts[0])
	if err != nil {
		return true, nil
	}
	rows, err := strconv.Atoi(parts[1])
	if err != nil {
		return true, nil
	}
	if cols < 20 || rows < 5 || cols > 300 || rows > 120 {
		return true, nil
	}
	return true, session.WindowChange(rows, cols)
}

func shellTicketExpiry(now time.Time) (time.Duration, time.Time) {
	expiresIn := 30 * time.Second
	return expiresIn, now.Add(expiresIn)
}

func (h *Handler) serveShell(requestCtx context.Context, conn *websocket.Conn, serverID string) {
	ctx, cancel := context.WithCancel(requestCtx)
	defer cancel()

	client, session, err := h.ssh.OpenShell(ctx, serverID)
	if err != nil {
		writeWebSocketError(conn, sshFailureCode(err))
		return
	}

	stdin, err := session.StdinPipe()
	if err != nil {
		_ = session.Close()
		_ = client.Close()
		writeWebSocketError(conn, "ssh_shell_setup_failed")
		return
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		_ = session.Close()
		_ = client.Close()
		writeWebSocketError(conn, "ssh_shell_setup_failed")
		return
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		_ = stdin.Close()
		_ = session.Close()
		_ = client.Close()
		writeWebSocketError(conn, "ssh_shell_setup_failed")
		return
	}
	if err := session.Shell(); err != nil {
		_ = stdin.Close()
		_ = session.Close()
		_ = client.Close()
		writeWebSocketError(conn, "ssh_shell_start_failed")
		return
	}

	outbound := make(chan webSocketMessage, 64)
	control := make(chan webSocketMessage, 8)
	exits := make(chan error, 5)
	writerDone := make(chan struct{})
	var workers sync.WaitGroup
	reportExit := func(err error) {
		select {
		case exits <- err:
		case <-ctx.Done():
		}
	}

	workers.Add(1)
	go func() {
		defer workers.Done()
		defer close(writerDone)
		reportExit(writeWebSocketPump(ctx, conn, outbound, control))
	}()

	workers.Add(1)
	go func() {
		defer workers.Done()
		reportExit(readWebSocketPump(ctx, conn, stdin, func(payload []byte) (bool, error) {
			return handleTerminalResize(session, string(payload))
		}, control))
	}()

	workers.Add(1)
	go func() {
		defer workers.Done()
		reportExit(session.Wait())
	}()

	var outputReaders sync.WaitGroup
	for _, reader := range []io.Reader{stdout, stderr} {
		outputReaders.Add(1)
		workers.Add(1)
		go func(reader io.Reader) {
			defer workers.Done()
			defer outputReaders.Done()
			if err := copySSHOutput(ctx, reader, outbound); err != nil && !errors.Is(err, context.Canceled) {
				reportExit(err)
			}
		}(reader)
	}
	workers.Add(1)
	go func() {
		defer workers.Done()
		outputReaders.Wait()
		reportExit(nil)
	}()

	select {
	case <-requestCtx.Done():
	case <-exits:
	}
	cancel()
	_ = stdin.Close()
	_ = session.Close()
	_ = client.Close()

	select {
	case <-writerDone:
	case <-time.After(webSocketCloseWait):
	}
	_ = conn.Close()
	workers.Wait()
}
