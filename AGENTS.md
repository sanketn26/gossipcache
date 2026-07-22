# AGENTS.md

Guidance for AI coding agents working in this repository.

## Project Overview

GossipCache is a Go module for a distributed in-memory key-value cache that synchronizes nodes through a gossip protocol. The central design idea is that cache reads should stay local and fast; gossip is used to keep local caches eventually consistent across a cluster.

The module path is:

```text
github.com/sanketn26/gossipcache
```

**v1 product (locked):** hybrid in-process **L1** + native authoritative **L2 hub**
([docs/SEMANTICS.md](docs/SEMANTICS.md)). Redis/Postgres-as-SoT and independent
full-value gossip are out of scope for v1.

**Code today:** local in-memory L1 only. See [docs/STATUS.md](docs/STATUS.md).
Build order: [docs/impl/README.md](docs/impl/README.md) (Common, Hub and Node files per phase, P0–P8).

## Repository Map

- `pkg/gossipcache/`: public client-facing API. Keep exported API changes deliberate and documented.
- `pkg/gossipcache/inmemory/`: public constructor for local memory cache.
- `internal/cache/`: local L1 coordination over storage (pre–state-machine).
- `internal/storage/`: storage interfaces and errors.
- `internal/storage/memory/`: in-memory storage, sharding, TTL expiration, and eviction policies.
- `internal/config/`: configuration (L1 + L2 hub placeholders), loading, validation.
- `internal/observability/`: logging and Prometheus metrics support.
- `examples/server/`: local L1 example (`-tags example`).
- `test/benchmark/`: benchmark tests.
- `docs/`: architecture, deployment, diagrams, and implementation planning.

Target packages (`internal/l1`, `internal/l2`, `internal/control`, `cmd/l2`, …)
are planned in the phase files but not implemented yet. Verify the tree before editing.

## Development Commands

Prefer Make targets when they match the task:

```bash
make test        # go test -v -race ./...
make test-short  # go test -v -short ./...
make coverage    # race tests plus coverage report
make bench       # benchmarks
make fmt         # go fmt ./...
make vet         # go vet ./...
make lint        # golangci-lint if installed
make build       # build cmd/gossipcache into bin/
make all         # fmt, vet, lint, test, build
```

Direct Go commands are also fine for narrow work:

```bash
go test ./...
go test -run TestName ./path/to/package
go test -bench=. -benchmem ./...
go fmt ./...
go vet ./...
go mod tidy
```

Use `go mod tidy` only when dependency changes require it.

## Coding Guidelines

- Follow existing Go style and package boundaries.
- Keep public API in `pkg/gossipcache` small and stable.
- Put private implementation details under `internal/`.
- Preserve context-aware APIs. Existing cache and storage calls accept `context.Context`; new I/O, cache, gossip, or storage operations should do the same.
- Copy mutable byte slices at API/storage boundaries when retaining or returning values, so callers cannot mutate internal cache state.
- Keep concurrency changes race-safe. The default full test target uses `-race`; use it for changes touching shared state, goroutines, storage, or metrics.
- Avoid introducing network calls into local cache read paths unless the mode semantics explicitly require them.
- Prefer interfaces at package boundaries where the repo already uses them; avoid broad abstractions without a concrete caller.
- Keep comments useful and sparse. Document exported identifiers and non-obvious concurrency or consistency behavior.

## Go Software Development Best Practices

- Write simple, readable Go before clever Go.
- Use clear package boundaries. CLI entry points, configuration, cache orchestration, storage engines, observability, network/gossip behavior, and public client APIs should remain separate.
- Prefer small functions with explicit inputs, outputs, and error returns.
- Keep interfaces small and consumer-owned where practical. Define an interface close to the package that uses it, not automatically next to the implementation.
- Accept `context.Context` as the first argument for operations that may block, perform I/O, coordinate goroutines, or depend on cancellation.
- Return errors instead of panicking. Panics should be reserved for programmer errors or truly unrecoverable initialization mistakes.
- Wrap errors with useful context using `fmt.Errorf("...: %w", err)` and preserve sentinel errors where callers rely on `errors.Is`.
- Avoid package-level mutable state except for well-defined constants, registries, or metrics that are safe for concurrent use.
- Keep side effects at the edges. File I/O, subprocess execution, network calls, backing-store calls, and peer communication should be isolated behind small APIs that tests can replace.
- Validate all external inputs, especially config files, environment variables, node addresses, cache keys, TTLs, max sizes, protocol messages, and backing-store responses.
- Fail safely with actionable error messages.
- Do not ignore errors. If an error is intentionally ignored, make the reason obvious in code.
- Prefer dependency injection for storage engines, backing stores, clocks/timers when needed, loggers, metrics collectors, transports, and discovery providers.
- Copy mutable `[]byte`, maps, and slices when crossing API or storage boundaries if later mutation would corrupt internal state.
- Keep goroutine lifetimes explicit. Every background goroutine should have a cancellation or close path, and shutdown should be idempotent.
- Use `sync.Mutex`, `sync.RWMutex`, `atomic`, and channels deliberately. Favor the simplest concurrency primitive that fits the ownership model.
- Keep tests deterministic. Avoid sleeps when a controllable clock, short interval, or direct method call can make behavior reliable.
- Use `t.TempDir()` for filesystem behavior in tests.
- Avoid real network, backing-store, Docker, or service calls in unit tests. Keep integration and long-running tests separate from the normal package suite.
- Add tests for every behavior change, especially around TTLs, eviction, config validation, concurrent access, close semantics, gossip protocol behavior, and public API errors.
- Maintain backwards compatibility for exported types, methods, config keys, metrics names, and documented behavior unless intentionally changing them.
- Avoid broad refactors while fixing narrow issues.
- Make logging useful for debugging distributed behavior, but avoid leaking cache values, credentials, connection strings, tokens, or sensitive config.
- Prefer explicit configuration through config files, environment variables, or typed defaults in `internal/config` rather than hidden behavior.
- When adding new storage engines, backing stores, discovery providers, gossip message types, config options, metrics, or public API methods, update the implementation, docs, and tests together.

## SOLID Principles For Go

### Single Responsibility Principle (SRP)

- Each package, type, and function should have one clear reason to change.
- Keep command startup, config loading, cache coordination, storage, gossip/network behavior, observability, and public API wrappers separated.
- Avoid large manager types that own configuration, storage, networking, metrics, retries, and protocol behavior all at once.
- Cache orchestration should coordinate dependencies, not embed storage-engine or transport-specific details directly.

### Open/Closed Principle (OCP)

- Design stable behavior so it can be extended without repeatedly editing core orchestration.
- New storage engines, backing stores, eviction policies, discovery mechanisms, conflict resolvers, and transports should fit behind small interfaces or registration points when there is more than one real implementation.
- Prefer adapters and strategy types over large conditional chains when behavior is expected to grow.
- Do not add abstraction only for imagined future needs; add it when there is a concrete extension point in this repo.

### Liskov Substitution Principle (LSP)

- Implementations of an interface should be safely interchangeable.
- Any `storage.Storage` implementation should preserve expected return values, copy semantics, context behavior, close behavior, and error semantics.
- Mock or fake implementations in tests should behave like production implementations where callers depend on behavior.
- Avoid leaking implementation-specific assumptions from memory storage, backing stores, or transports into shared cache orchestration.

### Interface Segregation Principle (ISP)

- Keep interfaces focused and minimal.
- Do not force storage engines, backing stores, discovery providers, or transports to implement methods they do not need.
- Split read-only, mutating, lifecycle, streaming, or batch capabilities when a caller only needs one subset.
- Prefer small Go interfaces over large abstract contracts.

### Dependency Inversion Principle (DIP)

- High-level cache and service orchestration should depend on abstractions, not concrete storage, backing-store, discovery, or transport implementations.
- Inject dependencies instead of constructing concrete implementations deep inside orchestration code.
- Keep SDKs, wire formats, external clients, and provider-specific behavior isolated behind adapter packages.
- Make components replaceable for tests, local development, alternative backing stores, sandboxed execution, and future deployment environments.

## Architecture Invariants

- Local L1 cache access should remain the fast path.
- In hybrid v1, the L2 hub is the version authority; control plane carries invalidations only.
- Do not reintroduce Redis-as-SoT or UDP gossip control plane for v1.
- TTL and eviction behavior should be bounded, testable, and deterministic where practical.
- Shutdown paths should be idempotent and should stop goroutines cleanly.

## Testing Expectations

Add or update tests next to the package being changed. Important cases for this project include:

- TTL expiration and stale entry removal.
- Cache misses, hits, deletes, and multi-key operations.
- Eviction policy behavior under size pressure.
- Closed storage/cache behavior.
- Config defaults, validation, and env/file loading.
- Metrics/logging behavior when touching observability code.
- Race-sensitive paths for sharded maps, atomics, goroutines, and close handling.

For behavioral changes, run the smallest relevant package test first, then `make test` or `go test -race ./...` when feasible.

## Documentation

Update docs when changing architecture, public API, configuration, deployment assumptions, or operating-mode semantics. Start with:

- `README.md` for user-facing overview changes.
- `docs/ARCHITECTURE.md` for system design changes.
- `docs/TECHNICAL_SPEC.md` for protocol, data structure, or API details.
- `docs/DEPLOYMENT.md` for runtime and environment changes.
- `docs/impl/` for implementation-plan updates.

## Agent Workflow

1. Inspect the current files before assuming planned docs are implemented.
2. Check `git status --short` before editing and do not revert unrelated user changes.
3. Keep edits focused on the requested behavior.
4. Run `go fmt` on touched Go packages.
5. Run targeted tests, then broader tests when the change affects shared behavior.
6. In the final response, summarize changed files and verification performed. Mention any tests not run.

## Things To Avoid

- Do not make broad rewrites of package structure unless explicitly requested.
- Do not add dependencies for small utilities that the standard library can handle cleanly.
- Do not hide test failures by weakening assertions.
- Do not change module path, exported names, or config keys casually.
- Do not introduce background goroutines without clear shutdown ownership and tests.
