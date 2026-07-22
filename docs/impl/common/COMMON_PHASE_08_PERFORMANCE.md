# Common P8 — Performance contract

**Depends on:** [COMMON_PHASE_05_OBSERVABILITY.md](COMMON_PHASE_05_OBSERVABILITY.md)
baselines and all earlier correctness gates.

- [ ] Freeze representative local-hit, miss, write, fanout, replay and recovery
  benchmark workloads.
- [ ] Record environment, dataset, concurrency and durability settings.
- [ ] Require before/after evidence for optimizations.
- [ ] Keep state-machine, race, durability and protocol suites as regression
  gates.
- [ ] Report local and network paths separately.
- [ ] Report Fast acknowledgement, asynchronous persistence saturation and Sync
  fence latency separately, including backlog depth.

## Implementation detail

### Benchmark harness

- Go `testing.B` benchmarks under `test/benchmark/` plus a repeatable load
  driver in `cmd/loadgen` for multi-process runs. Every result records the
  environment header: CPU model, GOMAXPROCS, Go version, dataset size,
  concurrency, `storage_profile`, `write_mode`, and hub partition count.
- Fixed workloads (frozen so runs are comparable): `hit` (100% valid local),
  `miss` (cold fetch), `write_fast`, `write_sync`, `fanout` (1 writer / N
  subscribers), `replay` (forced gap), `recovery` (durable restart time).

### Hybrid reference profile `gc-v1-reference`

Unless a benchmark intentionally isolates one variable, multi-process reports
use this named profile: 1 Hub, 10 Node processes, 16 Hub partitions, 1,000,000
keys, 1 KiB values, 95:5 read/write ratio, Zipfian access (`s=1.1`), 64 client
workers per Node and all Nodes subscribed to partitions they hold. Run memory,
durable/Fast and durable/Sync profiles separately. Fault runs inject one dropped
stream range, one 5-second checkpoint stall and one Hub restart.

The provisional convergence objective is invalidation apply p99 below **500 ms**
from Hub commit eligibility to peer state-machine apply under the fault-free
reference profile. Reports include p50/p95/p99, throughput, allocations, queue
depth and hardware/runtime metadata. The threshold remains explicitly
provisional until measured baselines justify tightening or revising it.

### Reporting rules

- Local-path and network-path numbers are reported **separately** — a valid hit
  (`< 1ms` p99 objective; sub-µs only as a measured micro-benchmark claim) is
  never averaged with a hub round trip.
- Fast, Sync and async-persist-saturation numbers are reported separately with
  backlog depth as an axis; they are never collapsed into one "write latency".
- Optimizations require before/after evidence (`benchstat` delta with
  allocations) and must keep the state-machine, race, durability and protocol
  suites green as regression gates.
- Results that change the reference profile are not compared as regressions;
  both the old and new profiles must be reported during a profile revision.

**Exit:** performance claims are reproducible and no optimization weakens a
cross-component invariant.
