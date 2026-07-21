# Testing Strategy (v1 hybrid)

**Behavior under test:** [../SEMANTICS.md](../SEMANTICS.md)
**Detail:** [HYBRID_BACKED_MODE.md](HYBRID_BACKED_MODE.md)

Focus on **L1 state machine**, **hub commit + stream**, **interest/held-key apply**, **W**, and **readiness**—not Redis or independent-mode gossip.

## Pyramid

| Layer | Share | Goal |
|-------|------:|------|
| Unit | most | Pure logic, no network |
| Integration | medium | L1 ↔ hub, multi-L1 |
| Fault / chaos | fewer | Gaps, stall, crash, W timeout |
| Bench | gated | Hits, writes, invalidation p99 |

Coverage target: **>80%** on `internal/l1`, control-plane, hub commit path.

## 1. Unit

### L1 state machine

Every transition in SEMANTICS §5–7, including:

- Hit / miss / TTL eviction
- Invalidation during `FETCHING` (ceiling, no cancel waiters)
- Reject L2 response below ceiling; equality accepted
- Stale policies: `Never`, `StaleIfError`, `ServeStaleWhileRevalidate`
- Writer install before OK (read-your-writes)
- Own invalidation at same version is idempotent

Use a fake `L2Client` and fake stream injector.

### Version tag

- Order only within same `partition_id`
- Cross-partition compare rejected / not ordered
- Tombstone vs missing unversioned response

### W logic

- W=0 returns without peer confirms
- W=k waits for k confirms; timeout → error, hub commit not rolled back
- Hop ack must not count as confirm

### Subscription / interest

- Empty key + invalidation → watermark only, no fetch
- Held key → STALE
- Subscribe on first interest (unit the registry, not real TCP)

## 2. Integration

### Minimum cluster

- 1 hub (dev single-node OK)
- ≥2 L1 processes

### Cases

| Case | Expect |
|------|--------|
| Write on A, Get on A | New value immediately |
| Write on A (W=0), Get on B before invalidate | May be old |
| Write on A (W=0), wait stream, Get on B | New after apply+fetch |
| Write on A (W=1) | Blocks until ≥1 peer confirm or timeout |
| Concurrent miss same key | Singleflight → one hub Get |
| Hub restart after commit | Stream replay / no lost durable write |
| L1 restart | EMPTY; demand fill; re-subscribe |

No Redis required.

## 3. Fault injection

| Fault | Expect |
|-------|--------|
| Drop stream frames | Gap → replay or reconciliation; not ready if unreconciled |
| Stop checkpoints | `STREAM_FRESHNESS_UNKNOWN` |
| Hub unavailable | Writes fail; misses per stale policy |
| Crash hub mid-barrier | No ack without durable value+stream event |
| Crash L1 after hop ack before apply | Application watermark not advanced; no silent loss |
| W>0, no peers | Timeout error; hub still has write |

## 4. Performance

Against a **named** load profile (see hybrid load profile section):

- L1 hit: alloc-free path; p99 budget
- Miss / write latency vs hub
- Invalidation apply rate
- Compare W=0 vs W=1 write p99

Do not gate on sub-µs marketing claims without published hardware numbers.

## 5. Observability tests

- Cardinality allowlist (no raw key labels)
- Readiness reason codes under fault matrix
- Telemetry export failure does not block Get/Set/apply

Align with [PHASE_05_OBSERVABILITY.md](PHASE_05_OBSERVABILITY.md) (P5).

## 6. What not to prioritize (v1)

- Vector-clock / independent-mode suites as primary
- Redis backing-store integration as SoT
- MicroVM matrix (optional later)
- Unsubscribe-when-empty edge storms

## 7. Commands

```bash
go test ./...
go test -race ./...
go test -cover ./...
go test -run TestL1_ ./internal/l1/...
go test -run TestIntegration_ ./test/integration/...
go test -bench=. ./...
```

## 8. Exit criteria (implementation milestones)

| Gate | Pass when |
|------|-----------|
| P1 / H1 | All SM transitions + race unit tests green |
| P2 / H2 | Disconnect/replay/freshness/W tests green |
| P3 / H3 | Hub crash/restart: no lost acknowledged write |
| P4 / H4 | Ready never true with unreconciled gap or stale checkpoint |
| P5 | Fault-to-signal + cardinality CI |
| P6 | Rogue client rejected under mTLS |
| P7 | Demo scenarios fail closed on non-convergence |
| P8 / H5 | Optim benches + correctness suite still green |
