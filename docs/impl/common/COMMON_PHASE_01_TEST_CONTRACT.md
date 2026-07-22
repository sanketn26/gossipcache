# Common P1 — State-machine test contract

**Depends on:** [COMMON_PHASE_00_CONTRACTS.md](COMMON_PHASE_00_CONTRACTS.md).

## Decisions

- [ ] Freeze `OK`, `NOT_FOUND`, `NOT_CAUGHT_UP` and terminal error behavior.
- [ ] Define versioned tombstone and expiry response semantics.
- [ ] Define deterministic fake hooks for blocked fetch, delayed mutation and
  injected failure.

## Implementation detail

### Hub seam consumed by the state machine

The Node state machine depends only on this interface (defined in `internal/l1`,
implemented by both fake and real hub):

```go
type HubClient interface {
    Get(ctx context.Context, key []byte, min *wire.VersionTag) (GetResult, error)
    Set(ctx context.Context, key, value []byte, ttl time.Duration, opt WriteOptions) (wire.VersionTag, error)
    Delete(ctx context.Context, key []byte, opt WriteOptions) (wire.VersionTag, error)
}

type GetResult struct {
    Value         []byte
    HubGeneration uint64
    Version       wire.VersionTag
    TTL           time.Duration
    Kind          RecordKind // Value | Tombstone
    Status        wire.Status
}
```

### Response semantics to freeze

- `NOT_FOUND` returns no version; the slot stays `EMPTY` (never a negative-cache
  slot in v1).
- A tombstone returns a real `VersionTag` with `Kind == Tombstone`; it installs
  like a value and satisfies read-your-writes for a delete.
- Expiry is a hub-side versioned mutation, not a status: the hub returns
  `NOT_FOUND` after the expiring mutation commits, never a silently stale value.
- `NOT_CAUGHT_UP` is returned when `min_version` exceeds the hub's committed
  head for that key's partition; the caller retries, it is not terminal.

### Deterministic fake hooks

The fake exposes barrier-style hooks so races are orchestrated without sleeps:

```go
type FakeHooks struct {
    BeforeGetReturn func(key []byte) (release func())   // block a fetch mid-flight
    BeforeCommit    func(m Mutation) (release func())   // delay a write commit
    FailNext        func(op Op) error                   // inject one terminal/retryable error
}
```

`release` closures are signalled by the test; the fake blocks on them inside the
call, giving exact interleavings for invalidate-during-fetch and write/fetch
races.

### Exhaustive state/event matrix

The Node P1 table suite instantiates every applicable row below under all three
stale policies. “Drop” means remove the retained record and return to `EMPTY`;
“stale” means retain an immutable record only when policy permits it.

| Starting state | Event | Required result |
|----------------|-------|-----------------|
| `EMPTY` | Get | enter `FETCHING`; one Hub Get for all concurrent callers |
| `EMPTY` | unknown-key invalidation | remain `EMPTY`; advance stream watermark only; allocate no slot |
| `FETCHING` | equal/newer invalidation | raise ceiling; keep waiters; accept response only when version ≥ ceiling |
| `FETCHING` | older invalidation | ignore version; keep fetch and waiters unchanged |
| `FETCHING` | successful value/tombstone response | install immutable `VALID` record when generation matches and version ≥ ceiling |
| `FETCHING` | `NOT_FOUND` | return to `EMPTY`; do not install an unversioned negative entry |
| `FETCHING` | retryable/terminal error | wake all waiters; drop or serve retained stale value exactly per policy |
| `FETCHING` | generation mismatch | reject response, gate readiness, discard/revalidate old-generation state |
| `VALID` | Get before TTL | return copied value locally; no Hub call |
| `VALID` | equal/older invalidation | idempotent; retain current record |
| `VALID` | newer invalidation | raise ceiling; drop for `StaleNever`, otherwise retain as `STALE` |
| `VALID` | TTL reached or memory eviction | drop locally; next Get demand-fetches; never mint a version |
| `VALID`/`STALE` | local Set success | install returned value/version before returning, then treat self-invalidation as idempotent |
| `VALID`/`STALE` | local Delete success | install returned tombstone/version before returning; reads report miss |
| `STALE` | Get with `StaleNever` | impossible retained state; behave as `EMPTY` |
| `STALE` | Get with `StaleIfError` | synchronously revalidate; serve stale only if fetch fails |
| `STALE` | Get with `ServeStaleWhileRevalidate` | return stale copy and start/join one background refresh |
| any retained state | Hub generation changes | stop serving it as valid, gate readiness and reconcile before ready |

Tombstone TTL/eviction follows the same version and generation rules as values.
Tests also cover invalidation during fetch, write during fetch, expiry during
fetch, equal-version self-invalidation and Stop while refresh is blocked.

## Cross-component verification

- [ ] Fetch/invalidate, fetch/write and delete/fetch races use the Hub interface.
- [ ] No Hub behavior is mocked inside the Node state-machine package.
- [ ] Every matrix row runs deterministically for each applicable stale policy,
  including generation mismatch and background-refresh shutdown.

**Exit:** Node P1 tests are deterministic and the fake behavior matches the
future real Hub contract.
