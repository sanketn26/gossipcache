package wire

import "strconv"

// Status is the closed set of data-plane result codes.
type Status uint16

const (
	StatusOK                       Status = 0
	StatusNotFound                 Status = 1
	StatusNotCaughtUp              Status = 2
	StatusErrDurabilityUnavailable Status = 3
	StatusErrBadGeneration         Status = 4
	StatusErrRateLimited           Status = 5
	StatusErrInvalidArgument       Status = 6
	StatusErrWriteConfirmTimeout   Status = 7
	StatusErrInternal              Status = 8
)

// Valid reports whether s is a status defined by this protocol version.
func (s Status) Valid() bool {
	return s <= StatusErrInternal
}

// String returns the stable protocol spelling of s.
func (s Status) String() string {
	switch s {
	case StatusOK:
		return "OK"
	case StatusNotFound:
		return "NOT_FOUND"
	case StatusNotCaughtUp:
		return "NOT_CAUGHT_UP"
	case StatusErrDurabilityUnavailable:
		return "ERR_DURABILITY_UNAVAILABLE"
	case StatusErrBadGeneration:
		return "ERR_BAD_GENERATION"
	case StatusErrRateLimited:
		return "ERR_RATE_LIMITED"
	case StatusErrInvalidArgument:
		return "ERR_INVALID_ARGUMENT"
	case StatusErrWriteConfirmTimeout:
		return "ERR_WRITE_CONFIRM_TIMEOUT"
	case StatusErrInternal:
		return "ERR_INTERNAL"
	default:
		return "Status(" + strconv.FormatUint(uint64(s), 10) + ")"
	}
}
