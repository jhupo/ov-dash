package servers

import "testing"

func TestCollectionRetryDelaySecondsBacksOffAndCaps(t *testing.T) {
	tests := []struct {
		name         string
		interval     int
		failureCount int
		want         int
	}{
		{name: "first failure uses interval", interval: 60, failureCount: 1, want: 60},
		{name: "second failure doubles", interval: 60, failureCount: 2, want: 120},
		{name: "minimum interval", interval: 1, failureCount: 1, want: 10},
		{name: "zero failure count normalizes", interval: 30, failureCount: 0, want: 30},
		{name: "caps at one hour", interval: 300, failureCount: 9, want: 3600},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := collectionRetryDelaySeconds(tt.interval, tt.failureCount)
			if got != tt.want {
				t.Fatalf("collectionRetryDelaySeconds(%d, %d) = %d, want %d", tt.interval, tt.failureCount, got, tt.want)
			}
		})
	}
}
