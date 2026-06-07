package worker

import (
	"testing"
	"time"
)

func TestRetryBackoff(t *testing.T) {
	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{attempt: 0, want: 5 * time.Second},
		{attempt: 1, want: 5 * time.Second},
		{attempt: 2, want: 10 * time.Second},
		{attempt: 3, want: 20 * time.Second},
		{attempt: 5, want: time.Minute},
		{attempt: 9, want: time.Minute},
	}

	for _, tt := range tests {
		if got := retryBackoff(tt.attempt); got != tt.want {
			t.Fatalf("retryBackoff(%d) = %s, want %s", tt.attempt, got, tt.want)
		}
	}
}
