package servers

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

func (h *Handler) consumeShellTicket(ctx context.Context, serverID string, ticket string) error {
	if h.cache == nil {
		return errors.New("ssh_ticket_store_unavailable")
	}
	ticket = strings.TrimSpace(ticket)
	if ticket == "" {
		return errors.New("missing_ssh_ticket")
	}
	key := shellTicketKey(ticket)
	storedServerID, err := h.cache.Get(ctx, key)
	if err != nil {
		return errors.New("invalid_ssh_ticket")
	}
	_ = h.cache.Delete(ctx, key)
	if storedServerID != serverID {
		return errors.New("invalid_ssh_ticket")
	}
	return nil
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
