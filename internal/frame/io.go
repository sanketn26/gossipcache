package frame

import (
	"errors"
	"fmt"
	"io"
)

// WriteAll writes every byte of frame to writer, retrying short writes.
func WriteAll(writer io.Writer, frame []byte) error {
	for len(frame) > 0 {
		written, err := writer.Write(frame)
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrShortWrite
		}
		frame = frame[written:]
	}
	return nil
}

// ReadExact reads exactly n bytes. EOF/unexpected EOF are wrapped as
// truncated so protocol callers can match with errors.Is.
func ReadExact(reader io.Reader, n int, truncated error) ([]byte, error) {
	if truncated == nil {
		truncated = ErrTruncated
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(reader, buf); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, fmt.Errorf("%w: %w", truncated, err)
		}
		return nil, err
	}
	return buf, nil
}

// AppendFrame concatenates a sealed header and payload into one buffer.
func AppendFrame(header, payload []byte) []byte {
	frame := make([]byte, len(header)+len(payload))
	copy(frame, header)
	copy(frame[len(header):], payload)
	return frame
}

// SplitFrame validates that frame is exactly headerSize + payloadLen bytes
// and returns the header and payload slices. trailing/truncated are the
// protocol-specific errors to return.
func SplitFrame(frame []byte, headerSize int, payloadLen uint32, truncated, trailing error) (header, payload []byte, err error) {
	if truncated == nil {
		truncated = ErrTruncated
	}
	if trailing == nil {
		trailing = ErrTrailing
	}
	if len(frame) < headerSize {
		return nil, nil, truncated
	}
	total := headerSize + int(payloadLen)
	if len(frame) < total {
		return nil, nil, truncated
	}
	if len(frame) > total {
		return nil, nil, trailing
	}
	return frame[:headerSize], frame[headerSize:], nil
}
