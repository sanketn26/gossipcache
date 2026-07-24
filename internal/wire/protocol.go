package wire

import "fmt"

// ProtocolVersion is a data/control protocol compatibility version.
type ProtocolVersion uint16

const (
	// CurrentProtocolVersion is emitted by this implementation.
	CurrentProtocolVersion ProtocolVersion = 1
	// MinSupportedProtocolVersion is the oldest accepted peer version.
	MinSupportedProtocolVersion ProtocolVersion = 1
)

// ProtocolRange is exchanged during handshake so both peers can fail closed
// when they do not share a protocol version.
type ProtocolRange struct {
	Version      ProtocolVersion
	MinSupported ProtocolVersion
}

// CurrentProtocolRange returns this implementation's supported range.
func CurrentProtocolRange() ProtocolRange {
	return ProtocolRange{
		Version:      CurrentProtocolVersion,
		MinSupported: MinSupportedProtocolVersion,
	}
}

// Validate checks that the range is non-zero and ordered.
func (r ProtocolRange) Validate() error {
	if r.MinSupported == 0 || r.Version == 0 || r.MinSupported > r.Version {
		return fmt.Errorf("%w: min=%d version=%d", ErrInvalidProtocolRange, r.MinSupported, r.Version)
	}
	return nil
}

// Negotiate selects the highest mutually supported protocol version.
func (r ProtocolRange) Negotiate(peer ProtocolRange) (ProtocolVersion, error) {
	if err := r.Validate(); err != nil {
		return 0, err
	}
	if err := peer.Validate(); err != nil {
		return 0, err
	}

	version := min(r.Version, peer.Version)
	minimum := max(r.MinSupported, peer.MinSupported)
	if version < minimum {
		return 0, fmt.Errorf(
			"%w: local=%d-%d peer=%d-%d",
			ErrIncompatibleProtocolRanges,
			r.MinSupported,
			r.Version,
			peer.MinSupported,
			peer.Version,
		)
	}
	return version, nil
}
