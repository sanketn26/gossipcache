# Sequences (v1 hybrid)

Normative rules: [../SEMANTICS.md](../SEMANTICS.md).

## Components

```text
App → L1 (state machine, singleflight)
        ├─ hit → local
        ├─ miss/write → L2 hub RPC
        └─ invalidation stream ← hub (subscribers of shard)
```

## Read hit

```mermaid
sequenceDiagram
    App->>L1: Get(key)
    L1->>L1: VALID
    L1-->>App: value
```

## Read miss

```mermaid
sequenceDiagram
    App->>L1: Get(key)
    L1->>L1: EMPTY → FETCHING
    L1->>L2: Get(key, min?)
    L2-->>L1: value, VersionTag
    L1->>L1: VALID if ≥ ceiling
    L1-->>App: value
```

## Write (W = 0 default)

```mermaid
sequenceDiagram
    participant A as L1 writer
    participant H as L2 hub
    participant B as L1 peer

    App->>A: Set(key,v)
    A->>H: Set RPC
    H->>H: value + VersionTag + stream event
    H-->>A: OK, VersionTag
    A->>A: install VALID
    A-->>App: OK
    H->>B: invalidate
    B->>B: VALID → STALE
    Note over B: next Get → H.Get
```

## Write with W &gt; 0 (hub-aggregated)

```mermaid
sequenceDiagram
    participant A as L1 writer
    participant H as L2 hub
    participant B as L1 peer

    App->>A: Set(..., W=1)
    A->>H: Set RPC (W=1 in request)
    H->>H: durable commit first
    H->>B: invalidate
    B->>B: apply if slot exists
    B-->>H: InvalidateConfirm (node_id dedup)
    Note over H: wait until W distinct nodes
    H-->>A: OK + VersionTag (or timeout error; commit stands)
    A->>A: install VALID (even on confirm timeout)
    A-->>App: OK or ErrWriteConfirmTimeout
```

## Fetch vs invalidation race

```mermaid
sequenceDiagram
    L1->>L2: Get (FETCHING)
    Note over L1: invalidation v=5 raises ceiling
    L2-->>L1: value v=4
    L1->>L1: reject
    L1->>L2: Get min=5
    L2-->>L1: v=5
    L1->>L1: VALID
```

## Stream gap / freshness

```mermaid
sequenceDiagram
    Hub->>L1: seq 10,11,13
    L1->>L1: gap 12
    L1->>Hub: replay 12
    alt available
        Hub-->>L1: 12
    else expired
        L1->>L1: RECONCILIATION_REQUIRED
    end
```

Silent stall (no checkpoints) → `/readyz` **STREAM_FRESHNESS_UNKNOWN**.

## Readiness

```mermaid
sequenceDiagram
    Kubelet->>L1: /livez
    L1-->>Kubelet: 200
    Kubelet->>L1: /readyz
    alt gap or stale checkpoint
        L1-->>Kubelet: 503
    else ok
        L1-->>Kubelet: 200
    end
```
