package rpc

import (
	"bytes"
	"time"

	"github.com/sanketn26/gossipcache/internal/wire"
)

// DefaultDedupWindow is how long a hub retains a per-node MutationID outcome
// so retries join the original waiter or replay its result.
const DefaultDedupWindow = 5 * time.Minute

// DedupKey scopes a MutationID by the authenticated node identity so IDs
// cannot collide across nodes. The hub retains outcomes for DefaultDedupWindow.
type DedupKey struct {
	NodeID     uint64
	MutationID wire.MutationID
}

// Validate checks that both components of the dedup key are present.
func (k DedupKey) Validate() error {
	if k.NodeID == 0 {
		return ErrInvalidNodeID
	}
	if k.MutationID.IsZero() {
		return wire.ErrMutationIDRequired
	}
	return nil
}

// MutationFingerprint is the immutable request content compared on MutationID
// reuse. Timeout is excluded so a retry may lengthen its wait without being
// rejected as a mismatched fingerprint.
type MutationFingerprint struct {
	Op        Op
	Key       []byte
	Value     []byte
	TTLMillis uint64
	Mode      wire.WriteMode
	W         uint16
	Confirm   wire.ConfirmLevel
}

// Fingerprint returns the immutable mutation fingerprint for dedup matching.
func Fingerprint(req MutationRequest) MutationFingerprint {
	return MutationFingerprint{
		Op:        req.Op,
		Key:       wire.CopyBytes(req.Key),
		Value:     wire.CopyBytes(req.Value),
		TTLMillis: req.TTLMillis,
		Mode:      req.Mode,
		W:         req.W,
		Confirm:   req.Confirm,
	}
}

// Equal reports whether two fingerprints describe the same mutation content.
func (f MutationFingerprint) Equal(other MutationFingerprint) bool {
	return f.Op == other.Op &&
		f.TTLMillis == other.TTLMillis &&
		f.Mode == other.Mode &&
		f.W == other.W &&
		f.Confirm == other.Confirm &&
		bytes.Equal(f.Key, other.Key) &&
		bytes.Equal(f.Value, other.Value)
}

// CheckMatch returns ErrMutationFingerprintMismatch when other does not match f.
// A mismatched reuse of a MutationID is a terminal invalid request and must not
// change the original mutation or waiter.
func (f MutationFingerprint) CheckMatch(other MutationFingerprint) error {
	if !f.Equal(other) {
		return ErrMutationFingerprintMismatch
	}
	return nil
}

// Clone returns an ownership-independent fingerprint.
func (f MutationFingerprint) Clone() MutationFingerprint {
	f.Key = wire.CopyBytes(f.Key)
	f.Value = wire.CopyBytes(f.Value)
	return f
}
