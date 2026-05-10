package gossipcache

import (
	"errors"
	"testing"
)

func TestSentinelErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		msg  string
	}{
		{name: "key not found", err: ErrKeyNotFound, msg: "key not found"},
		{name: "key too large", err: ErrKeyTooLarge, msg: "key too large"},
		{name: "value too large", err: ErrValueTooLarge, msg: "value too large"},
		{name: "cache full", err: ErrCacheFull, msg: "cache full"},
		{name: "closed", err: ErrClosed, msg: "cache closed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err == nil {
				t.Fatal("error is nil")
			}
			if got := tt.err.Error(); got != tt.msg {
				t.Fatalf("Error() = %q, want %q", got, tt.msg)
			}
			if !errors.Is(tt.err, tt.err) {
				t.Fatalf("errors.Is(%v, %v) = false, want true", tt.err, tt.err)
			}
		})
	}
}
