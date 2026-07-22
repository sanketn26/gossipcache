# Node P8 — Performance

**Depends on:** Node P5 baselines.

**Common contract:** [COMMON_PHASE_08_PERFORMANCE.md](../common/COMMON_PHASE_08_PERFORMANCE.md).

## Functional work

- [ ] Benchmark valid hit, miss, stale hit, invalidation apply and contention.
- [ ] Profile slot locking, allocation, byte copies and singleflight fan-in.
- [ ] Optimize the local fast path only from measured evidence.
- [ ] Preserve copy boundaries, race safety and state-machine behavior.
- [ ] Report end-to-end miss/write separately from local-hit latency.

## Implementation detail

### Fast-path focus

- Dedicated `testing.B` for: valid hit, cold miss, stale hit, invalidation apply
  and slot contention — separated so a hit regression is attributable.
- Profile `slot.mu` hold time, per-read allocations, the `wire.CopyBytes`
  boundary copies, and singleflight fan-in under concurrent misses. Prefer
  reducing copies (single copy-out per read) and shard count tuning over
  widening locks.

### Invariants held while optimizing

- The valid-hit path stays free of network and global-authority work — any
  optimization that adds either is rejected regardless of throughput gain.
- Copy boundaries, race safety and state-machine transitions are preserved;
  end-to-end miss/write latency is reported separately from local-hit latency so
  the two are never averaged.

## Verification

- [ ] Before/after benchmark evidence and allocation counts.
- [ ] State-machine, race and integration suites remain green.

**Exit:** documented local-path gains exist without adding network work to a
valid hit or weakening correctness.
