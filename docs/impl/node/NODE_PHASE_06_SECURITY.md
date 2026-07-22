# Node P6 — Security

**Depends on:** frozen Node P3 protocols; integrates with Node P4 readiness.

**Common contract:** [COMMON_PHASE_06_SECURITY.md](../common/COMMON_PHASE_06_SECURITY.md).

## Functional work

- [ ] Authenticate the hub using workload identity/mTLS.
- [ ] Validate protocol, cluster identity and `hub_generation`.
- [ ] Reload credentials and reconnect streams without leaking goroutines.
- [ ] Fail closed on incompatible or unauthenticated mutation/RPC paths.
- [ ] Protect node management/debug endpoints.

## Implementation detail

### Hub authentication

- The node dials with `tls.Config{MinVersion: VersionTLS13, RootCAs: bundle,
  Certificates: [leaf]}` and verifies the hub's SAN role/`cluster_id` — it
  accepts authority only from the configured compatible hub, never any TLS peer.
- After handshake it runs the common P6 order (SAN/cluster → protocol range →
  `hub_generation`); a mismatch fails closed and gates readiness.

### Rotation and reconnect

- Cert/CA reload swaps an `atomic.Pointer[tls.Certificate]`; reconnecting streams
  reuse the new material without leaking the old stream goroutines (joined via
  the P4 lifecycle).
- On rotation the node keeps valid local records but holds readiness at
  `GenerationRevalidating`/reconnect until streams are re-established, so it never
  serves while blind to invalidations.

### Fail-closed surfaces

- Mutation/RPC on an incompatible or unauthenticated path returns terminal
  errors and never downgrades transport (no plaintext fallback).
- Node management/debug endpoints bind pod-local and require the admin role if
  exposed.

## Verification

- [ ] Rogue/wrong-cluster hub and certificate-expiry tests.
- [ ] Rotation preserves valid local state but gates readiness until reconnect.

**Exit:** the node accepts authority only from the configured compatible hub and
never silently downgrades transport security.
