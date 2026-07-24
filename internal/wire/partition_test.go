package wire

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"testing"
)

func TestXXHash64ReferenceValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		key  []byte
		want uint64
	}{
		{name: "empty", key: []byte{}, want: 0x6ec6d05f61c7e7a7},
		{name: "short", key: []byte("a"), want: 0x727c10e0d238e188},
		{name: "medium", key: []byte("gossipcache"), want: 0x0ce55a1886928622},
		{
			name: "stripe",
			key: []byte{
				0, 1, 2, 3, 4, 5, 6, 7,
				8, 9, 10, 11, 12, 13, 14, 15,
				16, 17, 18, 19, 20, 21, 22, 23,
				24, 25, 26, 27, 28, 29, 30, 31,
			},
			want: 0xbfb3e4ef6096c49c,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := xxHash64(test.key, partitionSeed); got != test.want {
				t.Fatalf("xxHash64() = %#016x, want %#016x", got, test.want)
			}
		})
	}
}

func TestPartitionGoldenVectors(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("testdata/partition_vectors.json")
	if err != nil {
		t.Fatalf("read vectors: %v", err)
	}

	var vectors []struct {
		KeyHex         string `json:"key_hex"`
		PartitionCount uint32 `json:"partition_count"`
		PartitionID    uint32 `json:"partition_id"`
	}
	if err := json.Unmarshal(data, &vectors); err != nil {
		t.Fatalf("decode vectors: %v", err)
	}
	if len(vectors) == 0 {
		t.Fatal("partition vectors are empty")
	}

	for i, vector := range vectors {
		key, err := hex.DecodeString(vector.KeyHex)
		if err != nil {
			t.Fatalf("vector %d key: %v", i, err)
		}
		if got := PartitionOf(key, vector.PartitionCount); got != vector.PartitionID {
			t.Errorf("vector %d: PartitionOf(%x, %d) = %d, want %d",
				i, key, vector.PartitionCount, got, vector.PartitionID)
		}
	}
}

func TestPartitionOfRejectsZeroPartitions(t *testing.T) {
	t.Parallel()

	defer func() {
		if got := recover(); !errors.Is(asError(got), ErrInvalidPartitionCount) {
			t.Fatalf("panic = %v, want %v", got, ErrInvalidPartitionCount)
		}
	}()
	PartitionOf([]byte("key"), 0)
}

func asError(value any) error {
	err, _ := value.(error)
	return err
}
