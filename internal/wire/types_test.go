package wire_test

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/sanketn26/gossipcache/internal/wire"
)

func TestVersionTagIsNewer(t *testing.T) {
	t.Parallel()

	base := wire.VersionTag{PartitionID: 2, Sequence: 10}
	if !(wire.VersionTag{PartitionID: 2, Sequence: 11}).IsNewer(base) {
		t.Fatal("higher sequence in same partition should be newer")
	}
	if (wire.VersionTag{PartitionID: 3, Sequence: 11}).IsNewer(base) {
		t.Fatal("tags in different partitions must not compare as newer")
	}
	if base.IsNewer(base) {
		t.Fatal("equal tags must not compare as newer")
	}
}

func TestMutationIDLayout(t *testing.T) {
	t.Parallel()

	id := wire.NewMutationID(0x0102030405060708, 0x1112131415161718)
	want := wire.MutationID{
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18,
	}
	if id != want {
		t.Fatalf("NewMutationID() = %x, want %x", id, want)
	}
	if id.NodeEpoch() != 0x0102030405060708 || id.Counter() != 0x1112131415161718 {
		t.Fatalf("mutation components did not round trip: epoch=%x counter=%x", id.NodeEpoch(), id.Counter())
	}
	if id.IsZero() {
		t.Fatal("non-zero mutation ID reported zero")
	}
	if !(wire.MutationID{}).IsZero() {
		t.Fatal("zero mutation ID reported non-zero")
	}
}

func TestZeroValueDefaults(t *testing.T) {
	t.Parallel()

	var options wire.WriteOptions
	if options.Mode != wire.WriteFast {
		t.Fatalf("zero write mode = %d, want WriteFast", options.Mode)
	}
	if options.ConfirmLevel != wire.ConfirmInvalidateApplied {
		t.Fatalf("zero confirm level = %d, want ConfirmInvalidateApplied", options.ConfirmLevel)
	}
	if err := options.Validate(); err != nil {
		t.Fatalf("zero write options should be valid: %v", err)
	}

	var profile wire.StorageProfile
	if profile != wire.StorageMemory {
		t.Fatalf("zero storage profile = %d, want StorageMemory", profile)
	}
	if err := profile.Validate(false); err != nil {
		t.Fatalf("default memory profile should be valid: %v", err)
	}
}

func TestStorageProfileValidation(t *testing.T) {
	t.Parallel()

	if err := wire.StorageDurable.Validate(false); !errors.Is(err, wire.ErrPersistenceConfigRequired) {
		t.Fatalf("durable without persistence error = %v", err)
	}
	if err := wire.StorageDurable.Validate(true); err != nil {
		t.Fatalf("durable with persistence should be valid: %v", err)
	}
	if err := wire.StorageProfile(2).Validate(true); !errors.Is(err, wire.ErrInvalidStorageProfile) {
		t.Fatalf("unknown profile error = %v", err)
	}
	if wire.PersistenceConfigured(" \t") {
		t.Fatal("whitespace-only data directory should not count as configured")
	}
	if !wire.PersistenceConfigured("/var/lib/gossipcache") {
		t.Fatal("non-empty data directory should count as configured")
	}
}

func TestRequestValidationAndCloning(t *testing.T) {
	t.Parallel()

	key := []byte("key")
	value := []byte("value")
	version := wire.VersionTag{PartitionID: wire.PartitionOf(key, 16), Sequence: 7}
	get := wire.GetRequest{Key: key, MinVersion: &version}
	if err := get.Validate(16); err != nil {
		t.Fatalf("valid get: %v", err)
	}
	getClone := get.Clone()

	set := wire.SetRequest{
		Key:        key,
		Value:      value,
		TTLMillis:  1500,
		MutationID: wire.NewMutationID(1, 1),
	}
	if err := set.Validate(); err != nil {
		t.Fatalf("valid set: %v", err)
	}
	setClone := set.Clone()

	deleteRequest := wire.DeleteRequest{
		Key:        key,
		MutationID: wire.NewMutationID(1, 2),
	}
	if err := deleteRequest.Validate(); err != nil {
		t.Fatalf("valid delete: %v", err)
	}
	deleteClone := deleteRequest.Clone()

	key[0] = 'K'
	value[0] = 'V'
	version.Sequence = 99
	if string(getClone.Key) != "key" || getClone.MinVersion.Sequence != 7 {
		t.Fatalf("get clone aliases source: %+v", getClone)
	}
	if string(setClone.Key) != "key" || string(setClone.Value) != "value" {
		t.Fatalf("set clone aliases source: %+v", setClone)
	}
	if string(deleteClone.Key) != "key" {
		t.Fatalf("delete clone aliases source: %+v", deleteClone)
	}
}

func TestRequestBounds(t *testing.T) {
	t.Parallel()

	validID := wire.NewMutationID(1, 1)
	tests := []struct {
		name string
		run  func() error
		want error
	}{
		{
			name: "empty key",
			run:  func() error { return (wire.GetRequest{}).Validate(16) },
			want: wire.ErrKeyEmpty,
		},
		{
			name: "large key",
			run: func() error {
				return (wire.DeleteRequest{Key: make([]byte, wire.MaxKeyLen+1), MutationID: validID}).Validate()
			},
			want: wire.ErrKeyTooLarge,
		},
		{
			name: "large value",
			run: func() error {
				return (wire.SetRequest{
					Key: []byte("key"), Value: make([]byte, wire.MaxValueLen+1), MutationID: validID,
				}).Validate()
			},
			want: wire.ErrValueTooLarge,
		},
		{
			name: "missing mutation ID",
			run: func() error {
				return (wire.SetRequest{Key: []byte("key")}).Validate()
			},
			want: wire.ErrMutationIDRequired,
		},
		{
			name: "invalid write mode",
			run: func() error {
				return (wire.DeleteRequest{
					Key: []byte("key"), MutationID: validID,
					Options: wire.WriteOptions{Mode: 2},
				}).Validate()
			},
			want: wire.ErrInvalidWriteMode,
		},
		{
			name: "invalid confirm level",
			run: func() error {
				return (wire.DeleteRequest{
					Key: []byte("key"), MutationID: validID,
					Options: wire.WriteOptions{ConfirmLevel: 1},
				}).Validate()
			},
			want: wire.ErrInvalidConfirmLevel,
		},
		{
			name: "negative timeout",
			run: func() error {
				return (wire.DeleteRequest{
					Key: []byte("key"), MutationID: validID,
					Options: wire.WriteOptions{Timeout: -time.Millisecond},
				}).Validate()
			},
			want: wire.ErrInvalidWriteTimeout,
		},
		{
			name: "sub-millisecond timeout",
			run: func() error {
				return (wire.DeleteRequest{
					Key: []byte("key"), MutationID: validID,
					Options: wire.WriteOptions{Timeout: 500 * time.Microsecond},
				}).Validate()
			},
			want: wire.ErrTimeoutNotWholeMillis,
		},
		{
			name: "timeout exceeds wire maximum",
			run: func() error {
				return (wire.DeleteRequest{
					Key: []byte("key"), MutationID: validID,
					Options: wire.WriteOptions{Timeout: (wire.MaxWriteTimeoutMillis + 1) * time.Millisecond},
				}).Validate()
			},
			want: wire.ErrWriteTimeoutTooLarge,
		},
		{
			name: "wrong minimum partition",
			run: func() error {
				key := []byte("key")
				version := wire.VersionTag{PartitionID: (wire.PartitionOf(key, 16) + 1) % 16, Sequence: 1}
				return (wire.GetRequest{Key: key, MinVersion: &version}).Validate(16)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := test.run()
			if err == nil {
				t.Fatal("expected validation error")
			}
			if test.want != nil && !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestCopyBytes(t *testing.T) {
	t.Parallel()

	if wire.CopyBytes(nil) != nil {
		t.Fatal("CopyBytes should preserve nil")
	}
	source := []byte("value")
	copied := wire.CopyBytes(source)
	source[0] = 'V'
	if bytes.Equal(source, copied) || string(copied) != "value" {
		t.Fatalf("copy aliases source: source=%q copied=%q", source, copied)
	}
}

func TestTTLMillis(t *testing.T) {
	t.Parallel()

	if got, err := wire.TTLMillis(1500 * time.Millisecond); err != nil || got != 1500 {
		t.Fatalf("TTLMillis() = %d, %v; want 1500, nil", got, err)
	}
	if got, err := wire.TTLMillis(0); err != nil || got != wire.TTLNone {
		t.Fatalf("TTLMillis(0) = %d, %v; want TTLNone, nil", got, err)
	}
	if _, err := wire.TTLMillis(time.Microsecond); !errors.Is(err, wire.ErrTTLNotWholeMilliseconds) {
		t.Fatalf("fractional ttl error = %v", err)
	}
	if _, err := wire.TTLMillis(-time.Millisecond); err == nil {
		t.Fatal("negative ttl should fail")
	}
}
