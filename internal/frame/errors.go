// Package frame provides shared binary framing primitives for GossipCache
// control and data-plane protocols.
//
// Protocol packages (control, rpc) own message schemas, magics, and header
// layouts. This package owns the mechanical pieces they would otherwise
// duplicate: big-endian codec helpers, CRC32C header sealing, and stream I/O.
package frame

import "errors"

var (
	// ErrTruncated reports that a header or payload ended before the declared
	// length. Protocol packages typically alias this as ErrTruncatedFrame.
	ErrTruncated = errors.New("truncated frame")
	// ErrTrailing reports bytes remaining after a complete payload decode.
	// Protocol packages typically alias this as ErrTrailingPayload.
	ErrTrailing = errors.New("frame has trailing bytes")
	// ErrInvalidBool reports a bool field that was not 0 or 1.
	ErrInvalidBool = errors.New("invalid bool encoding")
)
