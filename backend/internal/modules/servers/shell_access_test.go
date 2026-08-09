package servers

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

type shellCacheStub struct {
	values map[string]string
	err    error
}

func newShellCacheStub() *shellCacheStub {
	return &shellCacheStub{values: map[string]string{}}
}

func (c *shellCacheStub) Take(ctx context.Context, key string) (string, error) {
	if c.err != nil {
		return "", c.err
	}
	value, ok := c.values[key]
	if !ok {
		return "", errors.New("missing")
	}
	delete(c.values, key)
	return value, nil
}

func (c *shellCacheStub) Set(ctx context.Context, key string, value string, ttl time.Duration) error {
	c.values[key] = value
	return nil
}

func TestConsumeShellTicketConsumesMatchingTicket(t *testing.T) {
	cache := newShellCacheStub()
	value, err := encodeShellTicket(shellTicket{ServerID: "srv_1", UserID: "user_1"})
	if err != nil {
		t.Fatalf("encodeShellTicket returned error: %v", err)
	}
	cache.values[shellTicketKey("ticket-1")] = value
	handler := &Handler{cache: cache}

	if err := handler.consumeShellTicket(context.Background(), "srv_1", "user_1", " ticket-1 "); err != nil {
		t.Fatalf("consumeShellTicket returned error: %v", err)
	}
	if err := handler.consumeShellTicket(context.Background(), "srv_1", "user_1", "ticket-1"); err == nil || err.Error() != "invalid_ssh_ticket" {
		t.Fatalf("replayed ticket error = %v, want invalid_ssh_ticket", err)
	}
}

func TestConsumeShellTicketRejectsMismatchedServerOrUser(t *testing.T) {
	cache := newShellCacheStub()
	handler := &Handler{cache: cache}

	serverMismatch, _ := encodeShellTicket(shellTicket{ServerID: "srv_2", UserID: "user_1"})
	cache.values[shellTicketKey("ticket-server")] = serverMismatch
	if err := handler.consumeShellTicket(context.Background(), "srv_1", "user_1", "ticket-server"); err == nil || err.Error() != "invalid_ssh_ticket" {
		t.Fatalf("server mismatch error = %v, want invalid_ssh_ticket", err)
	}

	userMismatch, _ := encodeShellTicket(shellTicket{ServerID: "srv_1", UserID: "user_2"})
	cache.values[shellTicketKey("ticket-user")] = userMismatch
	if err := handler.consumeShellTicket(context.Background(), "srv_1", "user_1", "ticket-user"); err == nil || err.Error() != "invalid_ssh_ticket" {
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

func TestWebSocketPumpsSendBinaryAndAcceptTextAndBinary(t *testing.T) {
	type serverResult struct {
		input string
		err   error
	}
	result := make(chan serverResult, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgradeWebSocket(w, r, []string{"http://terminal.example"})
		if err != nil {
			result <- serverResult{err: err}
			return
		}
		defer conn.Close()

		ctx, cancel := context.WithCancel(r.Context())
		outbound := make(chan webSocketMessage, 1)
		control := make(chan webSocketMessage, 1)
		writerDone := make(chan error, 1)
		go func() {
			writerDone <- writeWebSocketPump(ctx, conn, outbound, control)
		}()
		outbound <- webSocketMessage{messageType: websocket.BinaryMessage, payload: []byte("ready")}

		var input bytes.Buffer
		readErr := readWebSocketPump(ctx, conn, &input, nil, control)
		cancel()
		_ = conn.Close()
		<-writerDone
		if websocket.IsCloseError(readErr, websocket.CloseNormalClosure) {
			readErr = nil
		}
		result <- serverResult{input: input.String(), err: readErr}
	}))
	defer server.Close()

	webSocketURL := "ws" + strings.TrimPrefix(server.URL, "http")
	header := http.Header{}
	header.Set("Origin", "http://terminal.example")
	conn, response, err := websocket.DefaultDialer.Dial(webSocketURL, header)
	if response != nil {
		defer response.Body.Close()
	}
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close()

	messageType, payload, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read server output: %v", err)
	}
	if messageType != websocket.BinaryMessage || string(payload) != "ready" {
		t.Fatalf("server output = type %d payload %q", messageType, payload)
	}
	if err := conn.WriteMessage(websocket.TextMessage, []byte("text-")); err != nil {
		t.Fatalf("write text input: %v", err)
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, []byte("binary")); err != nil {
		t.Fatalf("write binary input: %v", err)
	}
	if err := conn.WriteControl(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
		time.Now().Add(time.Second),
	); err != nil {
		t.Fatalf("close websocket: %v", err)
	}

	select {
	case serverResult := <-result:
		if serverResult.err != nil {
			t.Fatalf("server pumps: %v", serverResult.err)
		}
		if serverResult.input != "text-binary" {
			t.Fatalf("server input = %q", serverResult.input)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("websocket pumps did not stop")
	}
}
