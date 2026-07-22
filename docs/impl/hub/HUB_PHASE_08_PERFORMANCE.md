# Hub P8 — Performance

**Depends on:** Hub P5 baselines.

**Common contract:** [COMMON_PHASE_08_PERFORMANCE.md](../common/COMMON_PHASE_08_PERFORMANCE.md).

## Functional work

- [ ] Benchmark Get, memory commit, durable sync, recovery, replay and fanout
  independently.
- [ ] Profile lock contention, allocation, batching and subscriber queues.
- [ ] Optimize behind storage/clock/transport seams only when measured.
- [ ] Preserve buffered portable I/O as the development baseline.
- [ ] Include index and replay overhead in capacity accounting.
- [ ] Report durable write amplification, compaction stalls, storage-cache
  duplication, recovery time and disk capacity separately from memory mode.
- [ ] Measure Fast queue saturation and Sync fence latency at multiple backlog
  depths; do not combine them into one write-latency number.

## Implementation detail

### What to isolate

- Separate benchmarks for `Get` (memory read), memory commit, durable
  `AppendSync`, WAL recovery time, replay serving and stream fanout — each a
  distinct `testing.B` so a regression is attributable.
- Profile lock contention on `partition.mu`, per-mutation allocation, group-commit
  batch sizing, and subscriber send-queue behavior under fanout. Prefer sharding
  partitions and pooling encode buffers over widening lock scope.

### Optimization seams

- Only optimize behind the storage (`DurabilityStore`), clock and transport
  interfaces so the atomic-commit and ordered-publish invariants are unchanged.
- Keep buffered portable file I/O as the durable baseline; any OS-specific fast
  path is opt-in and measured.
- Report durable write amplification, compaction stalls, storage-cache
  duplication, recovery time and disk capacity **separately** from memory-mode
  numbers, and Fast-queue saturation separately from Sync fence latency at
  several backlog depths.

## Verification

- [ ] Before/after benchmark evidence for every optimization.
- [ ] Durability, recovery, race and protocol suites remain unchanged and green.

**Exit:** documented gains exist under representative load without weakening
the atomic commit or replay invariants.
