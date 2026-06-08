package queue

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

type Job struct {
	ID             string         `json:"id"`
	Type           string         `json:"type"`
	Payload        map[string]any `json:"payload"`
	IdempotencyKey string         `json:"idempotency_key,omitempty"`
	Attempts       int            `json:"attempts"`
	MaxAttempts    int            `json:"max_attempts"`
	CreatedAt      time.Time      `json:"created_at"`
}

const DefaultMaxAttempts = 3

var (
	ErrDuplicateIdempotencyKey = errors.New("duplicate idempotency key")
	ErrJobNotRequeueable       = errors.New("job is not requeueable")
)

func NewJob(jobType string, payload map[string]any) (Job, error) {
	if jobType == "" {
		return Job{}, errors.New("job type is required")
	}
	if payload == nil {
		payload = map[string]any{}
	}

	id, err := randomID()
	if err != nil {
		return Job{}, err
	}

	return Job{
		ID:          id,
		Type:        jobType,
		Payload:     payload,
		MaxAttempts: DefaultMaxAttempts,
		CreatedAt:   time.Now().UTC(),
	}, nil
}

func (j *Job) Normalize() {
	j.IdempotencyKey = strings.TrimSpace(j.IdempotencyKey)
	if j.Payload == nil {
		j.Payload = map[string]any{}
	}
	if j.MaxAttempts < 1 {
		j.MaxAttempts = DefaultMaxAttempts
	}
	if j.CreatedAt.IsZero() {
		j.CreatedAt = time.Now().UTC()
	}
}

func randomID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
