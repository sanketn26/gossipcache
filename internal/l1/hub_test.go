package l1_test

import (
	"errors"
	"testing"
	"time"

	"github.com/sanketn26/gossipcache/internal/l1"
	"github.com/sanketn26/gossipcache/internal/wire"
)

func TestGetResultValidate(t *testing.T) {
	t.Parallel()

	version := wire.VersionTag{PartitionID: 2, Sequence: 7}
	tests := []struct {
		name    string
		result  l1.GetResult
		wantErr error
	}{
		{
			name: "value",
			result: l1.GetResult{
				Value: []byte("value"), Version: version, Kind: l1.RecordValue,
				TTL: time.Second, Status: wire.StatusOK,
			},
		},
		{
			name: "versioned tombstone",
			result: l1.GetResult{
				Version: version, Kind: l1.RecordTombstone, Status: wire.StatusOK,
			},
		},
		{name: "not found has no version", result: l1.GetResult{Status: wire.StatusNotFound}},
		{name: "not caught up has no record", result: l1.GetResult{Status: wire.StatusNotCaughtUp}},
		{name: "terminal error has no record", result: l1.GetResult{Status: wire.StatusErrInternal}},
		{
			name:    "OK requires version",
			result:  l1.GetResult{Kind: l1.RecordValue, Status: wire.StatusOK},
			wantErr: l1.ErrMissingVersion,
		},
		{
			name: "not found rejects version",
			result: l1.GetResult{
				Version: version, Status: wire.StatusNotFound,
			},
			wantErr: l1.ErrUnexpectedVersion,
		},
		{
			name: "not caught up rejects value",
			result: l1.GetResult{
				Value: []byte("value"), Status: wire.StatusNotCaughtUp,
			},
			wantErr: l1.ErrUnexpectedValue,
		},
		{
			name:    "record rejects invalid kind",
			result:  l1.GetResult{Version: version, Kind: 20, Status: wire.StatusOK},
			wantErr: l1.ErrInvalidRecordKind,
		},
		{
			name: "tombstone rejects value",
			result: l1.GetResult{
				Value: []byte("value"), Version: version, Kind: l1.RecordTombstone, Status: wire.StatusOK,
			},
			wantErr: l1.ErrUnexpectedValue,
		},
		{
			name:    "negative ttl",
			result:  l1.GetResult{Version: version, Kind: l1.RecordValue, TTL: -1, Status: wire.StatusOK},
			wantErr: l1.ErrInvalidResultTTL,
		},
		{
			name:    "non-record rejects ttl",
			result:  l1.GetResult{TTL: time.Second, Status: wire.StatusNotFound},
			wantErr: l1.ErrInvalidResultTTL,
		},
		{
			name:    "write-only status rejected",
			result:  l1.GetResult{Status: wire.StatusErrWriteConfirmTimeout},
			wantErr: l1.ErrInvalidGetStatus,
		},
		{
			name:    "unknown status rejected",
			result:  l1.GetResult{Status: wire.Status(99)},
			wantErr: l1.ErrInvalidGetStatus,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.result.Validate()
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Validate() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestGetResultCloneCopiesValue(t *testing.T) {
	t.Parallel()

	original := l1.GetResult{Value: []byte("value")}
	cloned := original.Clone()
	cloned.Value[0] = 'V'
	if string(original.Value) != "value" {
		t.Fatalf("Clone() aliased original value: %q", original.Value)
	}
}

func TestMutationCloneCopiesBytes(t *testing.T) {
	t.Parallel()

	original := l1.Mutation{Key: []byte("key"), Value: []byte("value")}
	cloned := original.Clone()
	cloned.Key[0] = 'K'
	cloned.Value[0] = 'V'
	if string(original.Key) != "key" || string(original.Value) != "value" {
		t.Fatalf("Clone() aliased original mutation: key=%q value=%q", original.Key, original.Value)
	}
}
