package storage

import (
	"testing"
	"time"
)

func TestEntryIsExpiredAt(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	tests := []struct {
		name      string
		expiresAt time.Time
		now       time.Time
		want      bool
	}{
		{name: "no expiration", expiresAt: time.Time{}, now: now, want: false},
		{name: "future expiration", expiresAt: now.Add(time.Hour), now: now, want: false},
		{name: "past expiration", expiresAt: now.Add(-time.Hour), now: now, want: true},
		{name: "exactly at expiration", expiresAt: now, now: now, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := &Entry{ExpiresAt: tt.expiresAt}

			if got := entry.IsExpiredAt(tt.now); got != tt.want {
				t.Fatalf("IsExpiredAt(%v) = %t, want %t", tt.now, got, tt.want)
			}
		})
	}
}

func TestEntryIsExpired(t *testing.T) {
	tests := []struct {
		name      string
		expiresAt time.Time
		want      bool
	}{
		{
			name:      "no expiration",
			expiresAt: time.Time{},
			want:      false,
		},
		{
			name:      "future expiration",
			expiresAt: time.Now().Add(time.Hour),
			want:      false,
		},
		{
			name:      "past expiration",
			expiresAt: time.Now().Add(-time.Hour),
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := &Entry{ExpiresAt: tt.expiresAt}

			if got := entry.IsExpired(); got != tt.want {
				t.Fatalf("IsExpired() = %t, want %t", got, tt.want)
			}
		})
	}
}
