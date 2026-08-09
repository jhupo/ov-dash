package servers

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

const (
	webSocketReadLimit  = 1 << 20
	webSocketWriteWait  = 10 * time.Second
	webSocketPongWait   = 60 * time.Second
	webSocketPingPeriod = 25 * time.Second
	webSocketCloseWait  = time.Second
)

type webSocketMessage struct {
	messageType int
	payload     []byte
}

type terminalResizeHandler func([]byte) (bool, error)

func isWebSocketUpgradeRequest(r *http.Request) bool {
	return websocket.IsWebSocketUpgrade(r)
}

func webSocketOriginAllowed(r *http.Request, allowedOrigins []string) bool {
	origin, ok := canonicalWebSocketOrigin(r.Header.Get("Origin"))
	if !ok {
		return false
	}
	originURL, _ := url.Parse(origin)
	if strings.EqualFold(originURL.Host, strings.TrimSpace(r.Host)) {
		return true
	}
	for _, allowed := range allowedOrigins {
		candidate, valid := canonicalWebSocketOrigin(allowed)
		if valid && strings.EqualFold(candidate, origin) {
			return true
		}
	}
	return false
}

func canonicalWebSocketOrigin(value string) (string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.User != nil || parsed.Host == "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", false
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return "", false
	}
	return strings.ToLower(parsed.Scheme) + "://" + strings.ToLower(parsed.Host), true
}

func upgradeWebSocket(w http.ResponseWriter, r *http.Request, allowedOrigins []string) (*websocket.Conn, error) {
	upgrader := websocket.Upgrader{
		HandshakeTimeout: webSocketWriteWait,
		ReadBufferSize:   4096,
		WriteBufferSize:  4096,
		CheckOrigin: func(request *http.Request) bool {
			return webSocketOriginAllowed(request, allowedOrigins)
		},
	}
	return upgrader.Upgrade(w, r, nil)
}

func writeWebSocketPump(
	ctx context.Context,
	conn *websocket.Conn,
	outbound <-chan webSocketMessage,
	control <-chan webSocketMessage,
) error {
	ticker := time.NewTicker(webSocketPingPeriod)
	defer ticker.Stop()

	for {
		select {
		case message := <-control:
			if err := writeWebSocketMessage(conn, message); err != nil {
				return err
			}
			continue
		default:
		}

		select {
		case <-ctx.Done():
			deadline := time.Now().Add(webSocketCloseWait)
			_ = conn.WriteControl(
				websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
				deadline,
			)
			return ctx.Err()
		case message := <-control:
			if err := writeWebSocketMessage(conn, message); err != nil {
				return err
			}
		case message := <-outbound:
			if err := writeWebSocketMessage(conn, message); err != nil {
				return err
			}
		case <-ticker.C:
			if err := writeWebSocketMessage(conn, webSocketMessage{messageType: websocket.PingMessage}); err != nil {
				return err
			}
		}
	}
}

func writeWebSocketMessage(conn *websocket.Conn, message webSocketMessage) error {
	if err := conn.SetWriteDeadline(time.Now().Add(webSocketWriteWait)); err != nil {
		return err
	}
	return conn.WriteMessage(message.messageType, message.payload)
}

func readWebSocketPump(
	ctx context.Context,
	conn *websocket.Conn,
	stdin io.Writer,
	resize terminalResizeHandler,
	control chan<- webSocketMessage,
) error {
	conn.SetReadLimit(webSocketReadLimit)
	if err := conn.SetReadDeadline(time.Now().Add(webSocketPongWait)); err != nil {
		return err
	}
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(webSocketPongWait))
	})
	conn.SetPingHandler(func(payload string) error {
		return enqueueWebSocketMessage(ctx, control, webSocketMessage{
			messageType: websocket.PongMessage,
			payload:     []byte(payload),
		})
	})
	conn.SetCloseHandler(func(int, string) error {
		return nil
	})

	for {
		messageType, payload, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		if messageType != websocket.TextMessage && messageType != websocket.BinaryMessage {
			continue
		}
		if resize != nil {
			handled, resizeErr := resize(payload)
			if handled {
				if resizeErr != nil {
					return resizeErr
				}
				continue
			}
		}
		if _, err := stdin.Write(payload); err != nil {
			return err
		}
	}
}

func enqueueWebSocketMessage(ctx context.Context, target chan<- webSocketMessage, message webSocketMessage) error {
	select {
	case target <- message:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func copySSHOutput(ctx context.Context, reader io.Reader, outbound chan<- webSocketMessage) error {
	buffer := make([]byte, 32*1024)
	for {
		count, err := reader.Read(buffer)
		if count > 0 {
			payload := append([]byte(nil), buffer[:count]...)
			if sendErr := enqueueWebSocketMessage(ctx, outbound, webSocketMessage{
				messageType: websocket.BinaryMessage,
				payload:     payload,
			}); sendErr != nil {
				return sendErr
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}

func writeWebSocketError(conn *websocket.Conn, code string) {
	_ = conn.SetWriteDeadline(time.Now().Add(webSocketWriteWait))
	_ = conn.WriteMessage(websocket.BinaryMessage, []byte(code+"\r\n"))
	_ = conn.WriteControl(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseInternalServerErr, code),
		time.Now().Add(webSocketCloseWait),
	)
}
