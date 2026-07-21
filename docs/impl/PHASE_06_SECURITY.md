# Phase 6: Security Hardening (hybrid)

**Goal**: Production security for the hybrid L1 + L2 hub surface—mTLS on data and control planes, management-port protection, rate limits, and rotation runbooks.

**Prerequisites**: **P3–P4** so L2 RPC (`:7400`), invalidation streams (`:7401`), and mgmt probes (`:8081`) exist. Often parallel with or after **P5**. See [PHASE_PLAN.md](PHASE_PLAN.md), [SEMANTICS.md](../SEMANTICS.md).

**Status**: Optional for trusted private networks; **required** before exposing RPC, stream, or management ports across untrusted networks.

**Sequence:** P6 in [PHASE_PLAN.md](PHASE_PLAN.md) (after P3–P4; before untrusted deploy).

**Normative behavior:** [SEMANTICS.md](../SEMANTICS.md) §9 (mTLS TCP control plane).  
**Wire detail:** [HYBRID_BACKED_MODE.md](HYBRID_BACKED_MODE.md) §5.2 (TLS 1.3 + workload identity/mTLS; connect validates cluster ID and `hub_generation`).

> This phase is **not** memberlist / UDP gossip security. Redis-as-SoT and custom gossip join-HMAC designs are obsolete. Prefer SEMANTICS if anything conflicts.

---

## Overview

v1 product shape:

```text
App + L1 ── mTLS RPC :7400 ──► L2 hub
     ▲                            │
     └──── mTLS streams :7401 ────┘
           mgmt :8081 (restricted)
```

Security work hardens **those** surfaces. Peer L1 fanout (if present) uses the same TLS policy as hub streams; it is not a separate “gossip encryption” stack.

Dev may run plain TCP ([PHASE_PLAN](PHASE_PLAN.md) P3). Production and this phase assume TLS 1.3 + mTLS (or an equivalent mesh-injected identity).

---

## Objectives

- [ ] TLS 1.3 on L2 data RPC and invalidation streams
- [ ] mTLS (or mesh workload identity) for L1↔hub authentication
- [ ] Connection handshake validates **cluster ID** and **`hub_generation`** (reject mismatch → not ready / refuse serve)
- [ ] Management port (`/livez`, `/startupz`, `/readyz`, `/debug/*`, `/metrics` if co-located) bound to localhost or network policy; debug/admin routes authenticated
- [ ] Rate limiting: L2 RPC per client, stream subscribe/reconnect storms, management/debug HTTP
- [ ] Certificate (and optional shared-secret) rotation runbooks without silent readiness lies
- [ ] No values, raw keys, credentials, or TLS material in logs/traces ([PHASE_05_OBSERVABILITY](PHASE_05_OBSERVABILITY.md))

---

## Implementation Steps

### TDD rhythm

Security starts with **negative** tests: unauthenticated, wrong CA, wrong SAN, expired cert, wrong `hub_generation`, and rate-limit abuse must fail closed before positive paths are green.

### Step 1: TLS config shared by L2 RPC and control plane

Packages (align with [PHASE_PLAN](PHASE_PLAN.md) tree): `internal/config` TLS fields; builders used by `internal/l2/client`, `internal/l2/server`, `internal/control`.

```go
// internal/config or internal/tls — illustrative
type TLSConfig struct {
    Enabled            bool
    CertFile           string
    KeyFile            string
    CAFile             string
    MutualTLS          bool   // production default true for hub-facing ports
    InsecureSkipVerify bool   // dev only; reject in production validation
    MinVersion         uint16 // tls.VersionTLS13
}
```

Rules:

- One policy for **RPC and streams** (same identity story).
- Hub and L1 both present certificates under mTLS.
- `InsecureSkipVerify` and plain TCP are config-gated; production validation fails if enabled.

### Step 2: Identity and generation at connect

On L2 RPC and stream dial/accept:

1. Complete TLS handshake (peer cert required when mTLS on).
2. Exchange application hello: `cluster_id`, `hub_generation`, protocol version.
3. Mismatch → close connection; L1 readiness surfaces `HUB_GENERATION_MISMATCH` / `PROTOCOL_INCOMPATIBLE` as appropriate ([SEMANTICS](../SEMANTICS.md) §10).

No shared-secret “gossip join” channel. Membership is **hub-mediated** (subscribe to partition streams), not SWIM join auth.

### Step 3: Management and debug authentication

- Prefer bind `MgmtListen` to `127.0.0.1` or a private Service with NetworkPolicy.
- `/livez` may stay unauthenticated for kubelet if the port is not public.
- `/readyz` and `/startupz`: unauthenticated only if network-restricted; otherwise require auth.
- `/debug/cache`, `/debug/peers`, drain/`preStop` helpers: **authenticated** (API key, mTLS client cert, or platform auth).
- Do not expose pprof on public interfaces.

```go
// illustrative middleware — constant-time compare for bearer tokens
func APIKeyAuth(next http.Handler, expected string) http.Handler { /* ... */ }
```

### Step 4: Rate limiting

Bounded token buckets (or equivalent) for:

| Surface | Limit dimension |
|---------|-----------------|
| L2 Get/Set/Delete | per authenticated client / identity |
| Stream subscribe + reconnect | per client + per partition stream |
| Management HTTP | per source (low ceiling on debug) |

Emit Prometheus counters with **bounded** labels only (no raw IPs as high-cardinality series—hash or coarse buckets if needed). Telemetry must not block data path ([HYBRID §9](HYBRID_BACKED_MODE.md#9-observability-contract)).

### Step 5: Rotation runbooks

Document in [DEPLOYMENT.md](../DEPLOYMENT.md) (or `docs/runbooks/`):

- Rolling cert reload on hub and L1 without dual-publish or generation lies
- Accept current+previous CA/cert during a bounded overlap window
- `hub_generation` bump only when sequence spaces cannot continue ([SEMANTICS](../SEMANTICS.md) §3)—not as a routine cert rotation step
- K8s: cert-manager / projected SA tokens / mesh identity examples
- How to verify mTLS from a debug pod; how to confirm rogue clients are rejected

---

## Tests

| Area | Cases |
|------|--------|
| `internal/.../tls_test.go` | Trusted peers complete RPC + stream handshake; wrong CA, expired cert, missing client cert fail |
| Handshake app-layer | Wrong `cluster_id` / `hub_generation` rejected; L1 not ready |
| Mgmt auth | Missing/malformed/unauthorized credentials rejected on debug routes |
| Rate limit | Sustained abuse rejected; recovery after window |
| `test/integration/security_test.go` | Hub + two L1s with mTLS: valid cluster works; rogue L1 cannot subscribe or write |

---

## Deliverables

- [ ] Shared TLS builder + config validation (prod rejects insecure flags)
- [ ] mTLS (or mesh identity) on `:7400` and `:7401`
- [ ] Connect-time `cluster_id` + `hub_generation` checks
- [ ] Restricted mgmt listen + authenticated debug/admin routes
- [ ] Rate limiters on RPC, stream, and mgmt ingress
- [ ] Rotation and threat-model notes in deployment docs

---

## Out of scope

- Custom RUDP security
- Memberlist shared-key gossip encryption
- Redis/Valkey ACLs as the product trust boundary
- Independent-mode peer mesh auth (not v1)
- Full zero-trust mesh productization beyond “works with standard mTLS/workload identity”

---

## Success criteria

1. Untrusted clients cannot complete L2 RPC or stream subscribe when mTLS is required.
2. Generation/cluster mismatch cannot leave a node “ready” while serving as if consistent.
3. Debug endpoints are not anonymously reachable on a default production config.
4. Cert rotation has a tested path that does not require silent control-plane loss without readiness reflecting it.
5. SEMANTICS control-plane and readiness rules remain intact.
