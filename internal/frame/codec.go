package frame

import (
	"encoding/binary"
	"fmt"

	"github.com/sanketn26/gossipcache/internal/wire"
)

// Encoder accumulates big-endian protocol fields into a contiguous buffer.
type Encoder struct {
	bytes []byte
}

// NewEncoder returns an empty encoder.
func NewEncoder() *Encoder {
	return &Encoder{}
}

// Bytes returns the accumulated payload. The slice is valid until the next
// mutating call on e.
func (e *Encoder) Bytes() []byte { return e.bytes }

// Reset clears the buffer for reuse.
func (e *Encoder) Reset() { e.bytes = e.bytes[:0] }

// Uint8 appends a single byte.
func (e *Encoder) Uint8(value uint8) { e.bytes = append(e.bytes, value) }

// Uint16 appends a big-endian uint16.
func (e *Encoder) Uint16(value uint16) { e.appendFixed(2, uint64(value)) }

// Uint32 appends a big-endian uint32.
func (e *Encoder) Uint32(value uint32) { e.appendFixed(4, uint64(value)) }

// Uint64 appends a big-endian uint64.
func (e *Encoder) Uint64(value uint64) { e.appendFixed(8, value) }

// Bool appends 1 for true and 0 for false.
func (e *Encoder) Bool(value bool) {
	if value {
		e.Uint8(1)
	} else {
		e.Uint8(0)
	}
}

// Raw appends bytes without a length prefix.
func (e *Encoder) Raw(value []byte) { e.bytes = append(e.bytes, value...) }

// LengthBytes appends a uint32 length followed by value.
func (e *Encoder) LengthBytes(value []byte) {
	e.Uint32(uint32(len(value)))
	e.Raw(value)
}

// VersionTag appends a wire.VersionTag as partition then sequence.
func (e *Encoder) VersionTag(v wire.VersionTag) {
	e.Uint32(v.PartitionID)
	e.Uint64(v.Sequence)
}

func (e *Encoder) appendFixed(size int, value uint64) {
	start := len(e.bytes)
	e.bytes = append(e.bytes, make([]byte, size)...)
	switch size {
	case 2:
		binary.BigEndian.PutUint16(e.bytes[start:], uint16(value))
	case 4:
		binary.BigEndian.PutUint32(e.bytes[start:], uint32(value))
	case 8:
		binary.BigEndian.PutUint64(e.bytes[start:], value)
	}
}

// Decoder reads big-endian protocol fields from a fixed buffer.
type Decoder struct {
	bytes     []byte
	offset    int
	truncated error
}

// NewDecoder returns a decoder over b. Truncation returns ErrTruncated unless
// overridden with WithTruncated.
func NewDecoder(b []byte) *Decoder {
	return &Decoder{bytes: b, truncated: ErrTruncated}
}

// WithTruncated sets the error returned when a read runs past the buffer.
// Protocol packages pass their plane-specific ErrTruncatedFrame so callers can
// match with errors.Is without mapping.
func (d *Decoder) WithTruncated(err error) *Decoder {
	if err != nil {
		d.truncated = err
	}
	return d
}

// Remaining reports unread bytes.
func (d *Decoder) Remaining() int { return len(d.bytes) - d.offset }

// Take returns the next size bytes or a truncation error.
func (d *Decoder) Take(size int) ([]byte, error) {
	if size < 0 || d.Remaining() < size {
		return nil, d.truncated
	}
	value := d.bytes[d.offset : d.offset+size]
	d.offset += size
	return value, nil
}

// Uint8 reads one byte.
func (d *Decoder) Uint8() (uint8, error) {
	value, err := d.Take(1)
	if err != nil {
		return 0, err
	}
	return value[0], nil
}

// Uint16 reads a big-endian uint16.
func (d *Decoder) Uint16() (uint16, error) {
	value, err := d.Take(2)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint16(value), nil
}

// Uint32 reads a big-endian uint32.
func (d *Decoder) Uint32() (uint32, error) {
	value, err := d.Take(4)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint32(value), nil
}

// Uint64 reads a big-endian uint64.
func (d *Decoder) Uint64() (uint64, error) {
	value, err := d.Take(8)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint64(value), nil
}

// Bool reads a 0/1 byte. Other values fail closed.
func (d *Decoder) Bool() (bool, error) {
	value, err := d.Uint8()
	if err != nil {
		return false, err
	}
	switch value {
	case 0:
		return false, nil
	case 1:
		return true, nil
	default:
		return false, fmt.Errorf("%w: %d", ErrInvalidBool, value)
	}
}

// LengthBytes reads a length-prefixed byte slice. oversize is returned when
// the declared length exceeds max (use wire.ErrKeyTooLarge / ErrValueTooLarge).
func (d *Decoder) LengthBytes(max int, oversize error) ([]byte, error) {
	length, err := d.Uint32()
	if err != nil {
		return nil, err
	}
	if length > uint32(max) {
		return nil, oversize
	}
	value, err := d.Take(int(length))
	if err != nil {
		return nil, err
	}
	return wire.CopyBytes(value), nil
}

// VersionTag reads a partition/sequence pair.
func (d *Decoder) VersionTag() (wire.VersionTag, error) {
	var v wire.VersionTag
	var err error
	v.PartitionID, err = d.Uint32()
	if err != nil {
		return v, err
	}
	v.Sequence, err = d.Uint64()
	return v, err
}

// Count reads a uint32 element count, rejecting values above maximum or that
// cannot fit in the remaining buffer given minimumSize bytes per element.
func (d *Decoder) Count(maximum, minimumSize int, limitError error) (int, error) {
	count, err := d.Uint32()
	if err != nil {
		return 0, err
	}
	if count > uint32(maximum) {
		return 0, limitError
	}
	if uint64(count)*uint64(minimumSize) > uint64(d.Remaining()) {
		return 0, d.truncated
	}
	return int(count), nil
}
