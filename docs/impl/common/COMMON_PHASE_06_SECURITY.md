# Common P6 — Security contract

**Depends on:** frozen P3 gRPC Data/Control services; integrates with P4
readiness before production exit.

- [ ] Freeze TLS version, CA/SAN/workload identity and certificate rotation
  policy for the single gRPC listener.
- [ ] Validate protocol range, cluster identity and `hub_generation` at connect
  **and** on every Data RPC / Control Hello (schema fields below).
- [ ] Define authorization for Data RPCs, Control subscription and management.
- [ ] Define rate-limit behavior and observable rejection codes (Data vs Control).
- [ ] Add shared negative vectors for wrong CA/SAN/cluster/generation and expiry.

## Implementation detail

### Transport identity

- TLS 1.3 only (`tls.Config{MinVersion: VersionTLS13}`), mutual auth on the
  **single** gRPC listener (`7400` for both Data and Control). Management port
  stays plaintext but pod-local only.
- Peer authorization is by **workload identity encoded in the certificate SAN**
  (SPIFFE-style URI SAN, e.g. `spiffe://<trust-domain>/gossipcache/<role>`), not
  by IP. A shared `cluster_id` is carried both in the SAN path and in application
  schema fields so certs from another cluster fail closed.
- CA bundle, leaf cert and key are reloadable from disk via an
  `atomic.Pointer[tls.Certificate]` `GetCertificate`/`GetConfigForClient` hook;
  rotation swaps the pointer without dropping healthy connections and without
  touching sequence or generation state.

### Connect-time and per-RPC credentials (schema)

Unary gRPC does **not** create an implicit session after `Handshake`. Identity
and generation are carried explicitly so the Hub can reject before work:

| Surface | Fields | Failure |
|---------|--------|---------|
| `Data.HandshakeRequest` | `cluster_id`, `expected_hub_generation` (0 = bootstrap) | gRPC `PermissionDenied` / `FailedPrecondition` or app `STATUS_ERR_BAD_GENERATION` before advertise |
| `Data.HandshakeResponse` | `cluster_id` echo, `hub_generation` | — |
| `Data.GetRequest` / `MutateRequest` | `hub_generation` (required, non-zero) | **Before** lookup/commit: `STATUS_ERR_BAD_GENERATION`; **commits nothing** |
| `Data.HubStatusRequest` | `hub_generation` | `STATUS_ERR_BAD_GENERATION` in response status |
| `Control.Hello` | `cluster_id`, `expected_hub_generation` (0 = bootstrap) | `ControlError(STATUS_ERR_BAD_GENERATION)` then stream close |
| `Control.Subscribe` | `hub_generation` | `ControlError` on mismatch |

Validation order:

```text
1. TLS handshake (mTLS, cert chain to configured CA)
2. SAN role + cluster_id authorized for the requested operation class
3. Application cluster_id equals hub config (Handshake / Hello)
4. ProtocolVersion within supported range
5. hub_generation adoption or equality
   -> expected_hub_generation == 0 (bootstrap): adopt hub's advertised generation
      after successful Handshake/Hello; Node must then send that generation on
      every Get/Mutate and Control Subscribe
   -> expected_hub_generation != 0 and equal to hub current: continue
   -> expected_hub_generation != 0 and mismatch: STATUS_ERR_BAD_GENERATION /
      ControlError; Node discards/revalidates old-generation state, then
      reconnects with expected_hub_generation == 0 to adopt the new value
```

An absent expected generation (`0`) is bootstrap only — not generation zero as a
real hub incarnation (hub generations are always non-zero) and not a wildcard on
later Get/Mutate calls.

### Authorization classes

Closed set mapped to cert roles: `read` (Get), `write` (Set/Delete),
`subscribe` (control stream), `admin` (management). A node role gets
read/write/subscribe; admin is separate. Unauthorized op => terminal reject,
logged as a security event with SAN and cluster (never key/value).

### Rate limiting

Per-identity token buckets on connect/reconnect, subscription and mutation
paths (`RateLimitRPS`, `RateLimitBurst` configurable).

| Path | Rejection signal |
|------|------------------|
| Data Get/Mutate/Handshake excess | Application `STATUS_ERR_RATE_LIMITED` in the response (retryable) |
| Control stream already open, Subscribe excess | `ControlError{status: STATUS_ERR_RATE_LIMITED}` on the stream |
| Control connect / hard accept budget exhausted | gRPC status `ResourceExhausted` closing the stream (no app body yet) |

Limits never silently drop. Bounded rejection counters are required.

### Negative vectors (shared fixtures)

`internal/wire/testdata/security/`: wrong-CA cert, wrong-SAN role, wrong
`cluster_id`, expired leaf, first-connect generation adoption, and stale
`hub_generation` reconnects — consumed by both Hub and Node to prove bootstrap
and fail-closed behavior.

**Exit:** valid identities interoperate and rogue or incompatible identities
fail closed without changing data/version semantics.
