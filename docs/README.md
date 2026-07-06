# GossipCache Documentation

Welcome to the GossipCache documentation. This directory contains comprehensive technical documentation, architecture diagrams, and deployment guides.

## Documentation Structure

### Core Documentation

- **[STATUS.md](STATUS.md)** - **Single source of truth** for implementation status and the v1 scope contract; design docs describe the target, this describes reality
- **[adr/](adr/)** - Architecture decision records — both accepted and binding for v1:
  - [ADR-0001](adr/0001-gossip-transport.md): gossip transport is `hashicorp/memberlist` (no custom network layer)
  - [ADR-0002](adr/0002-evict-on-notify.md): backed-mode invalidation is evict-on-notify (no checksums, no tombstones, demand-driven pulls)
- **[ARCHITECTURE.md](ARCHITECTURE.md)** - High-level system architecture, design decisions, and component overview
- **[TECHNICAL_SPEC.md](TECHNICAL_SPEC.md)** - Detailed technical specifications including data structures, protocols, APIs, and interfaces
- **[DEPLOYMENT.md](DEPLOYMENT.md)** - Deployment guides for EC2, Docker, and Kubernetes environments
- **[impl/README.md](impl/README.md)** - Phased implementation plan, package structure, gap analysis, and testing strategy
- **[impl/IMPLEMENTATION_GUIDE.md](impl/IMPLEMENTATION_GUIDE.md)** - Hands-on implementation steps with code examples and checkpoints
- **[impl/PHASE_4_5_SECURITY.md](impl/PHASE_4_5_SECURITY.md)** - Optional security hardening plan for untrusted network deployments
- **[impl/PHASE_5_DEMO_POLISH_SPONSORSHIP.md](impl/PHASE_5_DEMO_POLISH_SPONSORSHIP.md)** - Demo, repository polish, and Buy Me a Coffee setup

### Diagrams

All diagrams use Mermaid format and can be viewed in any Markdown viewer that supports Mermaid (GitHub, VS Code, etc.).

- **[diagrams/BACKED_MODE_SEQUENCES.md](diagrams/BACKED_MODE_SEQUENCES.md)** - Sequence diagrams for backed mode operations:
  - Cache read/write flows
  - Gossip change detection and pull mechanism
  - Backing store failure handling
  - Singleflight pattern
  - Anti-entropy synchronization
  - Node join/bootstrap process

- **[diagrams/INDEPENDENT_MODE_SEQUENCES.md](diagrams/INDEPENDENT_MODE_SEQUENCES.md)** - Sequence diagrams for independent mode operations:
  - Vector clock-based conflict detection
  - Conflict resolution strategies (LWW, custom merge, siblings)
  - Network partition and healing
  - Data propagation via gossip
  - TTL expiration and tombstones

- **[diagrams/COMPONENT_DIAGRAMS.md](diagrams/COMPONENT_DIAGRAMS.md)** - Component interaction diagrams:
  - Read/write operation flows
  - Gossip message processing
  - Anti-entropy synchronization
  - Node join and failure detection
  - Memory management and eviction
  - Health check flows

## Quick Navigation

### By Role

**For Architects:**
- Start with [ARCHITECTURE.md](ARCHITECTURE.md) for system overview
- Review design decisions and trade-offs
- Understand consistency models

**For Developers:**
- Read [TECHNICAL_SPEC.md](TECHNICAL_SPEC.md) for API and interface details
- Review data structures and protocols
- Check sequence diagrams for implementation flows
- Use [Phase 5](impl/PHASE_5_DEMO_POLISH_SPONSORSHIP.md) when preparing the public demo and v0.1.0 release

**For DevOps/SRE:**
- Read [DEPLOYMENT.md](DEPLOYMENT.md) for deployment guides
- Review monitoring and observability sections
- Check troubleshooting guides

### By Topic

**Operating Modes:**
- Backed Mode: [Architecture](ARCHITECTURE.md#backed-mode), [Sequences](diagrams/BACKED_MODE_SEQUENCES.md), [Deployment](DEPLOYMENT.md#backed-mode-with-elasticache-redis)
- Independent Mode: [Architecture](ARCHITECTURE.md#independent-mode), [Sequences](diagrams/INDEPENDENT_MODE_SEQUENCES.md), [Deployment](DEPLOYMENT.md#independent-mode-no-backing-store)

**Gossip Protocol:**
- [Protocol Overview](ARCHITECTURE.md#51-gossip-protocol)
- [Message Specifications](TECHNICAL_SPEC.md#5-protocol-specifications)
- [Backed Mode Gossip](diagrams/BACKED_MODE_SEQUENCES.md#5-gossip-change-detection--pull)
- [Independent Mode Gossip](diagrams/INDEPENDENT_MODE_SEQUENCES.md#4-gossip-data-propagation-no-conflicts)

**Conflict Resolution:**
- [Design Decisions](ARCHITECTURE.md#53-conflict-resolution-independent-mode)
- [Technical Spec](TECHNICAL_SPEC.md#53-conflict-resolution-independent-mode)
- [Sequence Diagrams](diagrams/INDEPENDENT_MODE_SEQUENCES.md#6-conflict-resolution-strategies)

**Deployment:**
- [EC2 Deployment](DEPLOYMENT.md#ec2-deployment)
- [Docker Deployment](DEPLOYMENT.md#docker-deployment)
- [Kubernetes Deployment](DEPLOYMENT.md#kubernetes-deployment)

**Monitoring:**
- [Observability](ARCHITECTURE.md#monitoring--observability)
- [Metrics](TECHNICAL_SPEC.md#101-metrics-prometheus-format)
- [Operations](DEPLOYMENT.md#monitoring--operations)

## Key Concepts

### Core Philosophy

> **Caches must be local.** If accessing a cache requires a network call, you're just pushing the problem elsewhere.

GossipCache provides microsecond-level local cache access while maintaining eventual consistency across nodes via gossip protocol.

### Performance Targets

- **Local cache access**: < 1ms (memory speed)
- **Cache hit**: No network call required
- **Performance gain**: 100-1000x faster than direct database access

### Operating Modes

**Backed Mode:**
- Invalidation-only gossip: key + version, evict on notify ([ADR-0002](adr/0002-evict-on-notify.md))
- Pulls happen only on demand, singleflighted, on the next local read
- Redis/Valkey in v1; Postgres/MySQL planned ([STATUS.md](STATUS.md))
- Backing store is source of truth

**Independent Mode:**
- Full-data gossip (includes values)
- No external dependencies
- Vector clock-based conflict resolution
- Suitable for ephemeral data

## Mermaid Diagram Support

All sequence diagrams in this documentation use Mermaid syntax. To view them:

### In GitHub
Diagrams render automatically in GitHub's Markdown viewer.

### In VS Code
Install the "Markdown Preview Mermaid Support" extension.

### In Other Tools
Use any Markdown viewer with Mermaid support, or copy diagrams to [mermaid.live](https://mermaid.live) for rendering.

## Contributing to Documentation

When adding new documentation:

1. **Place files appropriately:**
   - High-level docs: `docs/` root
   - Diagrams: `docs/diagrams/`

2. **Use consistent formatting:**
   - Follow existing structure
   - Include table of contents for long docs
   - Use Mermaid for all diagrams

3. **Link liberally:**
   - Cross-reference related sections
   - Link to code when relevant
   - Keep navigation easy

4. **Update this README:**
   - Add new documents to structure
   - Update quick navigation if needed

## Additional Resources

- **Main README**: [../README.md](../README.md)
- **CLAUDE.md**: [../CLAUDE.md](../CLAUDE.md) - Guidance for AI assistants
- **GitHub Repository**: https://github.com/sanketn26/gossipcache
- **Documentation Site**: https://docs.gossipcache.io (coming soon)

## Feedback

Found an issue with the documentation? Have a suggestion?

- Open an issue: https://github.com/sanketn26/gossipcache/issues
- Discuss: https://github.com/sanketn26/gossipcache/discussions

---

**Last Updated**: 2026-07-06
**Version**: 0.1.0
