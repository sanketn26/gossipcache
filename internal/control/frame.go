package control

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"reflect"

	"github.com/sanketn26/gossipcache/internal/wire"
)

const frameMagic uint32 = 0x47435331

var crc32cTable = crc32.MakeTable(crc32.Castagnoli)

// Header is the decoded fixed portion of a control frame.
type Header struct {
	Version    wire.ProtocolVersion
	Type       MessageType
	PayloadLen uint32
}

// MarshalFrame validates and encodes one complete control frame.
func MarshalFrame(message Message) ([]byte, error) {
	if message == nil || (reflect.ValueOf(message).Kind() == reflect.Pointer && reflect.ValueOf(message).IsNil()) {
		return nil, fmt.Errorf("%w: nil", ErrInvalidMessage)
	}
	version := wire.CurrentProtocolVersion
	if message.Type() == MessageHello {
		// Hello uses the oldest locally supported schema so an older compatible
		// peer can decode the advertised range and negotiate a shared version.
		version = wire.MinSupportedProtocolVersion
	}
	return MarshalFrameVersion(message, version)
}

// MarshalFrameVersion validates and encodes one complete control frame using
// a specific locally supported schema version. Connection owners use this
// after Hello negotiation when the selected version is older than current.
func MarshalFrameVersion(message Message, version wire.ProtocolVersion) ([]byte, error) {
	if message == nil || (reflect.ValueOf(message).Kind() == reflect.Pointer && reflect.ValueOf(message).IsNil()) {
		return nil, fmt.Errorf("%w: nil", ErrInvalidMessage)
	}
	if !supportedVersion(version) {
		return nil, fmt.Errorf("%w: %d", ErrUnsupportedVersion, version)
	}
	if err := message.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidMessage, err)
	}
	payload, err := marshalPayloadVersion(version, message)
	if err != nil {
		return nil, err
	}
	if len(payload) > MaxControlPayload {
		return nil, ErrPayloadTooLarge
	}

	frame := make([]byte, HeaderSize+len(payload))
	binary.BigEndian.PutUint32(frame[0:4], frameMagic)
	binary.BigEndian.PutUint16(frame[4:6], uint16(version))
	binary.BigEndian.PutUint16(frame[6:8], uint16(message.Type()))
	binary.BigEndian.PutUint32(frame[8:12], uint32(len(payload)))
	binary.BigEndian.PutUint32(frame[12:16], crc32.Checksum(frame[:12], crc32cTable))
	copy(frame[HeaderSize:], payload)
	return frame, nil
}

// UnmarshalFrame decodes exactly one complete frame. Unknown schemas and
// malformed headers fail closed.
func UnmarshalFrame(frame []byte) (Message, error) {
	return unmarshalFrame(frame, nil)
}

// UnmarshalFrameVersion decodes one complete frame and requires the negotiated
// protocol version.
func UnmarshalFrameVersion(frame []byte, version wire.ProtocolVersion) (Message, error) {
	if !supportedVersion(version) {
		return nil, fmt.Errorf("%w: %d", ErrUnsupportedVersion, version)
	}
	return unmarshalFrame(frame, &version)
}

func unmarshalFrame(frame []byte, expectedVersion *wire.ProtocolVersion) (Message, error) {
	if len(frame) < HeaderSize {
		return nil, ErrTruncatedFrame
	}
	header, err := parseHeader(frame[:HeaderSize])
	if err != nil {
		return nil, err
	}
	if expectedVersion != nil && header.Version != *expectedVersion {
		return nil, fmt.Errorf("%w: got %d, want %d", ErrUnsupportedVersion, header.Version, *expectedVersion)
	}
	total := HeaderSize + int(header.PayloadLen)
	if len(frame) < total {
		return nil, ErrTruncatedFrame
	}
	if len(frame) > total {
		return nil, ErrTrailingPayload
	}
	return unmarshalPayloadVersion(header.Version, header.Type, frame[HeaderSize:])
}

// ReadFrame reads and decodes one frame from a stream.
func ReadFrame(reader io.Reader) (Message, error) {
	return readFrame(reader, nil)
}

// ReadFrameVersion reads one frame and requires the negotiated protocol
// version.
func ReadFrameVersion(reader io.Reader, version wire.ProtocolVersion) (Message, error) {
	if !supportedVersion(version) {
		return nil, fmt.Errorf("%w: %d", ErrUnsupportedVersion, version)
	}
	return readFrame(reader, &version)
}

func readFrame(reader io.Reader, expectedVersion *wire.ProtocolVersion) (Message, error) {
	headerBytes := make([]byte, HeaderSize)
	if _, err := io.ReadFull(reader, headerBytes); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, fmt.Errorf("%w: %w", ErrTruncatedFrame, err)
		}
		return nil, err
	}
	header, err := parseHeader(headerBytes)
	if err != nil {
		return nil, err
	}
	if expectedVersion != nil && header.Version != *expectedVersion {
		return nil, fmt.Errorf("%w: got %d, want %d", ErrUnsupportedVersion, header.Version, *expectedVersion)
	}
	payload := make([]byte, header.PayloadLen)
	if _, err := io.ReadFull(reader, payload); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, fmt.Errorf("%w: %w", ErrTruncatedFrame, err)
		}
		return nil, err
	}
	return unmarshalPayloadVersion(header.Version, header.Type, payload)
}

// WriteFrame writes one validated frame to a stream.
func WriteFrame(writer io.Writer, message Message) error {
	frame, err := MarshalFrame(message)
	if err != nil {
		return err
	}
	return writeAll(writer, frame)
}

// WriteFrameVersion writes one validated frame using the negotiated protocol
// version.
func WriteFrameVersion(writer io.Writer, message Message, version wire.ProtocolVersion) error {
	frame, err := MarshalFrameVersion(message, version)
	if err != nil {
		return err
	}
	return writeAll(writer, frame)
}

func writeAll(writer io.Writer, frame []byte) error {
	for len(frame) > 0 {
		written, writeErr := writer.Write(frame)
		if writeErr != nil {
			return writeErr
		}
		if written == 0 {
			return io.ErrShortWrite
		}
		frame = frame[written:]
	}
	return nil
}

func parseHeader(encoded []byte) (Header, error) {
	if len(encoded) != HeaderSize {
		return Header{}, ErrTruncatedFrame
	}
	if binary.BigEndian.Uint32(encoded[0:4]) != frameMagic {
		return Header{}, ErrBadMagic
	}
	if crc32.Checksum(encoded[:12], crc32cTable) != binary.BigEndian.Uint32(encoded[12:16]) {
		return Header{}, ErrBadHeaderCRC
	}
	version := wire.ProtocolVersion(binary.BigEndian.Uint16(encoded[4:6]))
	if !supportedVersion(version) {
		return Header{}, fmt.Errorf("%w: %d", ErrUnsupportedVersion, version)
	}
	messageType := MessageType(binary.BigEndian.Uint16(encoded[6:8]))
	if !messageType.Valid() {
		return Header{}, fmt.Errorf("%w: %d", ErrUnknownMessageType, messageType)
	}
	payloadLen := binary.BigEndian.Uint32(encoded[8:12])
	if payloadLen > MaxControlPayload {
		return Header{}, fmt.Errorf("%w: %d", ErrPayloadTooLarge, payloadLen)
	}
	return Header{Version: version, Type: messageType, PayloadLen: payloadLen}, nil
}

func supportedVersion(version wire.ProtocolVersion) bool {
	current := wire.CurrentProtocolRange()
	if version < current.MinSupported || version > current.Version {
		return false
	}
	// Keep schema support explicit: raising CurrentProtocolVersion without
	// adding the matching codec must fail closed rather than silently decoding
	// a new schema as an old one.
	switch version {
	case 1:
		return true
	default:
		return false
	}
}
