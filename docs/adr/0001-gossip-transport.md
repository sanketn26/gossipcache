# ADR-0001: Gossip Transport — hashicorp/memberlist vs Custom

**Status**: Proposed
**Date**: 2026-07-05

## Context

Phase 2 as originally planned includes a hand-rolled network layer: TCP/UDP
transports, a wire protocol with magic numbers, connection pooling,
backpressure, peer management, and failure detection. That is several weeks of
work re-deriving infrastructure that `hashicorp/memberlist` (the library under
Consul and Serf) already provides: SWIM-based membership, failure detection,
gossip broadcast with piggybacking, encryption, and both TCP and UDP
transports.

GossipCache's actual novel logic is small and sits *above* the transport:

- metadata invalidation messages (key, version, checksum)
- version-compare + pull-from-backing-store on mismatch
- singleflight around pulls
- anti-entropy reconciliation

All of these fit memberlist's `Delegate`/`Broadcast` extension points.

## Decision

Use `hashicorp/memberlist` for membership, failure detection, and gossip
transport in v1. Implement GossipCache's invalidation protocol as a memberlist
delegate. Do not build a custom wire protocol, connection pool, or peer
manager.

## Consequences

**Positive:**

- Cuts weeks from Phase 2; effort goes to the differentiated invalidation
  logic instead of transport plumbing.
- Battle-tested failure detection and encryption for free (reduces the Phase
  4.5 security surface: memberlist has a built-in shared-key encryption
  layer).
- Static peer join is trivial (`Join([]string)`), and later discovery
  mechanisms only need to produce a seed list.

**Negative / accepted trade-offs:**

- Broadcast payloads ride UDP packets by default (~1400 byte budget); fine for
  metadata gossip, which is the v1 design anyway. Independent mode's full-data
  gossip (v2+) may need memberlist's reliable TCP sends or a side channel —
  revisit then.
- Less control over wire format and tuning than a custom protocol.
- New dependency with its own release cadence.

**Revisit if:** independent mode's full-data gossip or measured performance
shows memberlist's model is a poor fit. The invalidation protocol should stay
transport-agnostic (own message types, own handler interface) so the transport
can be swapped without touching cache logic.

## Effect on existing plans

`docs/impl/PHASE_2_BACKED_MODE.md` Steps 4 (Network Layer), 4.5 (Connection
Pooling & Backpressure), and the peer-management half of Step 5 are superseded
by this ADR if accepted. The gossip *message* design (Step 3) and the cache
integration (Step 6) remain valid.
