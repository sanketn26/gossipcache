package wire

import (
	"errors"
	"fmt"
)

// Shared authoritative-record validation used by the RPC GetResponse and L1
// GetResult seams. Kept out of types.go so the core wire models stay focused.

var (
	ErrInvalidGetStatus  = errors.New("invalid status for get result")
	ErrMissingVersion    = errors.New("successful record is missing a version")
	ErrUnexpectedVersion = errors.New("result status must not carry a version")
	ErrUnexpectedValue   = errors.New("result status must not carry a value")
	ErrInvalidRecordKind = errors.New("invalid record kind")
	// ErrInvalidResultTTL is returned by callers that enforce TTL shape
	// (duration vs millis) outside GetRecordFields.
	ErrInvalidResultTTL = errors.New("invalid result ttl")
)

// ValidateCommitted checks that v is a hub-assigned committed version.
// Hub partition sequences begin at one; a zero sequence is never a commit.
func (v VersionTag) ValidateCommitted() error {
	if v.Sequence == 0 {
		return ErrMissingVersion
	}
	return nil
}

// GetRecordFields is the shared authoritative-read record shape used by the
// RPC GetResponse and the L1 GetResult seams.
type GetRecordFields struct {
	Status  Status
	Version VersionTag
	Kind    RecordKind
	Value   []byte
}

// Validate checks frozen get-record semantics. StatusOK requires a committed
// version and a valid value-or-tombstone; all other get statuses carry no
// record data. Callers enforce TTL representation and hub generation separately.
func (r GetRecordFields) Validate() error {
	if !r.Status.Valid() || r.Status == StatusErrWriteConfirmTimeout {
		return fmt.Errorf("%w: %s", ErrInvalidGetStatus, r.Status)
	}
	if r.Status == StatusOK {
		if err := r.Version.ValidateCommitted(); err != nil {
			return err
		}
		if !r.Kind.Valid() {
			return fmt.Errorf("%w: %d", ErrInvalidRecordKind, r.Kind)
		}
		if r.Kind == RecordTombstone && len(r.Value) != 0 {
			return ErrUnexpectedValue
		}
		if len(r.Value) > MaxValueLen {
			return fmt.Errorf("%w: got %d, max %d", ErrValueTooLarge, len(r.Value), MaxValueLen)
		}
		return nil
	}
	if !r.Version.IsZero() {
		return ErrUnexpectedVersion
	}
	if len(r.Value) != 0 {
		return ErrUnexpectedValue
	}
	if r.Kind != 0 {
		return fmt.Errorf("%w: %d", ErrInvalidRecordKind, r.Kind)
	}
	return nil
}
