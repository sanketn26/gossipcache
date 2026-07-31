package wire_test

import (
	"errors"
	"testing"

	"github.com/sanketn26/gossipcache/internal/wire"
)

func TestVersionTagValidateCommitted(t *testing.T) {
	t.Parallel()

	if err := (wire.VersionTag{}).ValidateCommitted(); !errors.Is(err, wire.ErrMissingVersion) {
		t.Fatalf("zero tag error = %v", err)
	}
	if err := (wire.VersionTag{PartitionID: 1, Sequence: 0}).ValidateCommitted(); !errors.Is(err, wire.ErrMissingVersion) {
		t.Fatalf("zero sequence error = %v", err)
	}
	if err := (wire.VersionTag{PartitionID: 0, Sequence: 1}).ValidateCommitted(); err != nil {
		t.Fatalf("sequence 1 rejected: %v", err)
	}
}

func TestGetRecordFieldsValidate(t *testing.T) {
	t.Parallel()

	version := wire.VersionTag{PartitionID: 2, Sequence: 7}
	maxValue := make([]byte, wire.MaxValueLen)
	oversized := make([]byte, wire.MaxValueLen+1)

	tests := []struct {
		name    string
		record  wire.GetRecordFields
		wantErr error
	}{
		{
			name: "ok value",
			record: wire.GetRecordFields{
				Status:  wire.StatusOK,
				Version: version,
				Kind:    wire.RecordValue,
				Value:   []byte("value"),
			},
		},
		{
			name: "ok empty value",
			record: wire.GetRecordFields{
				Status:  wire.StatusOK,
				Version: version,
				Kind:    wire.RecordValue,
			},
		},
		{
			name: "ok max value",
			record: wire.GetRecordFields{
				Status:  wire.StatusOK,
				Version: version,
				Kind:    wire.RecordValue,
				Value:   maxValue,
			},
		},
		{
			name: "ok tombstone",
			record: wire.GetRecordFields{
				Status:  wire.StatusOK,
				Version: version,
				Kind:    wire.RecordTombstone,
			},
		},
		{
			name: "ok partition zero with sequence",
			record: wire.GetRecordFields{
				Status:  wire.StatusOK,
				Version: wire.VersionTag{PartitionID: 0, Sequence: 1},
				Kind:    wire.RecordValue,
				Value:   []byte("v"),
			},
		},
		{name: "not found", record: wire.GetRecordFields{Status: wire.StatusNotFound}},
		{name: "not caught up", record: wire.GetRecordFields{Status: wire.StatusNotCaughtUp}},
		{name: "rate limited", record: wire.GetRecordFields{Status: wire.StatusErrRateLimited}},
		{name: "durability unavailable", record: wire.GetRecordFields{Status: wire.StatusErrDurabilityUnavailable}},
		{name: "bad generation", record: wire.GetRecordFields{Status: wire.StatusErrBadGeneration}},
		{name: "invalid argument", record: wire.GetRecordFields{Status: wire.StatusErrInvalidArgument}},
		{name: "internal", record: wire.GetRecordFields{Status: wire.StatusErrInternal}},
		{
			name:    "write confirm timeout not a get status",
			record:  wire.GetRecordFields{Status: wire.StatusErrWriteConfirmTimeout},
			wantErr: wire.ErrInvalidGetStatus,
		},
		{
			name:    "unknown status",
			record:  wire.GetRecordFields{Status: wire.Status(99)},
			wantErr: wire.ErrInvalidGetStatus,
		},
		{
			name: "ok missing version",
			record: wire.GetRecordFields{
				Status: wire.StatusOK,
				Kind:   wire.RecordValue,
				Value:  []byte("v"),
			},
			wantErr: wire.ErrMissingVersion,
		},
		{
			name: "ok zero sequence",
			record: wire.GetRecordFields{
				Status:  wire.StatusOK,
				Version: wire.VersionTag{PartitionID: 2, Sequence: 0},
				Kind:    wire.RecordValue,
				Value:   []byte("v"),
			},
			wantErr: wire.ErrMissingVersion,
		},
		{
			name: "ok invalid kind",
			record: wire.GetRecordFields{
				Status:  wire.StatusOK,
				Version: version,
				Kind:    20,
			},
			wantErr: wire.ErrInvalidRecordKind,
		},
		{
			name: "ok tombstone with value",
			record: wire.GetRecordFields{
				Status:  wire.StatusOK,
				Version: version,
				Kind:    wire.RecordTombstone,
				Value:   []byte("x"),
			},
			wantErr: wire.ErrUnexpectedValue,
		},
		{
			name: "ok value too large",
			record: wire.GetRecordFields{
				Status:  wire.StatusOK,
				Version: version,
				Kind:    wire.RecordValue,
				Value:   oversized,
			},
			wantErr: wire.ErrValueTooLarge,
		},
		{
			name: "not found with version",
			record: wire.GetRecordFields{
				Status:  wire.StatusNotFound,
				Version: version,
			},
			wantErr: wire.ErrUnexpectedVersion,
		},
		{
			name: "not caught up with value",
			record: wire.GetRecordFields{
				Status: wire.StatusNotCaughtUp,
				Value:  []byte("stale"),
			},
			wantErr: wire.ErrUnexpectedValue,
		},
		{
			name: "error with kind",
			record: wire.GetRecordFields{
				Status: wire.StatusErrInternal,
				Kind:   wire.RecordTombstone, // non-zero; RecordValue is 0 and looks empty
			},
			wantErr: wire.ErrInvalidRecordKind,
		},
		{
			name: "error with zero partition version",
			record: wire.GetRecordFields{
				Status:  wire.StatusNotFound,
				Version: wire.VersionTag{PartitionID: 1},
			},
			wantErr: wire.ErrUnexpectedVersion,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := test.record.Validate()
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("Validate() error = %v, want %v", err, test.wantErr)
			}
		})
	}
}
