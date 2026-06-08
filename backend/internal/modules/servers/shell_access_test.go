package servers

import (
	"context"
	"errors"
	"testing"
	"time"
)

type shellCacheStub struct {
	values  map[string]string
	deleted []string
	err     error
}

func newShellCacheStub() *shellCacheStub {
	return &shellCacheStub{values: map[string]string{}}
}

func (c *shellCacheStub) Get(ctx context.Context, key string) (string, error) {
	if c.err != nil {
		return "", c.err
	}
	value, ok := c.values[key]
	if !ok {
		return "", errors.New("missing")
	}
	return value, nil
}

func (c *shellCacheStub) Set(ctx context.Context, key string, value string, ttl time.Duration) error {
	c.values[key] = value
	return nil
}

func (c *shellCacheStub) Delete(ctx context.Context, keys ...string) error {
	c.deleted = append(c.deleted, keys...)
	for _, key := range keys {
		delete(c.values, key)
	}
	return nil
}

func TestConsumeShellTicketConsumesMatchingTicket(t *testing.T) {
	cache := newShellCacheStub()
	cache.values[shellTicketKey("ticket-1")] = "srv_1"
	handler := &Handler{cache: cache}

	if err := handler.consumeShellTicket(context.Background(), "srv_1", " ticket-1 "); err != nil {
		t.Fatalf("consumeShellTicket returned error: %v", err)
	}
	if len(cache.deleted) != 1 || cache.deleted[0] != shellTicketKey("ticket-1") {
		t.Fatalf("ticket was not deleted: %#v", cache.deleted)
	}
}

func TestConsumeShellTicketRejectsMismatchedTicket(t *testing.T) {
	cache := newShellCacheStub()
	cache.values[shellTicketKey("ticket-1")] = "srv_2"
	handler := &Handler{cache: cache}

	if err := handler.consumeShellTicket(context.Background(), "srv_1", "ticket-1"); err == nil || err.Error() != "invalid_ssh_ticket" {
		t.Fatalf("error = %v, want invalid_ssh_ticket", err)
	}
}

func TestShellTicketExpiry(t *testing.T) {
	now := time.Date(2026, 6, 9, 15, 0, 0, 0, time.UTC)
	ttl, expiresAt := shellTicketExpiry(now)
	if ttl != 30*time.Second {
		t.Fatalf("ttl = %s", ttl)
	}
	if !expiresAt.Equal(now.Add(30 * time.Second)) {
		t.Fatalf("expiresAt = %s", expiresAt)
	}
}

func TestHandleTerminalResizeRecognition(t *testing.T) {
	handled, err := handleTerminalResize(nil, "plain text")
	if err != nil {
		t.Fatalf("handleTerminalResize returned error: %v", err)
	}
	if handled {
		t.Fatal("plain text should not be treated as resize")
	}

	handled, err = handleTerminalResize(nil, "\x1b]ovdash-resize;bad\x07")
	if err != nil {
		t.Fatalf("invalid resize payload should not call session: %v", err)
	}
	if !handled {
		t.Fatal("resize prefix should be handled")
	}

	handled, err = handleTerminalResize(nil, "\x1b]ovdash-resize;500;200\x07")
	if err != nil {
		t.Fatalf("out-of-range resize should not call session: %v", err)
	}
	if !handled {
		t.Fatal("out-of-range resize should be handled")
	}
}
