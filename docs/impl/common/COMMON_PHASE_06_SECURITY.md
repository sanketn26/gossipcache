# Common P6 — Security contract

**Depends on:** frozen P3 data/control wire contracts; integrates with P4
readiness before production exit.

- [ ] Freeze TLS version, CA/SAN/workload identity and certificate rotation
  policy for data and control protocols.
- [ ] Validate protocol range, cluster identity and `hub_generation` at connect.
- [ ] Define authorization for RPC, subscription and management operations.
- [ ] Define rate-limit behavior and observable rejection codes.
- [ ] Add shared negative vectors for wrong CA/SAN/cluster/generation and expiry.

## Implementation detail

### Transport identity

- TLS 1.3 only (`tls.Config{MinVersion: VersionTLS13}`), mutual auth on both the
  RPC (`7400`) and control (`7401`) ports. Management port stays plaintext but
  pod-local only.
- Peer authorization is by **workload identity encoded in the certificate SAN**
  (SPIFFE-style URI SAN, e.g. `spiffe://<trust-domain>/gossipcache/<role>`), not
  by IP. A shared `cluster_id` is carried both in the SAN path and validated in
  the handshake so certs from another cluster fail closed.
- CA bundle, leaf cert and key are reloadable from disk via an
  `atomic.Pointer[tls.Certificate]` `GetCertificate`/`GetConfigForClient` hook;
  rotation swaps the pointer without dropping healthy connections and without
  touching sequence or generation state.

### Connect-time validation order

```text
1. TLS handshake (mTLS, cert chain to configured CA)
2. SAN role + cluster_id authorized for the requested operation class
3. ProtocolVersion within supported range
4. hub_generation adoption or equality
   -> first connection with no expected generation: adopt the Hub's advertised
      generation, then complete initial validation before becoming ready
   -> reconnect with an expected generation equal to the Hub's current value:
      continue normally
   -> reconnect mismatch: ERR_BAD_GENERATION, discard/revalidate old-generation
      state, then reconnect with no expected generation to adopt the new value
```

An absent expected generation is a distinct bootstrap state, not generation
zero and not a wildcard on an established connection. The Node may use it only
when it has no locally trusted generation or after it has gated readiness and
discarded/revalidated all state associated with the previous generation.

### Authorization classes

Closed set mapped to cert roles: `read` (Get), `write` (Set/Delete),
`subscribe` (control stream), `admin` (management). A node role gets
read/write/subscribe; admin is separate. Unauthorized op => terminal reject,
logged as a security event with SAN and cluster (never key/value).

### Rate limiting

Per-identity token buckets on connect/reconnect, subscription and mutation
paths (`RateLimitRPS`, `RateLimitBurst` configurable). Excess returns
`ERR_RATE_LIMITED` (retryable) and increments a bounded rejection counter;
limits never silently drop.

### Negative vectors (shared fixtures)

`internal/wire/testdata/security/`: wrong-CA cert, wrong-SAN role, wrong
`cluster_id`, expired leaf, first-connect generation adoption, and stale
`hub_generation` reconnects — consumed by both Hub and Node to prove bootstrap
and fail-closed behavior.

**Exit:** valid identities interoperate and rogue or incompatible identities
fail closed without changing data/version semantics.
