// Package rpc holds data-plane helpers that sit above the gRPC Data service.
// Wire encoding is protobuf (api/proto/gossipcache/v1); this package owns
// idempotency fingerprints, status classification, and wire↔proto conversion.
package rpc

// DefaultRPCPort is the conventional hub gRPC listen port for both Data and
// Control services (v1 uses a single listener; no split control port).
const DefaultRPCPort = 7400
