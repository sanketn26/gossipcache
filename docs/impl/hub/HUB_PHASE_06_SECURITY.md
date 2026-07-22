# Hub P6 — Security

**Depends on:** frozen Hub P3 protocols; integrates with Hub P4 readiness.

**Common contract:** [COMMON_PHASE_06_SECURITY.md](../common/COMMON_PHASE_06_SECURITY.md).

## Functional work

- [ ] Require workload identity/mTLS on data and control ports in production.
- [ ] Validate protocol, cluster identity and `hub_generation` at connect.
- [ ] Authorize reads, mutations, subscriptions and management access.
- [ ] Rate-limit RPC, reconnect, subscription and debug traffic.
- [ ] Reload credentials without sequence or generation changes.

## Implementation detail

### Listener hardening

- RPC (`7400`) and control (`7401`) wrap `tls.Config{MinVersion: VersionTLS13,
  ClientAuth: RequireAndVerifyClientCert, ClientCAs: bundle}`. `GetConfigForClient`
  reads the CA/cert from an `atomic.Pointer` so rotation is hitless.
- After the TLS handshake the hub runs the common P6 validation order (SAN role +
  `cluster_id` → protocol range → `hub_generation`) before accepting any Get,
  mutation or Subscribe.

### Authorization map

```go
// cert role -> allowed operation classes
var hubACL = map[Role]OpSet{
    RoleNode:  {OpGet, OpSet, OpDelete, OpSubscribe},
    RoleAdmin: {OpManage},
}
```

Unauthorized op returns a terminal status and a bounded security event (SAN,
cluster, op — never key/value). Management access requires `RoleAdmin`.

### Rate limits and rotation

- Per-identity token buckets on connect/reconnect, subscribe and mutation paths;
  excess → `ERR_RATE_LIMITED`. Buckets are keyed by SAN identity, not IP.
- Credential reload swaps the cert pointer without changing any partition
  `sequence` or `hub_generation`; in-flight healthy connections continue.

## Verification

- [ ] Wrong CA/SAN/cluster/generation and expired credentials fail closed.
- [ ] Rotation and abuse tests preserve healthy clients and hub readiness.

**Exit:** an unauthenticated or incompatible node cannot read, mutate or
subscribe, and routine credential rotation does not alter data semantics.
