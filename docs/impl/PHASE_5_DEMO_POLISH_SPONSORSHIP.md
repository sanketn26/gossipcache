# Phase 5: Demo, Repository Polish, and Sponsorship

**Goal**: Turn GossipCache into a clear, demo-friendly open-source project that people can understand, try locally, star, and sponsor.

**Duration**: 1-2 weeks

**Prerequisites**: Phase 4 complete enough to run a stable multi-node backed-mode demo

**Status**: Not Started

## Overview

Phase 5 is the "show the work well" phase. The aim is not to add more core distributed systems behavior; it is to package the existing behavior into a crisp repository experience with a strong demo, useful docs, trustworthy examples, and a simple Buy Me a Coffee sponsorship path.

The project should feel like a polished solo open-source build: small enough to understand, real enough to run, and honest about trade-offs.

## Objectives

- [ ] Create a first-class local demo that starts a 3-node GossipCache cluster
- [ ] Show cache convergence after writes, updates, deletes, node restart, and backing-store recovery
- [ ] Add a simple visual demo dashboard or terminal demo script
- [ ] Add demo seed data and repeatable demo scenarios
- [ ] Add benchmark output comparing local cache hits with Redis/backing-store reads
- [ ] Rewrite the root README for clarity, screenshots, quick start, and positioning
- [ ] Add a "When to use / when not to use" section
- [ ] Add contribution, issue, and discussion templates
- [ ] Add GitHub repo metadata, topics, badges, and release notes
- [ ] Add Buy Me a Coffee links and funding metadata
- [ ] Record or script a short demo flow for social sharing
- [ ] Cut a demo-ready v0.1.0 release

## Positioning

Use this phase to sharpen the public story:

> GossipCache is a tiny Go-native local cache that keeps service instances warm using gossip.

Avoid positioning it as a Redis replacement. The strongest message is:

- Reads stay local.
- Gossip keeps nodes fresh enough.
- A backing store remains the source of truth in backed mode.
- Slight staleness is explicit and configurable.
- The project is educational, practical, and transparent about trade-offs.

## Implementation Steps

### Step 1: Demo Contract (Day 1)

Define exactly what the demo must prove before building presentation assets.

**Demo success criteria**:

- A new user can run one command and see a working cluster.
- Three nodes start locally with distinct HTTP and gossip ports.
- Redis or Valkey starts automatically for backed mode.
- A write to node 1 becomes readable locally from nodes 2 and 3 after gossip propagation.
- An update shows stale-then-fresh behavior clearly.
- A delete propagates cleanly.
- A node restart rejoins and warms up naturally.
- The demo prints or displays hit/miss, stale, propagation, and peer status.

**Suggested command**:

```bash
make demo
```

**Suggested files**:

```text
demo/
  docker-compose.yml
  seed.json
  scenarios/
    01-basic-convergence.sh
    02-update-and-staleness.sh
    03-delete-propagation.sh
    04-node-restart.sh
  README.md
```

### Step 2: Local Demo Environment (Day 1-2)

Create a repeatable local environment using Docker Compose.

**Components**:

- `redis` or `valkey` backing store
- `gossipcache-node-1`
- `gossipcache-node-2`
- `gossipcache-node-3`
- Optional demo UI or metrics viewer

**Quality bar**:

- No manual port hunting
- Clean logs with node IDs
- Health checks before scenarios run
- `make demo-down` removes containers and networks
- `make demo-logs` tails useful logs

### Step 3: Scenario Scripts (Day 2-3)

Add small scripts that tell a story through actual commands.

**Scenario 1: Basic convergence**

1. Set `product:123` on node 1.
2. Read immediately from node 2 and show miss or stale state if applicable.
3. Wait for gossip.
4. Read from node 2 and node 3.
5. Print propagation time.

**Scenario 2: Update and staleness**

1. Seed a value across all nodes.
2. Update the value through node 1.
3. Show old value may briefly exist on another node.
4. Show eventual fresh value.

**Scenario 3: Delete propagation**

1. Create a key.
2. Delete it through node 1.
3. Verify other nodes stop serving it after propagation.

**Scenario 4: Node restart**

1. Stop node 3.
2. Update several keys.
3. Restart node 3.
4. Show it rejoins and warms up naturally.

### Step 4: Visual Demo (Day 3-5)

Add either a lightweight web dashboard or a terminal UI. Keep it simple and reliable.

**Minimum dashboard view**:

- Node list and peer status
- Per-node key status for a small demo key set
- Hit/miss counters
- Last gossip event timestamp
- Propagation timeline
- Backing-store status

**Preferred implementation**:

- Static HTML/JS served from a demo endpoint or `demo/dashboard/`
- Poll node debug APIs every 500-1000ms
- No build-heavy frontend unless the repository already has that tooling

**Fallback implementation**:

- A terminal script that prints a live table using `watch`, `curl`, and `jq`

### Step 5: Benchmarks and Claims (Day 5-6)

Back the README claims with benchmark output generated from the repository.

**Benchmark cases**:

- Local in-memory cache hit
- Backed-mode local hit
- Redis direct read
- Backing-store miss and populate
- Gossip propagation latency under small cluster load

**Rules**:

- Do not overclaim.
- Include hardware and environment notes.
- Show p50, p95, and p99 where useful.
- Put raw benchmark output under `docs/benchmarks/`.
- Summarize the important result in the README.

### Step 6: README Rewrite (Day 6-7)

Make the root README the project's front door.

**Recommended README structure**:

```markdown
# GossipCache

Tiny Go-native local cache coherence using gossip.

## Why
## When To Use It
## When Not To Use It
## Quick Start
## Demo
## How It Works
## Backed Mode
## Independent Mode
## Benchmarks
## Status
## Roadmap
## Contributing
## Support
## License
```

**Include**:

- One strong diagram or screenshot from the demo
- One-command quick start
- A short Go usage example
- A clear early-development status note
- Links to architecture and implementation docs
- Buy Me a Coffee support link

### Step 7: Repository Polish (Day 7-8)

Add the files that make the repo feel cared for.

**Files**:

```text
.github/
  FUNDING.yml
  ISSUE_TEMPLATE/
    bug_report.yml
    feature_request.yml
    question.yml
  PULL_REQUEST_TEMPLATE.md
  workflows/
    ci.yml
CONTRIBUTING.md
CODE_OF_CONDUCT.md
SECURITY.md
CHANGELOG.md
```

**Repo settings checklist**:

- Description: `Tiny Go-native local cache coherence using gossip`
- Topics: `go`, `cache`, `distributed-systems`, `gossip-protocol`, `redis`, `valkey`, `eventual-consistency`
- Enable Discussions if you want design questions outside Issues
- Protect `main` once CI is stable
- Add a v0.1.0 milestone

### Step 8: Buy Me a Coffee Setup (Day 8)

Add sponsorship support without making the project feel salesy.

**Setup tasks**:

- Create or confirm Buy Me a Coffee page
- Add `.github/FUNDING.yml`
- Add a short `Support` section to README
- Add sponsor link to docs index
- Mention sponsorship in release notes

**Suggested README copy**:

```markdown
## Support

GossipCache is a solo open-source project. If it helped you learn something,
debug a design, or build a faster service, you can support the work here:

[Buy me a coffee](https://www.buymeacoffee.com/YOUR_HANDLE)
```

Replace `YOUR_HANDLE` before publishing.

### Step 9: Demo Recording and Launch Assets (Day 9-10)

Prepare a small launch package.

**Assets**:

- 60-90 second demo recording
- Terminal GIF or screenshot
- Short launch post
- v0.1.0 release notes
- Demo troubleshooting notes

**Demo recording outline**:

1. Start with `make demo`.
2. Show three nodes becoming healthy.
3. Write to node 1.
4. Watch nodes 2 and 3 converge.
5. Restart a node.
6. Show it rejoining.
7. End on the README support link.

### Step 10: v0.1.0 Release (Day 10)

Cut the first public demo release.

**Release checklist**:

- [ ] CI green
- [ ] `make demo` works from a clean checkout
- [ ] README quick start verified
- [ ] Demo scripts verified
- [ ] Benchmarks generated and committed
- [ ] Sponsorship links use the real handle
- [ ] License present
- [ ] Changelog updated
- [ ] GitHub release created with demo notes

## TDD and Verification Plan

Phase 5 still needs tests, but the emphasis is on user-facing repeatability.

| Area | Verification | Command |
|------|--------------|---------|
| Demo compose stack | Services become healthy | `make demo` |
| Demo scenarios | Scripts exit non-zero on failed convergence | `make demo-scenarios` |
| README commands | Fresh checkout instructions work | Manual clean-clone check |
| Dashboard | Shows all nodes and updates after writes | Browser or screenshot check |
| Benchmarks | Output committed and reproducible | `make bench` |
| CI | Tests, lint, and build pass | GitHub Actions |

## Success Criteria

Phase 5 is complete when:

- [ ] A visitor understands the project in under one minute from the README
- [ ] A developer can run the demo in under five minutes
- [ ] The demo proves local reads and eventual convergence
- [ ] The repository has issue templates, contribution docs, CI, and changelog
- [ ] Sponsorship links are present but tasteful
- [ ] v0.1.0 is tagged and has useful release notes

## Nice-To-Have Extras

- Animated SVG or GIF showing gossip propagation
- Hosted docs with a custom domain
- Comparison guide: GossipCache vs Redis client-side cache vs Hazelcast Near Cache
- Example apps:
  - Feature flags
  - Product catalog
  - Service config cache
- A "design notes" blog post explaining why gossip was chosen

## Anti-Goals

- Do not add major new distributed systems features in this phase.
- Do not claim production maturity unless the tests and operations story support it.
- Do not make sponsorship the first thing users see.
- Do not hide staleness or consistency trade-offs.

## Next Phase

After Phase 5, future work should be driven by user feedback from the demo release:

- Simplify the API if users struggle with setup
- Improve observability where the demo is confusing
- Add security hardening from [Phase 4.5](PHASE_4_5_SECURITY.md) if users deploy outside trusted networks
- Consider v0.2.0 features only after the v0.1.0 demo has been tried by real users
