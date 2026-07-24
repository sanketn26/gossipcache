package wire

import (
	"encoding/binary"
	"errors"
	"math/bits"
)

// Why a hand-rolled xxHash64 instead of a library:
//
// Partition routing is a frozen cross-process, cross-architecture contract:
// every node and hub must map a key to the same partition. Two constraints
// together rule out the obvious dependencies, so we vendor the ~70 lines below
// rather than trust an external one:
//
//   - Seed. The contract freezes seed 0x9E3779B185EBCA87 (see partitionSeed).
//     github.com/cespare/xxhash/v2 — the most-vetted Go xxHash — only hashes
//     with seed 0 and exposes no seeded API, so it cannot reproduce the frozen
//     vectors without reopening the spec.
//   - Endianness. github.com/OneOfOne/xxhash supports a seed, but on big-endian
//     ppc64 its build tags select an unsafe backend that reads words in native
//     byte order, producing different hashes than little-endian nodes and
//     silently violating the routing contract on that arch.
//
// This implementation reads every word via binary.LittleEndian, so it is
// byte-order independent by construction and matches the frozen golden vectors
// on every GOARCH. Keep it that way: prefer correctness-by-construction here
// over an external dependency whose behavior is seed- or arch-conditional.

// partitionSeed is the frozen xxHash64 seed for v1 partition routing. It is a
// locked contract constant (COMMON_PHASE_00_CONTRACTS.md, "Partition routing")
// and must match on every node and hub. Do not change it without amending the
// contract and regenerating testdata/partition_vectors.json.
const partitionSeed uint64 = 0x9E3779B185EBCA87

var ErrInvalidPartitionCount = errors.New("partition count must be greater than zero")

const (
	xxPrime1 uint64 = 11400714785074694791
	xxPrime2 uint64 = 14029467366897019727
	xxPrime3 uint64 = 1609587929392839161
	xxPrime4 uint64 = 9650029242287828579
	xxPrime5 uint64 = 2870177450012600261
)

// PartitionOf routes raw key bytes to a partition using seeded xxHash64 and
// unsigned modulo. It panics when partitionCount is zero because zero cannot
// describe a valid hub generation.
func PartitionOf(key []byte, partitionCount uint32) uint32 {
	if err := validatePartitionCount(partitionCount); err != nil {
		panic(err)
	}
	return uint32(xxHash64(key, partitionSeed) % uint64(partitionCount))
}

func validatePartitionCount(partitionCount uint32) error {
	if partitionCount == 0 {
		return ErrInvalidPartitionCount
	}
	return nil
}

// xxHash64 is a spec-compliant seeded xxHash64. It reads multi-byte words via
// binary.LittleEndian so results are identical on every GOARCH; see the
// file-level comment for why this is a routing-contract requirement.
func xxHash64(data []byte, seed uint64) uint64 {
	var hash uint64
	remaining := data

	if len(remaining) >= 32 {
		v1 := seed + xxPrime1 + xxPrime2
		v2 := seed + xxPrime2
		v3 := seed
		v4 := seed - xxPrime1

		for len(remaining) >= 32 {
			v1 = xxRound(v1, binary.LittleEndian.Uint64(remaining[0:8]))
			v2 = xxRound(v2, binary.LittleEndian.Uint64(remaining[8:16]))
			v3 = xxRound(v3, binary.LittleEndian.Uint64(remaining[16:24]))
			v4 = xxRound(v4, binary.LittleEndian.Uint64(remaining[24:32]))
			remaining = remaining[32:]
		}

		hash = bits.RotateLeft64(v1, 1) +
			bits.RotateLeft64(v2, 7) +
			bits.RotateLeft64(v3, 12) +
			bits.RotateLeft64(v4, 18)
		hash = xxMergeRound(hash, v1)
		hash = xxMergeRound(hash, v2)
		hash = xxMergeRound(hash, v3)
		hash = xxMergeRound(hash, v4)
	} else {
		hash = seed + xxPrime5
	}

	hash += uint64(len(data))

	for len(remaining) >= 8 {
		k := xxRound(0, binary.LittleEndian.Uint64(remaining[:8]))
		hash ^= k
		hash = bits.RotateLeft64(hash, 27)*xxPrime1 + xxPrime4
		remaining = remaining[8:]
	}

	if len(remaining) >= 4 {
		hash ^= uint64(binary.LittleEndian.Uint32(remaining[:4])) * xxPrime1
		hash = bits.RotateLeft64(hash, 23)*xxPrime2 + xxPrime3
		remaining = remaining[4:]
	}

	for _, b := range remaining {
		hash ^= uint64(b) * xxPrime5
		hash = bits.RotateLeft64(hash, 11) * xxPrime1
	}

	hash ^= hash >> 33
	hash *= xxPrime2
	hash ^= hash >> 29
	hash *= xxPrime3
	hash ^= hash >> 32
	return hash
}

func xxRound(accumulator, input uint64) uint64 {
	accumulator += input * xxPrime2
	accumulator = bits.RotateLeft64(accumulator, 31)
	return accumulator * xxPrime1
}

func xxMergeRound(accumulator, value uint64) uint64 {
	accumulator ^= xxRound(0, value)
	return accumulator*xxPrime1 + xxPrime4
}
