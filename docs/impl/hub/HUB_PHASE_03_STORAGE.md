# Hub P3 — Memory store and opt-in durability

**Depends on:** [HUB_PHASE_02_CONTROL_ORIGIN.md](HUB_PHASE_02_CONTROL_ORIGIN.md).

**Common contract:** [COMMON_PHASE_03_DATA_PROTOCOL.md](../common/COMMON_PHASE_03_DATA_PROTOCOL.md).

## Default memory profile

- [ ] Implement a complete, non-evicting sharded in-memory Hub table.
- [ ] Serialize each partition mutation across value/tombstone, version, expiry,
  stream sequence, dedup result and changefeed event.
- [ ] Publish the mutation and invalidation atomically to Hub readers/streams.
- [ ] Make TTL expiry a versioned mutation through the same partition path.
- [ ] Implement multi-partition routing and the real RPC server.
- [ ] On restart, start empty with a new `hub_generation` and require Node
  revalidation before readiness.

## Opt-in durable profile

- [ ] Put persistence behind a narrow `DurabilityStore` interface.
- [ ] Freeze a bounded, checksummed WAL/storage format or select an embedded
  engine through an explicit design decision.
- [ ] For Fast, enqueue the complete ordered persistence record before applying
  the atomic memory commit; acknowledgement does not wait for storage sync.
- [ ] For Sync, persist this mutation and all earlier partition mutations in one
  synchronous fence before memory visibility and acknowledgement.
- [ ] Recover the memory table, version heads, stream heads and replay state.
- [ ] Handle torn tails/corruption and define snapshot/compaction boundaries.
- [ ] Advertise the active storage profile in handshake, health and metrics.
- [ ] Implement a bounded ordered persistence queue per partition for Fast
  writes; never reorder version or stream sequences.
- [ ] Implement Sync as a partition fence that flushes all prior Fast mutations
  followed by the current mutation before success.
- [ ] Reject Sync without a healthy durable backend before memory commit.
- [ ] Define queue-full behavior explicitly (bounded backpressure or rejection;
  never silent persistence drop).
- [ ] On asynchronous persistence failure, expose degraded durability and reject
  Sync while retaining explicitly memory-only Fast semantics.

## Implementation detail

### Atomic partition transaction (`internal/l2`)

Every mutation runs one critical section per partition that produces value,
version, stream event and dedup result together:

```go
func (p *partition) commit(m Mutation) (commitResult, error) {
    p.mu.Lock(); defer p.mu.Unlock()
    if entry, ok := p.dedup.lookup(m.ID); ok {
        return entry.committedResultFor(m) // validates fingerprint; W handled by RPC
    }
    seq := p.sequence + 1
    ver := wire.VersionTag{PartitionID: p.id, Sequence: seq}
    ev  := invalidationEvent(m, ver, p.streamSeq+1)
    if p.durable != nil {                // durable profile
        if err := p.durable.stage(m.Mode, record, ev); err != nil { return commitResult{}, err }
    }
    p.table.apply(record)                // memory visibility
    p.sequence, p.streamSeq = seq, ev.StreamSequence
    p.dedup.storeCommitted(m, commitResult{Version: ver}) // includes fingerprint
    p.stream.publish(ev)
    return commitResult{Version: ver}, nil
}
```

`WriteFast` returns after `stage` enqueues (or immediately in memory mode);
`WriteSync` returns only after `stage` reports the fence durable. Version and
stream sequence are consumed only on success, so a rejected Sync leaves no hole.
The RPC layer attaches the committed dedup entry to one shared W waiter before
publication; retries join that waiter or replay its final outcome rather than
turning a previous W timeout into success.

### DurabilityStore seam (`internal/l2/durable`)

```go
type DurabilityStore interface {
    // Fast: append to the ordered per-partition queue; returns after buffered.
    AppendFast(partition uint32, rec Record, ev Event) error
    // Sync: flush all earlier queued Fast writes for the partition, then this
    // record, crossing the fsync boundary before returning.
    AppendSync(partition uint32, rec Record, ev Event) error
    Recover() (RecoverState, error) // rebuild table, version heads, stream heads
    Healthy() bool
}
```

- WAL format: length-prefixed, CRC32C-checksummed records
  `[len][crc][partition][version][stream_seq][kind][key][value/ttl]`. Torn tail
  on recovery is truncated at the last valid CRC.
- Fast path: one bounded ordered queue per partition drained by a single writer
  goroutine (group commit); queue-full applies backpressure or rejects with
  `ERR_RATE_LIMITED` — never a silent drop.
- Sync path: a fence enqueued behind outstanding Fast records; it fsyncs through
  its own record before signalling, so recovered sequences form a contiguous
  prefix with no holes.
- Recovery replays the WAL to reconstruct the memory table and per-partition
  version/stream heads; snapshot + compaction bound replay time. Unclean restart
  that may have lost an acked Fast tail advances `hub_generation` even on a valid
  prefix, forcing node revalidation.
- On async persist failure the store flips `Healthy()` false: new `WriteSync`
  returns `ERR_DURABILITY_UNAVAILABLE`, existing Fast memory results are retained
  (not rewritten), and `DurabilityDegraded` readiness/metric is exposed.

## Verification

- [ ] Both profiles pass the same Get/Set/Delete/TTL/protocol contract suite.
- [ ] Memory restart loses state, creates a different generation and cannot serve old Node
  entries as valid.
- [ ] Sync append/fence failure consumes no sequence and publishes nothing for
  the Sync mutation; asynchronous Fast failure is reported as degraded
  durability rather than rewriting the acknowledged memory result.
- [ ] Fast acknowledgement does not wait for storage sync.
- [ ] Sync waits for prior Fast backlog and recovery contains a contiguous prefix.
- [ ] All four Fast/Sync × W=0/W>0 result combinations are tested.
- [ ] Durable clean restart restores every Sync-acknowledged mutation and every
  Fast mutation known to have crossed the persistence boundary.
- [ ] An unclean restart that may have lost an acknowledged Fast tail recovers a
  valid persisted prefix under a new generation and forces Node revalidation.
- [ ] Retry idempotency and concurrent partition tests pass under `-race`.

## Durable-profile bottlenecks to expose

- Synchronous disk flush adds write tail latency and caps throughput.
- Per-partition ordering can turn storage latency into queueing latency.
- Sync latency includes the outstanding Fast persistence backlog.
- Bounded Fast queues introduce backpressure or write rejection under sustained
  storage lag.
- Group commit improves throughput but adds batching delay.
- WAL/SST compaction creates write amplification and latency variance.
- Values may exist in both the Hub memory table and storage-engine caches.
- TTL tombstones, replay retention and request dedup consume disk until GC.
- Startup recovery delays readiness and grows with unreduced log size.
- Disk capacity, corruption, backups and format upgrades become operator work.
- Local durability alone does not provide HA or survive machine/volume loss.

Metrics and benchmarks must report memory and durable profiles separately.

**Exit:** WriteFast is the default memory-latency path; WriteSync is available
only with opt-in healthy persistence, acts as an ordered durability fence, and
exposes its latency, throughput, recovery and capacity costs.

## HA scope

P3 implements one logical Hub process with internal partitions. Multi-replica
replication, leader fencing and automatic failover are post-v1 work; none may be
claimed by the P3 durability profile. The storage and generation contracts stay
compatible with a later replicated Hub, but a PVC is durability, not HA.
