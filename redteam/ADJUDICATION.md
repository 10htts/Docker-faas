# Adjudication record — 2026-07-21 hardening round

Adjudicator rulings on every objection in `OBJECTIONS.md`. Each ruling cites the
fix location and the test that proves it. Terminal states: **CLOSED** (fixed +
proven), **REJECTED** (not a defect), **DEFERRED** (accepted residual risk with
rationale). The adjudicator treated a fix as unproven until a test failed
against the pre-fix code and passed against the post-fix code.

## HIGH objections

### RT-201 — unauthenticated activity-lease mutation → **CLOSED**
Fix: `pkg/gateway/scale_handlers.go` `HandleActivityLease` now verifies the
request HMAC (`faascontract.VerifyActivityLeaseRequest`) with an isolated,
file-loadable secret (`FAAS_ACTIVITY_LEASE_SECRET[_FILE]`, `pkg/config/config.go`
`getSecretEnv`) **before** `leases.Apply`, signs the response, and returns
401/409/413/503 per a defined taxonomy. Startup fails closed when idle
scale-to-zero is enabled without the secret (`cmd/gateway/main.go`).
Evidence: `TestActivityLeaseRejects{UnsignedRequest,WrongSecret,TamperedField,
WhenSecretUnconfigured}`, `TestActivityLeaseAcceptsSignedAndResponseVerifies`,
`TestActivityLeaseGrantsNoDockerControl`; critic-sec verified property (1) "no
mutation before authentication" by grep-confirming `leases.Apply` has no other
production caller. Container evidence: unconfigured endpoint returns 503
(fail-closed) in the live smoke run.

### RT-202 — cold-start ready-state re-election + sleep tests → **CLOSED**
Fix: `AcquireColdStart` honors `ready` (`pkg/scaletozero/gate.go`) so a late
caller after a successful cold start never elects a second leader; the two
sleep-based gate tests were rewritten with channel barriers.
Evidence: `TestLateColdStartAfterReadyDoesNotReElect`,
`TestColdStartLateArrivalsNeverReElect` (200 iterations),
`TestConcurrentColdStartElectsSingleLeader` (rewritten, sleep-free),
`TestHundredDeterministicColdStartCycles`. critic-gate verified properties (1),
(3), (9). Remaining scope (ctx cancellation, failed-start retry, crash demotion)
tracked and closed under RT-213.

### RT-203 — restart safety unproven → **CLOSED**
Fix: `IdleReconciler.ReconcileOnce` seeds the idle clock on the first
post-restart observation of a running replica (`gates.MarkActivity` before the
snapshot), folds honored lease activity into the gate, and `LeaseRegistry.View`
now preserves the last-activity fact across expiry while still dropping counts
(SZ-08 fail-safe intact). Guarantees: (a) a surviving function is never
reclaimed before one full idle window elapses post-restart; (b) a genuinely idle
function still converges to reclaim.
Evidence: `TestRestartWithDurableWorkLeaseRenewalPreventsReclaim`,
`TestRestartIdleConvergenceSeedsWindowThenReclaims`,
`TestRestartMidReclaimConvergesIdempotently`,
`TestRestartReclaimFailureUnlocksGateAndRetries`,
`TestRestartLeaseGenerationSemantics` (all fake-clock, sleep-free).

### RT-205 — OpenFaaS conformance gaps → **CLOSED**
Fix (all in `pkg/gateway`, `pkg/types`, `pkg/provider/logs_stream.go`): new
`GET /system/function/{name}` (pinned FunctionStatus shape), `GET
/system/namespaces`, `GET /system/logs` now the pinned NDJSON log-message stream,
async `X-Callback-Url` honored, `/system/info` emits the pinned
`provider.provider` key with legacy `name` additive. Routes wired in
`cmd/gateway/main.go`.
Evidence: `TestHandleGetFunction_*`, `TestHandleListNamespaces_*`,
`TestHandleGetLogs_*`, `TestHandleInvokeFunctionAsync_{PostsCallback,
RejectsInvalidCallbackURL}`, `TestHandleSystemInfo_PinnedProviderInfoShape`.
Container evidence: faas-cli 0.18.0 `describe`, `logs` (NDJSON verified),
`namespaces`, async 202+X-Call-Id, `info` provider key — all pass live.

### RT-206 — official scale labels ignored → **CLOSED**
Fix: `pkg/gateway/idle_controller.go` honors `com.openfaas.scale.zero`,
`zero-duration`, `min`, `max`; deploy clamps initial replicas to `scale.min`
within config max; scale clamps to `scale.max`. Precedence: custom
`com.docker-faas.*` > official `com.openfaas.*` > config default (documented in
code + matrix).
Evidence: `TestPolicyFor_OfficialOpenFaaSLabels`, `TestScaleBoundsFromLabels`,
`TestHandleDeployFunction_HonorsOpenFaaSScaleMinLabel`,
`TestHandleScaleFunction_ClampsToMaxLabel` + custom-wins variants. Container: a
`replicas=9` scale request clamped live to the `scale.max=3` label.

### RT-213 — gate has no crash-demotion path → **CLOSED**
Fix: `GateRegistry.ReportZeroObserved` (generation-fenced) demotes stale
readiness; `AcquireColdStartCtx` honors ctx while parked behind a reclaim;
`ensureReadyFromZero` (`pkg/gateway/coldstart.go`) is a bounded acquire/verify/
retry loop covering follower failed-start retry and crash recovery; leader
`Complete` is now panic-safe (`runLeaderColdStart`, RT-215).
Evidence: `TestReportZeroObservedDemotesOnlyCurrentGeneration`,
`TestReportZeroObservedRejectedDuringScaleOps`,
`TestAcquireColdStartCtxCancelledDuringReclaim`,
`TestFailedStartFollowerFanOutAndReElection`,
`TestEnsureReadyFromZero{LeaderFailureRetries,CrashStaleReadyRecovers,
FollowerHonorsCancellation}`.

### RT-223 — orphan cleanup destroys other instances' containers → **CLOSED**
Severity HIGH, **found live** during the container smoke test (the smoke gateway
reclaimed two function containers of a separate docker-faas instance). Fix: every
function container is stamped with `LabelGateway = FUNCTIONS_NETWORK` (stable
per-deployment identity, survives restart), and the orphan scan
(`ObservedFunctions` / `listFunctionTypeContainers`, `pkg/provider`) is scoped to
that identity at both the Docker-filter and post-filter layers. Containers owned
by another gateway — or carrying no ownership label — are ignored.
Evidence: `TestObservedFunctionsExcludesForeignGatewayContainers` (faithful fake
daemon with a foreign + legacy container). Container evidence: before the fix the
smoke gateway reclaimed 2 foreign containers on boot; after the fix, 0 on the same
shared daemon, while still managing its own labeled functions. Incident and impact
disclosed in `OBJECTIONS.md`; impact recoverable (reclaimed containers self-heal
via scale-from-zero; the other gateway process was unaffected).

### RT-240 — stale-name cleanup bypasses ownership → **CLOSED**
Fix: `DockerProvider.canManageContainer` is now the single ownership predicate
for both per-function listing and `removeStaleContainerByName`. Explicit foreign
owners are always rejected; unlabeled containers are accepted only in lenient
single-gateway mode. Strict mode now fails safely on a conflicting foreign or
ambiguous daemon-global name instead of force-removing it.
Evidence: `TestRemoveStaleContainerByNameRespectsOwnership` exercises all four
owned/foreign/legacy and strict/lenient combinations through the real Docker SDK
client against the fake daemon. The full Linux race suite remains clean. A
post-fix isolated real-daemon smoke also confirmed that a stopped foreign
same-name container survives the rejected deployment unchanged.

### RT-241 - pinned faas-cli secret delete was incompatible -> **CLOSED**
Fix: `HandleDeleteSecret` now accepts the pinned `types.Secret` JSON body sent
by faas-cli 0.18.0, validates the optional namespace, keeps the legacy `?name=`
fallback, and returns 200. The 200 status is deliberate: the pinned OpenAPI
documents 204, but faas-cli 0.18.0 treats 204 as an unexpected status for this
operation and accepts only 200/202.
Evidence: `TestHandleDeleteSecret_AcceptsFaasCLIJSONBody`,
`TestHandleDeleteSecret_KeepsLegacyQueryName`,
`TestHandleDeleteSecret_RejectsUnknownNamespace`, and
`TestHandleDeleteSecret_RequiresName`. Live evidence:
`SMOKE_ID=codexfix-20260721 bash scripts/smoke-openfaas-cli.sh` passed
`faas-cli secret create/list/update/remove`.

## MED objections

### RT-204 — extension endpoints undocumented → **CLOSED**
`docs/SCALE_EXTENSIONS_CONTRACT.md` documents the versioned contract, security
model, status codes, semantics, upgrade path, and metrics for
`/system/scale/capabilities` and `/system/scale/activity-lease`.

### RT-207 — no pinned upstream versions → **CLOSED**
`redteam/CONFORMANCE_MATRIX.md` pins faas-provider v0.25.12, faas 0.27.13,
faas-cli 0.18.0, nats-queue-worker 0.14.2, with URLs and fetch date.

### RT-208 — `-race` unavailable → **CLOSED (mitigated)**
The Windows host lacks a C toolchain, so cgo/race is unavailable there. Mitigated
by running the full suite with `-race` inside a pinned `golang:1.25` Linux
container: build, vet, and all 11 tested packages pass race-clean, plus a 10× race
stress run of the storm/cold-start tests. Race-cleanliness is therefore proven
(in-container), not merely asserted. See `VERIFICATION.md`.

### RT-210 — replay/skew absent → **CLOSED** (subsumed by RT-214).

### RT-211 — storm determinism → **CLOSED**
`TestStormInvocationRacingReclaimDeterministic` drives 8 channel-sequenced
workers over 120 scripted iterations asserting `InFlight==0` inside
`ReclaimToZero`; both the CAS window and pre-snapshot window are pinned.

### RT-214 — lease body/skew/replay/rotation → **CLOSED**
Fix: `pkg/gateway/lease_auth.go` + `HandleActivityLease` add a 64 KiB body limit
(413 pre-auth), issued-at skew window (401), single-use nonce replay cache,
previous-secret rotation overlap, and response nonce binding.
Evidence: `TestActivityLeaseRejects{StaleTimestamp,FutureTimestamp,
MissingIssuedAtWhenSkewEnforced,ReplayedNonce,OversizedBodyBeforeAuth}`,
`TestActivityLeaseRotationAcceptsPreviousSignsWithActive`,
`TestNonceCacheBoundsAndExpiry`, `TestActivityLeaseInvalidRequestDoesNotBurnNonce`.

### RT-217 — replay-cache cap weaponizable → **CLOSED**
critic-sec objection: a hardcoded 8192 reject-when-full cap could make the
legitimate control plane's renewals 503 at scale, cascading into wrongful
reclaim. Fix: cap raised to 65536, operator-tunable
(`FAAS_ACTIVITY_LEASE_REPLAY_CAP`), and at capacity the **oldest** entry is
evicted and the request accepted (observability metric `replay_cache_evicted`)
rather than rejected. Evidence: `TestNonceCacheBoundsAndExpiry` (evict-oldest +
prune-on-expiry).

### RT-242 - logs smoke used an invalid faas-cli flag -> **CLOSED**
Fix: `scripts/smoke-openfaas-cli.sh` now uses the actual faas-cli 0.18.0 logs
flags: `faas-cli logs <name> --tail=false --lines 20`. The removed
`--follow=false` flag never existed at this pin.
Evidence: Git Bash `bash -n` passes for the harness, and the fresh isolated live
smoke (`SMOKE_ID=codexfix-20260721`) parsed logs successfully through the real
faas-cli against a containerized gateway.

## MED/LOW hardening (critic-driven) — **CLOSED**

- **RT-215** (leader panic wedges gate) — `runLeaderColdStart` defers a
  recover→Complete so `g.cold` is always cleared. Evidence: the crash-recovery
  and retry tests exercise the Complete-always path; critic-gate objection (1).
- **RT-216** (delete leaks gate/lease, stale ready survives redeploy) —
  `HandleDeleteFunction` now `Forget`s gate + lease state. Evidence: container
  redeploy-after-delete works live.
- **RT-218** (Complete double-close / unpaired FinishReclaim) — `coldStartOp.
  completed` guard + `functionGate.reclaiming` flag make both no-ops.
- **RT-219** (async goroutine panic kills process; cold start tied to request
  ctx; 500 ms first-check floor) — recover in the async goroutine, detached
  bounded context for async cold start, immediate first health check.
- **RT-220** (nonce burned before Validate; TTL edge; function-name charset;
  lease-map growth) — replay check moved after Validate, TTL = 2·skew+1s,
  `validateFunctionName` on the lease path, opportunistic expired-lease prune in
  `LeaseRegistry.Apply`. Evidence: `TestActivityLeaseInvalidRequestDoesNotBurnNonce`,
  `TestActivityLeaseRejectsMalformedFunctionName`.
- **RT-221** (future LastActivityAt pins function) — `Apply` anchors lastActive
  at `now` (renewal = current-activity assertion), never a caller-supplied future
  time.
- **RT-222** (listFunctionContainers missing type term) — the type=function term
  was added to the function-name filter.

## LOW / deferred

### RT-209 — `GET /system/secrets/{name}` returns the secret VALUE → **DEFERRED**
This endpoint is a docker-faas extension; OpenFaaS never returns secret values.
Recorded in the conformance matrix as a guarded extension recommended for removal
or explicit gating. Deferred (not a mandatory-operation contract break; behind
gateway auth) with a documented residual risk and a follow-up recommendation. It
does not block the OpenFaaS-contract verdict because it is additive and not part
of the standard API surface.

### RT-212 — provider scale bounds → **CLOSED** (folded into RT-206 clamp).

## Pre-existing hygiene (out of scope, recorded not fixed)
- 13 untouched source files carry CRLF line endings (Windows checkout) that
  `gofmt -l` flags; all files changed this session are gofmt-clean.
- `pkg/faascontract/contract.go` has a cosmetic gofmt doc-comment delta left
  UNCHANGED deliberately: the file is declared byte-identical across both repos,
  and reformatting only this copy would break that invariant.
