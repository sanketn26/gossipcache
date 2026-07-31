package frame_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"testing"

	"github.com/sanketn26/gossipcache/internal/frame"
	"github.com/sanketn26/gossipcache/internal/wire"
)

func TestEncoderDecoderRoundTrip(t *testing.T) {
	t.Parallel()

	var e frame.Encoder
	e.Uint8(7)
	e.Uint16(0x0102)
	e.Uint32(0x03040506)
	e.Uint64(0x0708090a0b0c0d0e)
	e.Bool(true)
	e.LengthBytes([]byte("key"))
	e.VersionTag(wire.VersionTag{PartitionID: 3, Sequence: 9})
	e.Raw([]byte{0xaa, 0xbb})

	d := frame.NewDecoder(e.Bytes())
	if v, err := d.Uint8(); err != nil || v != 7 {
		t.Fatalf("uint8 = %d, %v", v, err)
	}
	if v, err := d.Uint16(); err != nil || v != 0x0102 {
		t.Fatalf("uint16 = %d, %v", v, err)
	}
	if v, err := d.Uint32(); err != nil || v != 0x03040506 {
		t.Fatalf("uint32 = %d, %v", v, err)
	}
	if v, err := d.Uint64(); err != nil || v != 0x0708090a0b0c0d0e {
		t.Fatalf("uint64 = %d, %v", v, err)
	}
	if v, err := d.Bool(); err != nil || !v {
		t.Fatalf("bool = %v, %v", v, err)
	}
	if v, err := d.LengthBytes(wire.MaxKeyLen, wire.ErrKeyTooLarge); err != nil || string(v) != "key" {
		t.Fatalf("length bytes = %q, %v", v, err)
	}
	if v, err := d.VersionTag(); err != nil || v != (wire.VersionTag{PartitionID: 3, Sequence: 9}) {
		t.Fatalf("version = %v, %v", v, err)
	}
	if v, err := d.Take(2); err != nil || !bytes.Equal(v, []byte{0xaa, 0xbb}) {
		t.Fatalf("raw = %x, %v", v, err)
	}
	if d.Remaining() != 0 {
		t.Fatalf("remaining = %d", d.Remaining())
	}
}

func TestDecoderTruncationAndLimits(t *testing.T) {
	t.Parallel()

	planeTruncated := errors.New("plane truncated")
	d := frame.NewDecoder([]byte{0x00, 0x00}).WithTruncated(planeTruncated)
	if _, err := d.Uint32(); !errors.Is(err, planeTruncated) {
		t.Fatalf("truncation error = %v", err)
	}

	var e frame.Encoder
	e.LengthBytes(make([]byte, 8))
	d = frame.NewDecoder(e.Bytes())
	if _, err := d.LengthBytes(4, wire.ErrKeyTooLarge); !errors.Is(err, wire.ErrKeyTooLarge) {
		t.Fatalf("oversize error = %v", err)
	}

	e.Reset()
	e.Uint32(100)
	d = frame.NewDecoder(e.Bytes())
	if _, err := d.Count(10, 4, errors.New("too many")); err == nil || err.Error() != "too many" {
		t.Fatalf("count limit error = %v", err)
	}
}

func TestHeaderCRCSealAndVerify(t *testing.T) {
	t.Parallel()

	// Control-shaped 16-byte header.
	header := make([]byte, 16)
	frame.PutMagic(header, 0x47435331)
	frame.PutVersion(header, wire.CurrentProtocolVersion)
	frame.PutType(header, 3)
	binary.BigEndian.PutUint32(header[8:12], 20)
	frame.SealHeader(header)
	if !frame.VerifyHeaderCRC(header) {
		t.Fatal("sealed control header failed CRC verify")
	}
	header[0] ^= 0xff
	if frame.VerifyHeaderCRC(header) {
		t.Fatal("corrupted header verified")
	}

	// RPC-shaped 20-byte header.
	rpcHeader := make([]byte, 20)
	frame.PutMagic(rpcHeader, 0x47435231)
	frame.PutVersion(rpcHeader, 1)
	frame.PutType(rpcHeader, 7)
	binary.BigEndian.PutUint32(rpcHeader[8:12], 99)
	binary.BigEndian.PutUint32(rpcHeader[12:16], 14)
	frame.SealHeader(rpcHeader)
	if !frame.VerifyHeaderCRC(rpcHeader) {
		t.Fatal("sealed rpc header failed CRC verify")
	}
}

func TestWriteAllAndReadExact(t *testing.T) {
	t.Parallel()

	var w shortWriter
	payload := []byte("hello-frame")
	if err := frame.WriteAll(&w, payload); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(w.Bytes(), payload) {
		t.Fatalf("wrote %q", w.Bytes())
	}

	got, err := frame.ReadExact(bytes.NewReader(payload), len(payload), frame.ErrTruncated)
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("read exact = %q, %v", got, err)
	}
	if _, err := frame.ReadExact(bytes.NewReader(payload[:3]), len(payload), frame.ErrTruncated); !errors.Is(err, frame.ErrTruncated) {
		t.Fatalf("short read error = %v", err)
	}
}

func TestSplitAndAppendFrame(t *testing.T) {
	t.Parallel()

	header := []byte{1, 2, 3, 4}
	payload := []byte{5, 6}
	frameBytes := frame.AppendFrame(header, payload)
	h, p, err := frame.SplitFrame(frameBytes, 4, 2, frame.ErrTruncated, frame.ErrTrailing)
	if err != nil || !bytes.Equal(h, header) || !bytes.Equal(p, payload) {
		t.Fatalf("split = %x %x %v", h, p, err)
	}
	if _, _, err := frame.SplitFrame(frameBytes, 4, 1, frame.ErrTruncated, frame.ErrTrailing); !errors.Is(err, frame.ErrTrailing) {
		t.Fatalf("trailing error = %v", err)
	}
	if _, _, err := frame.SplitFrame(frameBytes[:5], 4, 2, frame.ErrTruncated, frame.ErrTrailing); !errors.Is(err, frame.ErrTruncated) {
		t.Fatalf("truncated error = %v", err)
	}
}

func TestIsNilMessage(t *testing.T) {
	t.Parallel()

	if !frame.IsNilMessage(nil) {
		t.Fatal("nil interface should be nil message")
	}
	var ptr *int
	if !frame.IsNilMessage(ptr) {
		t.Fatal("nil pointer should be nil message")
	}
	if frame.IsNilMessage(1) {
		t.Fatal("value should not be nil message")
	}
}

type shortWriter struct{ bytes.Buffer }

func (w *shortWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return w.Buffer.Write(p[:1])
}

func TestShortWriter(t *testing.T) {
	t.Parallel()
	var w shortWriter
	if _, err := w.Write([]byte("ab")); err != nil {
		t.Fatal(err)
	}
	if w.String() != "a" {
		t.Fatalf("got %q", w.String())
	}
	_ = io.EOF
}
