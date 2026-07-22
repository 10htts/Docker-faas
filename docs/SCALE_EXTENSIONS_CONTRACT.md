# Scale extension endpoints — versioned contract

**Status:** stable, additive extension. **Contract version:** `1.0.0`
(`pkg/faascontract.ContractVersion`, semantic; compatibility by MAJOR).

Docker-faas exposes two endpoints that are **not part of the OpenFaaS API**.
They are additive: no standard OpenFaaS route, method, payload, header, status
code, or label is altered by their presence. Official OpenFaaS clients
(faas-cli, SDKs) work unchanged with these endpoints ignored, and with idle
scale-to-zero disabled entirely (`IDLE_SCALE_TO_ZERO_ENABLED=false`, the
default) the provider behaves as a plain OpenFaaS-compatible gateway.
Negotiation is explicit: a control plane must first read
`/system/scale/capabilities` and proceed only when the capability it needs is
advertised.

Both endpoints sit behind the same gateway authentication as every other
`/system/*` route (basic auth / bearer token). The activity-lease endpoint
additionally requires message-level HMAC authentication (below).

---

## GET /system/scale/capabilities

Capability discovery for scale features. Read-only, unsigned (endpoint-auth
only), stable JSON:

```json
{
  "contract_version": "1.0.0",
  "provider": "docker-faas",
  "orchestration": "docker",
  "idle_scale_to_zero": true,
  "scale_from_zero": true,
  "kubernetes": { "supported": false, "reason": "..." }
}
```

- `idle_scale_to_zero` is `true` only when the provider can actually run the
  authenticated lease exchange (see below). A control plane MUST fail its own
  readiness rather than assume the capability when this is `false`.
- `kubernetes.supported` is always `false` for this provider; no Kubernetes
  claim is made.
- Unknown ADDITIONAL fields may appear in future MINOR versions; clients must
  ignore unknown fields. Fields are never removed or re-typed within a MAJOR.

Errors: `200` only (the document itself reports capability truthfully).

## POST /system/scale/activity-lease

The control plane (AIDrivenMES) periodically posts its durable work counts
(admitted / queued / running) for a function together with the generation
fence it sampled, and receives the provider's combined in-flight accounting
and idle decision. Wire schema: `pkg/faascontract.ActivityLeaseRequest` /
`ActivityLeaseResponse` — byte-identical in both repositories, with committed
golden fixtures under `pkg/faascontract/testdata/`.

### Message security model

| Layer | Mechanism |
|---|---|
| Transport | Gateway endpoint auth (basic/bearer); TLS termination recommended in front of the gateway |
| Authenticity | HMAC-SHA256 over a canonical field-labelled encoding (NOT raw JSON), domain-separated per message type; constant-time comparison |
| Secret | Dedicated, isolated shared secret `FAAS_ACTIVITY_LEASE_SECRET` (or `_FILE` for mounted secrets). Never reused from any other credential; no default; no fallback. Startup **fails closed** if idle scale-to-zero is enabled without it |
| Rotation | `FAAS_ACTIVITY_LEASE_SECRET_PREVIOUS[_FILE]` opens an overlap window: requests signed with active OR previous verify; responses are always signed with the ACTIVE secret. Remove the previous secret after rollover |
| Freshness | `issued_at` must lie within `now ± FAAS_ACTIVITY_LEASE_MAX_SKEW` (default 2m; `0` disables — logged, discouraged) |
| Replay | `nonce` is mandatory and single-use within the replay window (2×skew+1s). Bounded cache (default 65536 entries, `FAAS_ACTIVITY_LEASE_REPLAY_CAP`); at capacity the oldest entry is evicted and the request is still accepted (never rejected), so legitimate renewals never fail. Only authenticated requests reach the cache |
| Size | Request body limited to `FAAS_ACTIVITY_LEASE_BODY_LIMIT` (default 64 KiB; cannot be disabled) |
| Ordering | ALL checks run before any state mutation: a rejected request applies no lease, changes no counters, and issues no Docker command |
| Response binding | The response echoes the request `nonce` and is signed, so a captured older response cannot be substituted |

### Status codes

| Code | Meaning | Body |
|---|---|---|
| 200 | Lease processed; signed `ActivityLeaseResponse` | JSON, signed |
| 400 | Malformed JSON, failed structural validation, or missing nonce | text |
| 401 | Signature invalid, unknown secret, `issued_at` outside window, or nonce replayed | text (deliberately generic) |
| 409 | Contract MAJOR version mismatch (`provider_contract_version` + `peer_contract_version` in JSON body). The peer MUST treat this as its own readiness failure | JSON |
| 413 | Body over limit | text |
| 503 | Endpoint not wired, or no lease secret configured (fail closed) | text |

### Semantics

- `generation` in the request is the fence the counts were sampled under. A
  lease carrying an older generation than the live one is stored but reported
  `accepted: false` (`stale_generation`), and never protects the current
  replica.
- Lease counts expire after `lease_ttl_seconds` (default 30s). Expiry is
  fail-safe: a lost control plane cannot pin a function warm forever, and the
  idle window (default 300s) is an order of magnitude larger than the renewal
  period, so a live control plane cannot lose protection between renewals —
  including across a provider restart, where the first reconcile pass seeds
  the idle clock and therefore guarantees one FULL idle window before any
  post-restart reclaim.
- The response's `decision` (`hold` / `scale_to_zero` / `keep_warm`) is
  advisory telemetry for the control plane; the provider's reconciler is the
  only actor that executes reclaims, and it re-checks every fence under lock
  at execution time.

### Upgrade path

- MINOR/PATCH: additive fields only; both sides ignore unknown fields;
  signatures cover a fixed canonical field list per MAJOR, so additive fields
  never break verification.
- MAJOR: `contract_version` major bump; the provider answers `409` to old
  majors. Deploy order: upgrade provider (it keeps answering current major),
  then control plane. There is no silent downgrade: version mismatch is a
  hard readiness failure on the caller.
- Secret rotation is zero-downtime via the overlap variable (above).

### Metrics (Prometheus, `/system/metrics` or :9090/metrics)

`function_activity_leases_total{result}` with results: `accepted`,
`stale_generation`, `invalid`, `unauthenticated`, `version_mismatch`,
`unconfigured`, `too_large`, `skew`, `missing_nonce`, `replay`,
`replay_cache_evicted`; plus `function_scale_decisions_total{function_name,decision}`
and cold-start / reclaim counters. Logs never contain secrets or signatures.
