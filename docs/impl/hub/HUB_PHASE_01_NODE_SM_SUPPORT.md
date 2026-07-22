# Hub P1 — Node state-machine support

**Depends on:** [HUB_PHASE_00_FOUNDATION.md](HUB_PHASE_00_FOUNDATION.md).

**Common contract:** [COMMON_PHASE_01_TEST_CONTRACT.md](../common/COMMON_PHASE_01_TEST_CONTRACT.md).

P1 is primarily node-owned. Hub work is limited to keeping the fake precise
enough to drive every node transition deterministically.

## Functional work

- [ ] Support `min_version` and `NOT_CAUGHT_UP` test responses.
- [ ] Support versioned tombstones and expiry in fake responses.
- [ ] Expose deterministic hooks for blocked fetch, delayed write and failure.
- [ ] Preserve the same response semantics planned for the real hub.

## Implementation detail

### Fake precision surface

The P0 fake gains exactly the response cases the node state machine must
traverse — no scheduling of real invalidation streams yet.

- `min_version` handling: if a `GetRequest.MinVersion` exceeds the partition's
  committed head for that key, return `NOT_CAUGHT_UP` (retryable); otherwise
  return the committed record.
- Tombstones: `Delete` stores `record{Kind: Tombstone, Version}`; a subsequent
  `Get` returns the tombstone version (not `NOT_FOUND`) until it is overwritten
  or expires, so the node can install a delete for read-your-writes.
- Expiry: a TTL crossing is modeled as a versioned mutation applied lazily on
  read — the fake advances `sequence`, records a tombstone-like expiry, and
  returns `NOT_FOUND`.

### Deterministic hooks

Implements the `FakeHooks` barriers from the P1 common contract:

```go
func (f *FakeHub) SetHooks(FakeHooks) // BeforeGetReturn / BeforeCommit / FailNext
```

Hooks block inside the call on a test-signalled channel so invalidate-during-
fetch, write/fetch and delete/fetch races are reproduced without sleeps. All
response semantics are byte-for-byte what the real P3 hub will return, so tests
carry forward unchanged.

## Verification

- [ ] Invalidate-during-fetch test can be orchestrated without sleeps.
- [ ] Write/fetch and delete/fetch races are reproducible.

**Exit:** all Node P1 state-machine tests can run through the hub interface with
no hub behavior mocked inside `internal/l1`.
