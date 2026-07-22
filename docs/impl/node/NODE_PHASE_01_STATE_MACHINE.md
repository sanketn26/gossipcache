# Node P1 — Local state machine

**Depends on:** [NODE_PHASE_00_FOUNDATION.md](NODE_PHASE_00_FOUNDATION.md).

**Common contract:** [COMMON_PHASE_01_TEST_CONTRACT.md](../common/COMMON_PHASE_01_TEST_CONTRACT.md).

## Functional work

- [ ] Implement immutable records and `EMPTY/FETCHING/VALID/STALE` slots.
- [ ] Singleflight hub misses.
- [ ] Track an invalidation ceiling and reject older hub responses.
- [ ] Install committed writes before returning for local read-your-writes.
- [ ] Apply invalidations without allocating unknown-key slots.
- [ ] Implement TTL, tombstone, eviction and stale-policy transitions.
- [ ] Give every background operation explicit cancellation/close ownership.

## Implementation detail

### Slot model (`internal/l1`)

```go
type slot struct {
    mu       sync.Mutex
    state    KeyState        // EMPTY | FETCHING | VALID | STALE
    rec      *record         // immutable once published; replaced, never mutated
    ceiling  wire.VersionTag // highest invalidation seen for this key
    fetch    *fetchCall      // singleflight in-flight fetch, nil otherwise
    expireAt time.Time
}
type record struct { value []byte; version wire.VersionTag; kind RecordKind }
```

Records are immutable; a transition swaps the `*record` pointer under `slot.mu`
so readers copy-out a stable snapshot. Slots live in the sharded map from P0.

### Read path

```text
Get(key):
  VALID & not expired            -> return copy (no network, no authority)
  EMPTY/STALE/expired            -> singleflight fetch:
      FETCHING; hub.Get(key, min=ceiling)
      on return: if resp.version < ceiling -> reject (a newer invalidation won);
                 re-fetch or serve per stale policy
                 else install VALID/tombstone, wake waiters
```

- **Singleflight:** concurrent misses on one key share a single `fetchCall`; the
  first installs, the rest read the result. One hub request per key per race.
- **Ceiling:** an invalidation arriving during a fetch raises `ceiling` but does
  **not** cancel in-flight waiters; the response is accepted only if
  `resp.version >= ceiling`, otherwise it is rejected and re-fetched — this is
  the invalidate-during-fetch guard.

### Write path (local install)

`Set`/`Delete` call the hub, then install the returned committed version into
the local slot **before** returning, giving read-your-writes for the writing
process. A tombstone installs as `record{kind: Tombstone}` and reads as a miss.

### Invalidation apply

```text
apply(event):
  slot absent           -> ignore (never allocate a slot for an unknown key)
  event.version <= rec  -> idempotent (covers same-version self-invalidation)
  else                  -> raise ceiling; mark STALE (or drop per policy)
```

### Stale policy & TTL

`StaleNever` evicts on invalidation/expiry; `StaleIfError` serves stale only when
a revalidation fetch errors; `ServeStaleWhileRevalidate` serves the stale record
while a background singleflight refresh runs. Every background op
(refresh, expiry sweep) has explicit `context`/close ownership so `Stop` joins
them.

## Verification

- [ ] Table tests for every transition.
- [ ] Fetch/invalidate, fetch/write, delete/read and expiry races.
- [ ] Concurrent misses make one hub request.
- [ ] Race detector and local-hit benchmark stub.

**Exit:** state-machine and race tests pass; a valid hot hit performs no network
or global-authority work.
