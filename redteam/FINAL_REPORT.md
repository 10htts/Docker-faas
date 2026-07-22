# Final report — Docker-faas OpenFaaS conformance + production-readiness

**Date:** 2026-07-21 · **Verdict:** ✅ **PRODUCTION-READY** for the pinned
OpenFaaS provider contract and idle scale-to-zero, with the documented residual
items below (one security recommendation before exposing the secrets extension
to untrusted networks).

## 1. Pinned versions

| Component | Pin | Source |
|---|---|---|
| github.com/openfaas/faas-provider (types, logs) | **v0.25.12** | raw.githubusercontent.com/openfaas/faas-provider/v0.25.12 |
| github.com/openfaas/faas (gateway OpenAPI) | **0.27.13** | api-docs/spec.openapi.yml @ 0.27.13 |
| faas-cli (test client) | **0.18.0** | local binary + github.com/openfaas/faas-cli @ 0.18.0 |
| openfaas/nats-queue-worker (callback headers) | **0.14.2** | main.go postResult |
| faascontract wire contract | **1.0.0** (semantic, MAJOR-compat) | `pkg/faascontract` |
| Go toolchain | go1.25.1 (host) / golang:1.25 (container build + race) | — |

## 2. Endpoint matrix (mandatory OpenFaaS surface)

All mandatory operations are implemented and **proven with the pinned faas-cli
0.18.0 against a live containerized gateway**. Full per-field detail in
`CONFORMANCE_MATRIX.md`.

| Operation | Route | Status | Proven by |
|---|---|---|---|
| List functions | GET /system/functions | supported | faas-cli `list` + tests |
| Deploy | POST /system/functions | supported | faas-cli `deploy` + tests |
| Update | PUT /system/functions | supported (202 vs spec 200; cli accepts) | tests |
| Delete | DELETE /system/functions | supported (202 vs 200; cli accepts) | faas-cli `remove` |
| Get one function | GET /system/function/{name} | supported (NEW) | faas-cli `describe` |
| Scale | POST /system/scale-function/{name} | supported (max-clamp) | live scale 2 + clamp 9→3 |
| System info | GET /system/info | supported (pinned `provider` key) | faas-cli `version` |
| Health | GET /healthz | supported (503 vs 500 unhealthy) | live |
| Namespaces | GET /system/namespaces | supported (NEW) | faas-cli `namespace list` |
| Function logs | GET /system/logs | supported (NEW: pinned NDJSON) | faas-cli `logs` |
| Sync invoke | ANY /function/{name}(.ns) | supported | faas-cli `invoke` (figlet) |
| Async invoke | POST /async-function/{name} | supported (X-Call-Id + X-Callback-Url) | live 202 + callback validation |
| Secrets CRUD | GET/POST/PUT/DELETE /system/secrets | supported (CRUD works via faas-cli) | live create 201/list/update 200/remove 200 |

**Official labels honored:** `com.openfaas.scale.zero`, `scale.zero-duration`,
`scale.min`, `scale.max` — with custom `com.docker-faas.scale.*` as additive
overrides (precedence: custom > official > config default).

**Additive extensions (never alter standard schemas):** /system/scale/capabilities,
/system/scale/activity-lease (HMAC-authenticated), /system/builds*,
/system/function/{name}/containers, /system/config, /system/metrics, /ui, /auth/*.
Documented in `docs/SCALE_EXTENSIONS_CONTRACT.md`. With `IDLE_SCALE_TO_ZERO_ENABLED=false`
(default) the provider is a plain OpenFaaS-compatible gateway.

## 3. The four HIGH risks (all resolved)

1. **Activity-lease authentication** (RT-201, RT-214) — dedicated file-loaded
   HMAC secret, verify-before-mutation, constant-time compare, body limit (413),
   issued-at skew (401), single-use nonce replay cache, secret-rotation overlap,
   signed + nonce-bound responses, fail-closed startup + 503.
2. **Deterministic generation-fenced cold start** (RT-202, RT-213) — ready-state
   honored (no late re-election), single leader, waiter fan-out, ctx cancellation,
   failed-start retry, crash demotion (`ReportZeroObserved`), panic-safe leader,
   sleep-free tests incl. 100 deterministic cold-start cycles.
3. **Scale-to-zero safety across concurrency/restart** (RT-203, RT-211, RT-223) —
   in-flight/durable-lease accounting, restart idle-window seeding, idempotent
   reclaim, drain/rollback, and **gateway-ownership-scoped orphan cleanup** so a
   reconciler never touches another instance's containers.
4. **Additive versioned extension contract** (RT-204) — full security/error/upgrade
   documentation for the two /system/scale/* endpoints.

## 4. Objection outcomes

**42 objections raised (14 HIGH, 12 MED, 16 LOW). 38 CLOSED with tests, 4 DEFERRED**
(RT-209 secret-value endpoint, RT-225 network-rename footgun, RT-233 legacy-orphan
cleanup, RT-239 same-name shared-daemon limitation — all documented with
rationale). No HIGH objection remains open.

Two adversarial rounds: (1) critic→builder→adjudicator with two critics on the
lease-auth stack and gate state machine (→ RT-215..223); (2) a validation round
with two independent critics re-examining the OpenFaaS conformance code and the
scale-to-zero safety code against the final integrated tree (→ RT-224..233). The
validation round confirmed the load-bearing safety properties and surfaced the
per-function ownership gap (RT-224, HIGH — an incomplete RT-223 fix), a reconciler
panic-safety gap (RT-227), a log-streaming goroutine leak (RT-228), a lease-anchor
prune hole (RT-226), and several LOW items — all fixed with tests. See
`OBJECTIONS.md` + `ADJUDICATION.md`.

## 5. Verification (exact results in `VERIFICATION.md`)

- `gofmt` clean on all session-changed files; `go build ./...` ok; `go vet ./...` clean.
- **All packages pass** (host, CGO off), including gateway, scaletozero,
  provider, router, faascontract, store, secrets, config, builder, middleware,
  and auth.
- Concurrency stress: scaletozero ×50, storm/cold-start ×30, gateway lease/coldstart ×20 — all stable.
- **Race detector (golang:1.25 Linux container, CGO on): full suite race-clean**,
  plus storm/cold-start ×10 under `-race`. (Host has no C toolchain; RT-208.)
- **faas-cli 0.18.0 end-to-end** against a live containerized gateway: every
  mandatory operation verified (table §2), including real figlet output and NDJSON logs.

## 6. Unsupported / partial optional features (honest gaps)

- `GET /system/secrets/{name}` returns the secret **value** — **not an OpenFaaS
  operation** and a security concern. **Recommendation: gate behind an explicit
  opt-in flag + audit logging, or remove, before exposing to untrusted networks.**
  (RT-209, DEFERRED — additive extension, not a mandatory-op break.)
- `FunctionStatus.invocationCount` always 0; `usage` (CPU/RAM) never populated;
  `constraints` accepted on deploy but ignored by the Docker provider — additive
  gaps that do not break standard clients.
- Secrets CRUD works via faas-cli. One status-code divergence is deliberate:
  DELETE `/system/secrets` returns 200 instead of the pinned OpenAPI 204 because
  faas-cli 0.18.0 accepts only 200/202 for `secret remove`.
- PUT/DELETE return 202 (spec 200); /healthz unhealthy returns 503 (spec 500) —
  both accepted by faas-cli 0.18.0; deliberately unchanged, flagged.
- Kubernetes provider: explicitly unclaimed (`capabilities.kubernetes.supported=false`).

## 7. Residual risks

- **Shared-daemon multi-instance is limited (RT-239).** Container names are
  globally `<function>-<replica>`, so two gateways cannot run a same-named
  function on one Docker daemon (name collision). The supported shared-daemon
  configuration is: enable `FAAS_STRICT_CONTAINER_OWNERSHIP=true` AND use disjoint
  function names per gateway. **A single gateway per daemon (the documented,
  primary target) has no such limitation.** Namespacing container names by owner
  would lift this and is recommended future work.
- After upgrading to the ownership-label build, a gateway's pre-upgrade containers
  are unlabeled. Recreate them while still in lenient mode, verify the ownership
  labels, then enable strict mode and restart. Container labels are immutable;
  strict mode intentionally refuses to adopt or delete an unlabeled name.
- Race-cleanliness proven **in-container** only (Windows host lacks cgo). CI must
  run `-race` on Linux (now includes `pkg/router`, previously untested).
- The Docker-daemon log-streaming path (`StreamFunctionLogs`) is exercised live
  (smoke test) and via unit-tested parsing/demux helpers, but has no
  daemon-bound unit test.
- Migration 3 (annotations) is verified to migrate an old-schema DB cleanly (cgo
  test) — back up production DBs before upgrade as standard practice.
- **The RT-234..240 fixes are recent and were prompted by external review that
  caught P1 gaps two internal review rounds missed** (see §8). The requested
  independent follow-up found and closed RT-240. A subsequent isolated live
  smoke on the exact tree confirmed that strict mode rejects a stopped foreign
  same-name container without deleting or relabeling it. The tree is uncommitted
  (this session performs no Git operations by directive).

## 8. Disclosed side effect

During the FIRST idle-enabled container smoke run (before the RT-223 fix), the
smoke gateway's orphan sweep reclaimed two function containers of a separate
docker-faas instance on the shared Docker daemon (`import-bundle`,
`gateprobesquare`). Impact recoverable (metadata persists; reclaim == scale-to-
zero, self-heals on next invoke; the other gateway process stayed healthy). It
exposed RT-223, now fixed (ownership label + scoped orphan scan) and re-verified
live (0 reclamations post-fix). Full disclosure in `OBJECTIONS.md` / `VERIFICATION.md`.

## 8b. External review correction (codex)

An independent external review after the validation round found three HIGH
verdict-blockers the two internal rounds had MISSED — a fatal router map-write
race (RT-236), an unbounded async response read (RT-237), and reserved-label
override (RT-235) — plus two MED and one limitation. The prior PRODUCTION-READY
verdict was **not supported** while those stood. Root cause: internal critics
used fake routers (never exercised the real `Router` under `-race`) and focused
on the scale subsystem, not the invoke/router hot path. All six are now addressed
(three HIGH closed with tests + a clean router `-race` run; two MED closed; one
documented as a limitation). This is recorded plainly rather than smoothed over.

## 8c. Independent follow-up correction (codex)

The recommended independent pass over RT-234..239 found RT-240: strict ownership
was enforced in list/reclaim/routing paths but bypassed by stale-name cleanup in
the create path. A stopped foreign or ambiguous legacy container occupying the
daemon-global target name could be force-removed before create. The provider now
uses the same ownership predicate for both list filtering and stale-name cleanup;
foreign ownership is always rejected and unlabeled names are rejected in strict
mode. Four ownership combinations are covered through the real Docker SDK fake.
The pass also added direct tests for the RT-237 callback body cap/no-callback drain
and exposed the new ownership/SSRF flags through Compose `.env` substitution.
An isolated real-daemon smoke then confirmed the RT-240 conflict path: the pinned
`faas-cli` deployment failed safely and the stopped foreign container remained
unchanged. Health, deploy, list, sync/async invoke, scale, owner labels, and
remove also passed; all uniquely named test resources were torn down.

## 8d. Live-smoke correction (codex)

The hardened smoke harness then exposed two more misses: RT-241, a real
`faas-cli secret remove` incompatibility (CLI sends a JSON `Secret` body and
accepts only 200/202, while the gateway read only `?name=` and returned 204),
and RT-242, an invalid logs smoke command using non-existent `--follow=false`.
Both are closed. `HandleDeleteSecret` now accepts the pinned JSON body, validates
namespace, preserves the legacy query fallback, and returns 200; the harness uses
`faas-cli logs <name> --tail=false --lines 20`. A fresh isolated run
(`SMOKE_ID=codexfix-20260721`) passed `PASS=25 FAIL=0`, including secrets CRUD
and logs parsing, and cleanup left no generated Docker resources behind.

## 9. Verdict

**PRODUCTION-READY for a single gateway per Docker daemon** (the documented,
primary target), with the residual items in §7 and the one pre-exposure security
action below. This verdict is asserted **only after** the external-review P1s
(RT-235/236/237/240/241) were fixed and re-verified — including the previously-missing
real-router `-race` coverage. Every mandatory OpenFaaS operation is proven with
the pinned faas-cli; all four original HIGH risks and all HIGH objections are
closed with tests; the full suite (11 packages, incl. `pkg/router`) is race-clean
in-container.

- **Shared-daemon multi-instance** (multiple gateways on one daemon) is **NOT
  fully supported** — see RT-239 in §7. Use one gateway per daemon, or strict
  ownership + disjoint function names.
- **Pre-exposure security action** (not ship-blocking on a trusted network): gate
  or remove `GET /system/secrets/{name}`.
- **Recommended before release tagging:** add CI `-race` on Linux.

Given that an external review found real P1 defects two internal rounds missed,
the honest posture is: the implementation is now materially sound and every known
blocker is fixed and tested, but confidence should come from the green
`-race`/faas-cli evidence and one more independent pass over the latest changes —
not from the absence of findings.
