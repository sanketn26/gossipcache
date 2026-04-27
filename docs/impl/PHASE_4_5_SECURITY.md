# Phase 4.5: Security Hardening

**Goal**: Add production security controls for clusters that run outside a fully trusted private network.

**Duration**: 1-2 weeks

**Prerequisites**: Phase 4 complete

**Status**: Optional / Future

## Overview

Phase 4.5 closes the security gaps identified during implementation-plan review. It is optional for an MVP deployed in a trusted network, but should be treated as required before exposing the HTTP API or gossip ports across untrusted networks.

## Objectives

- [ ] TLS for all TCP gossip traffic
- [ ] Optional mTLS for node authentication
- [ ] Shared-secret authentication for join requests and gossip messages
- [ ] HTTP API authentication
- [ ] Rate limiting for API and gossip ingress
- [ ] Certificate and secret rotation runbook

## Implementation Steps

### Step 1: TLS for Gossip Transport

Add TLS config to the network layer without changing the gossip engine interface.

```go
// internal/network/tls.go
package network

import (
    "crypto/tls"
    "crypto/x509"
    "os"
)

type TLSConfig struct {
    Enabled            bool
    CertFile           string
    KeyFile            string
    CAFile             string
    MutualTLS          bool
    InsecureSkipVerify bool
}

func BuildTLSConfig(cfg TLSConfig) (*tls.Config, error) {
    if !cfg.Enabled {
        return nil, nil
    }

    cert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
    if err != nil {
        return nil, err
    }

    tlsCfg := &tls.Config{
        MinVersion:         tls.VersionTLS13,
        Certificates:       []tls.Certificate{cert},
        InsecureSkipVerify: cfg.InsecureSkipVerify,
    }

    if cfg.CAFile != "" {
        caPEM, err := os.ReadFile(cfg.CAFile)
        if err != nil {
            return nil, err
        }
        roots := x509.NewCertPool()
        roots.AppendCertsFromPEM(caPEM)
        tlsCfg.RootCAs = roots
        tlsCfg.ClientCAs = roots
    }

    if cfg.MutualTLS {
        tlsCfg.ClientAuth = tls.RequireAndVerifyClientCert
    }

    return tlsCfg, nil
}
```

### Step 2: Node Join Authentication

Protect `JoinRequest` and other membership messages with a shared HMAC when mTLS is not enabled.

```go
type AuthConfig struct {
    Enabled bool
    Method  string // "shared_secret" or "mtls"
    Secret  string
}

type SignedMessage struct {
    Message   []byte
    Timestamp int64
    Signature []byte
}
```

Validation rules:
- Reject unsigned join requests when auth is enabled.
- Reject stale timestamps outside a short clock-skew window.
- Use constant-time signature comparison.
- Rotate secrets by accepting current and previous secrets during a bounded transition window.

### Step 3: HTTP API Authentication

Add middleware in `internal/api/middleware.go`.

```go
func APIKeyAuth(next http.Handler, expected string) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+expected)) != 1 {
            http.Error(w, "unauthorized", http.StatusUnauthorized)
            return
        }
        next.ServeHTTP(w, r)
    })
}
```

### Step 4: Rate Limiting

Add bounded token-bucket limiters for:
- HTTP API requests per client IP
- Gossip messages per peer
- Join requests per source address

Rate-limit events should increment Prometheus counters and include peer/source labels with bounded cardinality.

### Step 5: Rotation Runbooks

Document:
- How to roll TLS certificates without downtime
- How to rotate shared secrets with current/previous secret overlap
- How to validate mTLS in Kubernetes and EC2 deployments
- How to disable debug and pprof endpoints on public interfaces

## Tests

- TLS handshake succeeds with a trusted CA.
- TLS handshake fails with an unknown CA.
- mTLS rejects clients without certificates.
- HMAC validation rejects tampered and stale messages.
- API middleware rejects missing or incorrect credentials.
- Rate limiter sheds excess load without blocking healthy peers.

## Deliverables

- [ ] `internal/network/tls.go`
- [ ] Auth config in `internal/config`
- [ ] Signed membership/gossip message validation
- [ ] HTTP auth middleware
- [ ] Rate limiter middleware
- [ ] Security runbook updates in deployment docs
