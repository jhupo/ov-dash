package events

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

type Event struct {
	ID        string         `json:"id"`
	Type      string         `json:"type"`
	Source    string         `json:"source"`
	Payload   map[string]any `json:"payload"`
	CreatedAt time.Time      `json:"created_at"`
}

func New(eventType string, source string, payload map[string]any) Event {
	if payload == nil {
		payload = map[string]any{}
	}
	return Event{
		ID:        randomID(),
		Type:      eventType,
		Source:    source,
		Payload:   payload,
		CreatedAt: time.Now().UTC(),
	}
}

func randomID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return time.Now().UTC().Format("20060102150405.000000000")
	}
	return hex.EncodeToString(b[:])
}
