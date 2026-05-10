package storage

import (
	"testing"
	"time"
)

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
