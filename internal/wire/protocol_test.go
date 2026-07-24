package wire_test

import (
	"errors"
	"testing"

	"github.com/sanketn26/gossipcache/internal/wire"
)

func TestProtocolNegotiation(t *testing.T) {
	t.Parallel()

	local := wire.ProtocolRange{MinSupported: 2, Version: 4}
	tests := []struct {
		name string
		peer wire.ProtocolRange
		want wire.ProtocolVersion
		err  error
	}{
		{name: "same", peer: wire.ProtocolRange{MinSupported: 2, Version: 4}, want: 4},
		{name: "older overlap", peer: wire.ProtocolRange{MinSupported: 1, Version: 3}, want: 3},
		{name: "newer overlap", peer: wire.ProtocolRange{MinSupported: 3, Version: 5}, want: 4},
		{
			name: "incompatible",
			peer: wire.ProtocolRange{MinSupported: 5, Version: 6},
			err:  wire.ErrIncompatibleProtocolRanges,
		},
		{
			name: "invalid",
			peer: wire.ProtocolRange{MinSupported: 3, Version: 2},
			err:  wire.ErrInvalidProtocolRange,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := local.Negotiate(test.peer)
			if !errors.Is(err, test.err) {
				t.Fatalf("Negotiate() error = %v, want %v", err, test.err)
			}
			if got != test.want {
				t.Fatalf("Negotiate() = %d, want %d", got, test.want)
			}
		})
	}
}

func TestCurrentProtocolRangeIsValid(t *testing.T) {
	t.Parallel()

	current := wire.CurrentProtocolRange()
	if current.Version != wire.CurrentProtocolVersion {
		t.Fatalf("current version = %d, want %d", current.Version, wire.CurrentProtocolVersion)
	}
	if current.MinSupported != wire.MinSupportedProtocolVersion {
		t.Fatalf("minimum version = %d, want %d", current.MinSupported, wire.MinSupportedProtocolVersion)
	}
	if err := current.Validate(); err != nil {
		t.Fatalf("current range is invalid: %v", err)
	}
}

func TestStatusValuesAndNames(t *testing.T) {
	t.Parallel()

	statuses := []struct {
		status wire.Status
		value  uint16
		name   string
	}{
		{wire.StatusOK, 0, "OK"},
		{wire.StatusNotFound, 1, "NOT_FOUND"},
		{wire.StatusNotCaughtUp, 2, "NOT_CAUGHT_UP"},
		{wire.StatusErrDurabilityUnavailable, 3, "ERR_DURABILITY_UNAVAILABLE"},
		{wire.StatusErrBadGeneration, 4, "ERR_BAD_GENERATION"},
		{wire.StatusErrRateLimited, 5, "ERR_RATE_LIMITED"},
		{wire.StatusErrInvalidArgument, 6, "ERR_INVALID_ARGUMENT"},
		{wire.StatusErrWriteConfirmTimeout, 7, "ERR_WRITE_CONFIRM_TIMEOUT"},
		{wire.StatusErrInternal, 8, "ERR_INTERNAL"},
	}

	for _, test := range statuses {
		if uint16(test.status) != test.value {
			t.Errorf("%s value = %d, want %d", test.name, test.status, test.value)
		}
		if got := test.status.String(); got != test.name {
			t.Errorf("Status(%d).String() = %q, want %q", test.status, got, test.name)
		}
		if !test.status.Valid() {
			t.Errorf("%s is not valid", test.name)
		}
	}
	if wire.Status(9).Valid() {
		t.Fatal("unknown status reported valid")
	}
	if got := wire.Status(9).String(); got != "Status(9)" {
		t.Fatalf("unknown status string = %q", got)
	}
}
