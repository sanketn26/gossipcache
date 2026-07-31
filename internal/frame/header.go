package frame

import (
	"encoding/binary"
	"hash/crc32"

	"github.com/sanketn26/gossipcache/internal/wire"
)

var crc32cTable = crc32.MakeTable(crc32.Castagnoli)

// CRCSize is the fixed CRC32C trailer stored at the end of every frame header.
const CRCSize = 4

// Checksum returns the CRC32C (Castagnoli) of data.
func Checksum(data []byte) uint32 {
	return crc32.Checksum(data, crc32cTable)
}

// SealHeader writes a CRC32C of header[:len-4] into the last four bytes.
// Both control (16-byte) and rpc (20-byte) headers use this layout: the CRC
// protects every header field except itself.
func SealHeader(header []byte) {
	if len(header) < CRCSize {
		return
	}
	protected := len(header) - CRCSize
	binary.BigEndian.PutUint32(header[protected:], Checksum(header[:protected]))
}

// VerifyHeaderCRC reports whether the trailing CRC32C matches the prefix.
func VerifyHeaderCRC(header []byte) bool {
	if len(header) < CRCSize {
		return false
	}
	protected := len(header) - CRCSize
	return Checksum(header[:protected]) == binary.BigEndian.Uint32(header[protected:])
}

// PutMagic writes a big-endian magic at the start of header.
func PutMagic(header []byte, magic uint32) {
	binary.BigEndian.PutUint32(header[0:4], magic)
}

// Magic returns the big-endian magic at the start of header.
func Magic(header []byte) uint32 {
	return binary.BigEndian.Uint32(header[0:4])
}

// PutVersion writes the protocol version at bytes [4:6].
func PutVersion(header []byte, version wire.ProtocolVersion) {
	binary.BigEndian.PutUint16(header[4:6], uint16(version))
}

// Version returns the protocol version at bytes [4:6].
func Version(header []byte) wire.ProtocolVersion {
	return wire.ProtocolVersion(binary.BigEndian.Uint16(header[4:6]))
}

// PutType writes the message type at bytes [6:8].
func PutType(header []byte, messageType uint16) {
	binary.BigEndian.PutUint16(header[6:8], messageType)
}

// Type returns the message type at bytes [6:8].
func Type(header []byte) uint16 {
	return binary.BigEndian.Uint16(header[6:8])
}

// IsNilMessage reports whether message is a nil interface or a nil pointer.
func IsNilMessage(message any) bool {
	if message == nil {
		return true
	}
	// Avoid importing reflect at every call site; mirror the previous check.
	return isNilPointer(message)
}
