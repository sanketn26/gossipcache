// Package l1 contains the node's local-cache coordination.
package l1

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/sanketn26/gossipcache/internal/wire"
)

var (
	ErrInvalidGetStatus  = errors.New("invalid status for Get result")
	ErrMissingVersion    = errors.New("successful record is missing a version")
	ErrUnexpectedVersion = errors.New("result status must not carry a version")
	ErrUnexpectedValue   = errors.New("result status must not carry a value")
	ErrInvalidRecordKind = errors.New("invalid record kind")
	ErrInvalidResultTTL  = errors.New("invalid result ttl")
)

// WriteOptions is the shared write-policy contract.
type WriteOptions = wire.WriteOptions

// RecordKind is the shared value-or-tombstone discriminator.
type RecordKind = wire.RecordKind

const (
	// RecordValue contains a live cache value.
	RecordValue = wire.RecordValue
	// RecordTombstone records an authoritative deletion.
	RecordTombstone = wire.RecordTombstone
)

// HubClient is the authoritative data-plane seam consumed by the node state
// machine.
type HubClient interface {
	Get(ctx context.Context, key []byte, min *wire.VersionTag) (GetResult, error)
	Set(ctx context.Context, key, value []byte, ttl time.Duration, opt WriteOptions) (wire.VersionTag, error)
	Delete(ctx context.Context, key []byte, opt WriteOptions) (wire.VersionTag, error)
}

// GetResult is an authoritative read result. StatusNotFound carries no version;
// a deleted key is instead StatusOK with RecordTombstone and a real version.
type GetResult struct {
	Value         []byte
	HubGeneration uint64
	Version       wire.VersionTag
	TTL           time.Duration
	Kind          RecordKind
	Status        wire.Status
}

// Clone returns an ownership-independent result.
func (r GetResult) Clone() GetResult {
	r.Value = wire.CopyBytes(r.Value)
	return r
}

// Validate checks the frozen authoritative-read response semantics. Not-found,
// not-caught-up, and error statuses never carry record data.
func (r GetResult) Validate() error {
	if !r.Status.Valid() || r.Status == wire.StatusErrWriteConfirmTimeout {
		return fmt.Errorf("%w: %s", ErrInvalidGetStatus, r.Status)
	}
	if r.TTL < 0 {
		return ErrInvalidResultTTL
	}
	if r.Status == wire.StatusOK {
		if r.Version.IsZero() {
			return ErrMissingVersion
		}
		if !r.Kind.Valid() {
			return fmt.Errorf("%w: %d", ErrInvalidRecordKind, r.Kind)
		}
		if r.Kind == wire.RecordTombstone && len(r.Value) != 0 {
			return ErrUnexpectedValue
		}
		return nil
	}
	if !r.Version.IsZero() {
		return ErrUnexpectedVersion
	}
	if len(r.Value) != 0 {
		return ErrUnexpectedValue
	}
	if r.TTL != 0 {
		return ErrInvalidResultTTL
	}
	return nil
}
