# Objection ledger

Status values: `open` → `in-fix` → `fixed-pending-adjudication` → `closed` /
`rejected` / `deferred`. Only the adjudicator moves an objection to a terminal
state. Critics may reopen a closure that lacks evidence.

| ID | Sev | Title | Owner | Status |
|----|-----|-------|-------|--------|
| RT-201 | HIGH | /system/scale/activity-lease accepts unauthenticated requests and mutates lease state before any check; responses unsigned | cv-session + orchestrator | CLOSED |

| RT-202 | HIGH | AcquireColdStart ignores ready state: late-caller re-election + sleep-based tests | cv-session + orchestrator | CLOSED |

| RT-203 | HIGH | Scale-to-zero restart safety unproven: leases and gate state are in-memory; after a gateway restart, durable work is invisible until the control plane re-leases, and post-restart reclaim behaviour has no test | builder-scale | CLOSED |

| RT-204 | MED | /system/scale/capabilities and /system/scale/activity-lease have no versioned public contract doc (security model, errors, upgrade path) | builder-docs | CLOSED |

| RT-205 | HIGH | OpenFaaS conformance gaps: no GET /system/function/{name} (faas-cli describe breaks), no GET /system/namespaces, /system/logs is text/plain not the OpenFaaS NDJSON log-message stream (faas-cli logs breaks), async invoke ignores X-Callback-Url, /system/info provider field is `name` where faas-provider emits `provider` | builder-conf | CLOSED |

| RT-206 | HIGH | Official OpenFaaS scale labels (com.openfaas.scale.min/max/zero/zero-duration) are not honored anywhere; only custom com.docker-faas.scale.* labels are parsed; deploy always starts 1 replica ignoring scale.min | builder-conf | CLOSED |

| RT-207 | MED | No pinned OpenFaaS spec / faas-provider types / faas-cli versions recorded in-repo; conformance claims are unanchored | builder-conf | CLOSED |

| RT-208 | MED | `-race` unavailable in this build environment (windows/amd64, no gcc ⇒ cgo race runtime unavailable); race-cleanliness cannot be claimed without alternate stress evidence | orchestrator | CLOSED |

| RT-209 | LOW | GET /system/secrets/{name} returns the secret VALUE — not part of the OpenFaaS API (OpenFaaS never returns secret values); must be documented as a guarded extension or removed | builder-conf | DEFERRED (documented) |

| RT-210 | MED | Replay protection absent on activity-lease (nonce is carried but never checked); timestamp skew not enforced | orchestrator | CLOSED |

| RT-211 | MED | Reconciler cold-start storm behaviour (N invocations racing one reclaim across restart boundaries) has no deterministic test at the reconciler level | builder-scale | CLOSED |

| RT-212 | LOW | scaleReq handling: OpenFaaS scale beyond com.openfaas.scale.max or below scale.min is not bounded by the provider | builder-conf | CLOSED |

| RT-213 | HIGH | Gate ready-state crash-demotion (ReportZeroObserved + retry loop + ctx-aware acquire) | orchestrator | CLOSED |

| RT-214 | MED | Activity-lease body limit, skew, replay cache, rotation overlap | orchestrator | CLOSED |

| RT-215 | MED | Leader panic in cold start skips Complete → gate wedged permanently for that function (every later invocation parks forever; reclaim+demotion both refuse while cold!=nil) | orchestrator | CLOSED |

| RT-216 | MED | HandleDeleteFunction never Forgets gate/lease state → per-function leak + stale ready survives delete→redeploy | orchestrator | CLOSED |

| RT-217 | MED | Replay-cache cap (8192 global, hardcoded, reject-when-full) can be exhausted by the LEGITIMATE control plane at ~700+ leased functions → renewal 503s → lease expiry → wrongful reclaim cascade | orchestrator | CLOSED |

| RT-218 | LOW | Cold-start API latent hazards: Complete double-close panic; unpaired FinishReclaim can hijack a cold-start channel | orchestrator | CLOSED |

| RT-219 | LOW | Async invoke: background goroutine has no recover (panic kills process); cold start tied to request ctx (client disconnect aborts fire-and-forget); waitForFunctionReady has 500ms first-check floor | orchestrator | CLOSED |

| RT-220 | LOW | Lease-path hygiene: nonce burned before Validate; TTL/skew closing-edge nanosecond; function name not charset-validated on lease path (metric cardinality); leases map pruned only by reconciler | orchestrator | CLOSED |

| RT-221 | LOW | Lease semantics doc gap: Apply stamps lastActive=max(req.LastActivityAt, now) — every renewal asserts activity by design (lease = activity assertion; control plane must STOP renewing idle functions). Undocumented; also declared far-future LastActivityAt honored | orchestrator | CLOSED |

| RT-222 | LOW | listFunctionContainers filters only the function-name label (missing type=function term); isNetworkNotFoundErr misses SDK objectNotFoundError form | orchestrator | CLOSED |

| RT-223 | HIGH | Orphan cleanup reclaims ALL `type=function` containers on the daemon regardless of owning gateway — an idle reconciler destroys other docker-faas instances' function containers on a shared Docker daemon. FOUND LIVE during the container smoke test: the isolated smoke gateway reclaimed two function containers (`import-bundle`, `gateprobesquare`) belonging to a separate docker-faas instance | orchestrator | CLOSED |

| RT-224 | HIGH | Per-function container listing (`listFunctionContainers`, feeding ReclaimToZero / ObservedReplicas / request routing / scale planning / logs) was NOT ownership-scoped — the RT-223 fix only covered the orphan sweep. Two instances running a same-named function on one daemon would still cross-count, cross-route, and cross-reclaim. Found during validation re-review | orchestrator | CLOSED |
| RT-225 | LOW | Operator footgun: `ownerID()` == FUNCTIONS_NETWORK, so CHANGING `FUNCTIONS_NETWORK` on an existing deployment makes the gateway treat all its pre-change containers as foreign (orphan sweep skips them → leak; they are never reclaimed by the new identity). Must be documented as a non-drop-in config change | orchestrator | DEFERRED (documented) |

| RT-226 | MED | My RT-220 opportunistic expired-lease prune in `LeaseRegistry.Apply` could delete ANOTHER function's expired lease before the reconciler folded its last-activity anchor, allowing that function to be reclaimed up to one reconcile interval before its true idle window elapsed. Found by validation critic | orchestrator | CLOSED |
| RT-227 | MED | Idle reconciler had NO panic recovery — a panic in a Docker SDK call (ReclaimToZero/ObservedReplicas) crashed the WHOLE gateway process, and a panic between TryBeginReclaim and FinishReclaim would wedge the gate. Asymmetric with the already-hardened cold-start (RT-215) and async (RT-219) paths. Found by validation critic | orchestrator | CLOSED |
| RT-228 | MED | Log streaming leaked a goroutine + io.Pipe on every client disconnect of an actively-logging function: the stdcopy writer parked in `pw.Write` was never unblocked (closing the Docker stream does not unblock a pending pipe WRITE). Found by validation critic | orchestrator | CLOSED |
| RT-229 | LOW | `GET /system/functions` silently DROPPED a function from the listing when its container query errored transiently (faas-cli list would show fewer functions than exist, no error). Found by validation critic | orchestrator | CLOSED |
| RT-230 | LOW | Blind SSRF: async `X-Callback-Url` validated scheme only, no block on loopback/RFC1918/metadata targets, despite the repo shipping those helpers for git URLs. Defense-in-depth (upstream OpenFaaS also allows any callback), not a conformance break. Found by validation critic | orchestrator | CLOSED (opt-in) |
| RT-231 | LOW | Doc drift I introduced with RT-217: `docs/SCALE_EXTENSIONS_CONTRACT.md` still stated replay cache 8192 / `replay_cache_full` / 503-exhausted, but code does 65536 / evict-oldest-accept / `replay_cache_evicted`. Found by validation critic | orchestrator | CLOSED |
| RT-232 | LOW | Decider did not guard `IdleDuration<=0` (would reclaim at idleFor==0); floor lived only in the policy source. Not reachable in prod (idle_controller floors to 5m), defense-in-depth. Found by validation critic | orchestrator | CLOSED |
| RT-233 | LOW | Orphan cleanup cannot reclaim LEGACY (pre-ownership-label) containers of an UNDECLARED function, though per-function reclaim of a DECLARED function still cleans them. Deliberately kept strict (never touch unlabeled — could be foreign) as the safer choice; leak-only, transient to the upgrade window | orchestrator | DEFERRED (documented) |

| RT-236 | HIGH | Router `selectContainer` did a get-or-create on the `roundRobin` map with NO synchronization (only the counter was atomic). Two concurrent first-requests = `fatal error: concurrent map writes` → whole-process crash. Exactly the cold-start storm case. Found by external review (codex) | orchestrator | CLOSED |
| RT-237 | HIGH | Async invoke did `io.ReadAll(resp.Body)` UNCONDITIONALLY (even with no X-Callback-Url) and unbounded, so a large/streaming function response × concurrent invocations could exhaust gateway memory. Found by external review (codex) | orchestrator | CLOSED |
| RT-235 | HIGH | Reserved container labels (gateway/function/type/replica/network) were applied BEFORE caller-supplied labels, so a deployment could OVERRIDE them — e.g. forge `com.docker-faas.gateway` to appear owned by another gateway and evade/hijack reclaim (defeats the RT-223/224 ownership model). Found by external review (codex) | orchestrator | CLOSED |
| RT-234 | HIGH | Legacy (unlabeled) same-name containers are treated as local on EVERY per-function path, so on a shared daemon one gateway could reclaim/scale another gateway's pre-upgrade container. The RT-233 tradeoff, escalated by external review (codex) | orchestrator | CLOSED (opt-in strict mode) |
| RT-238 | MED | Opt-in async SSRF guard validated only the INITIAL callback host; the default client followed redirects, so a permitted public callback could 30x-redirect to a metadata/private target. Found by external review (codex) | orchestrator | CLOSED |
| RT-239 | MED | Container names are globally `<function>-<replica>`, so two gateways CANNOT run a same-named function on one daemon (Docker name collision) — true shared-daemon same-name coexistence is not achievable without namespacing container names. Found by external review (codex) | orchestrator | DEFERRED (documented limitation) |
| RT-240 | HIGH | Strict ownership filtered list/reclaim paths, but `createContainer` still called `removeStaleContainerByName`, which force-removed a stopped container occupying the global name without checking its gateway label. A foreign or unlabeled same-name container could therefore still be destroyed before create. Found by independent follow-up review (codex) | orchestrator | CLOSED |
| RT-241 | HIGH | `faas-cli secret remove` was not actually compatible: the pinned CLI sends a JSON `Secret` body on DELETE `/system/secrets`, while the gateway only read `?name=`, then returned 204 even though faas-cli 0.18.0 accepts only 200/202. The hardened smoke exposed the prior false-green secrets claim. | orchestrator | CLOSED |
| RT-242 | MED | The committed smoke harness used a non-existent faas-cli 0.18.0 logs flag (`--follow=false`), so its logs check could not prove the pinned CLI stream contract. The gateway logs implementation was unit-covered, but the harness evidence was invalid until corrected. | orchestrator | CLOSED |

### External-review round detail (RT-234..239) — codex
An independent external review (codex) re-examined the tree after the validation
round and surfaced SIX findings, THREE of them HIGH and genuinely
verdict-blocking that the prior rounds MISSED: a fatal router map-write race
(RT-236), an unbounded async response read (RT-237), and reserved-label override
(RT-235). Root cause of the miss: the gateway/scale critics used FAKE routers and
never exercised the real `Router` under `-race`, and the reviews focused on the
scale-to-zero subsystem rather than the invoke/router hot path. This is a real
process gap — the earlier PRODUCTION-READY verdict was NOT supported while these
stood. Fixes + tests:
- RT-236: `Router.counterFor` guards the map with a mutex
  (`TestSelectContainerConcurrentFirstRequestsNoRace`, proven under `-race`).
- RT-237: async reads the body only when a callback exists, bounded by
  `io.LimitReader` (8 MiB); otherwise drains to `io.Discard`.
- RT-235: `functionContainerLabels` applies reserved keys AFTER caller labels so
  reserved always win (`TestFunctionContainerLabelsReservedWin`).
- RT-234: opt-in `FAAS_STRICT_CONTAINER_OWNERSHIP` makes per-function selection
  ignore unlabeled containers on a shared daemon
  (`TestStrictOwnershipExcludesUnlabeledLegacy`); default preserves single-gateway
  upgrades.
- RT-238: `asyncCallbackHTTPClient` re-validates every redirect hop when the guard
  is on (`TestAsyncCallbackClientRevalidatesRedirects`).
- RT-239: documented as a shared-daemon limitation (see verdict); the supported
  shared-daemon config is strict ownership + disjoint function names per gateway.
Also confirmed by codex: build, vet, full Linux `-race` suite pass; the working
tree is uncommitted (32 modified + 31 untracked — expected, this session performs
NO Git operations per its directive) and `pkg/faascontract/contract.go` fails
`gofmt -l` (deliberately left to preserve cross-repo byte-identity; documented).

### Independent follow-up correction (RT-240) — codex
The requested independent pass over RT-234..239 found one additional destructive
ownership bypass. `removeStaleContainerByName` inspected a daemon-global name and
removed any stale container before `ContainerCreate`, bypassing the owner filter
used by `listFunctionContainers`. The provider now uses one `canManageContainer`
predicate in both paths: an explicit owner must always match; unlabeled legacy
containers are accepted only in lenient mode. Evidence:
`TestRemoveStaleContainerByNameRespectsOwnership` covers owned+strict,
foreign+lenient, legacy+strict, and legacy+lenient through the real Docker SDK
against the fake daemon. Compose now forwards `FUNCTIONS_NETWORK`,
`FAAS_STRICT_CONTAINER_OWNERSHIP`, and `FAAS_ASYNC_CALLBACK_BLOCK_INTERNAL` from
`.env`. Strict-mode upgrade instructions now require relabeling by redeploying in
lenient mode before strict mode is enabled.

### Validation of the codex round (orchestrator, 2026-07-21)
Independently re-verified the RT-240 fix and audited EVERY container
stop/remove call site for ownership safety. Confirmed safe: `RemoveFunction`,
`ScaleFunction`, and `ReclaimToZero` all act only on the ownership-filtered
`listFunctionContainers` result, and `removeStaleContainerByName` now gates on
`canManageContainer`. One residual latent footgun found and removed: the dead,
unexported `removeContainer(name)` method force-removed by name with NO ownership
check (zero callers, linter-flagged unused) — deleted so it can never be wired up
into a bypass. Build, vet, full `-race` suite (11 packages incl. router), and
targeted ownership/router/async stability ×10 all pass on the exact current tree.

The same pass added the missing RT-237 regression evidence:
`TestReadAsyncCallbackBodyBoundsAndSignalsTruncation` and
`TestHandleInvokeFunctionAsyncWithoutCallbackDrainsResponse`. Truncated callback
bodies are explicitly marked with `X-Docker-Faas-Callback-Truncated: true`, and
the function's original full `Content-Length`/`Content-Range` is not forwarded.

### Live-smoke correction (RT-241..242) — codex
The hardened isolated smoke harness was run against the exact current tree and
found two remaining compatibility/evidence failures: `faas-cli secret remove`
returned 400 because DELETE only read `?name=`, and the logs probe used a flag
that faas-cli 0.18.0 does not provide. The gateway now accepts the pinned JSON
`Secret` body (including namespace validation) and returns 200 for delete,
because faas-cli 0.18.0 rejects the OpenAPI-documented 204. Regression evidence:
`TestHandleDeleteSecret_AcceptsFaasCLIJSONBody`,
`TestHandleDeleteSecret_KeepsLegacyQueryName`,
`TestHandleDeleteSecret_RejectsUnknownNamespace`, and
`TestHandleDeleteSecret_RequiresName`. The harness now uses
`faas-cli logs <name> --tail=false --lines 20`. A fresh isolated real-daemon
run (`SMOKE_ID=codexfix-20260721`) passed all 25 checks, including secrets CRUD,
logs parsing, namespaces, invoke, async, scale, raw contract checks, and remove;
all generated Docker resources were verified absent after cleanup.

### Validation round detail (RT-224..233)
Two independent read-only validation critics re-examined the OpenFaaS conformance
code and the scale-to-zero safety code against the FINAL integrated tree (after
the initial round closed). They confirmed the load-bearing safety properties hold
(single-leader cold start, reclaim CAS, restart seeding, lease anchoring, warm-min
clamp, ownership isolation) and surfaced the RT-224..233 gaps above — all real,
none in the four original HIGH risks. Fixes + tests:
- RT-226: `HandleActivityLease` folds activity into the gate on every accepted
  lease (`TestAcceptedLeaseFoldsActivityIntoGate`, `TestStaleGenerationLeaseDoesNotFoldActivity`).
- RT-227: `IdleReconciler.safeReclaim` (deferred FinishReclaim + recover) and a
  top-level recover in `ReconcileOnce` (`TestReconcilePanicIsContainedAndGateReleased`,
  `TestReconcilePanicPassIsRetryable`).
- RT-228: `defer pr.Close()` in `forwardContainerLogs` (`TestForwardContainerLogsReturnsOnCancel`,
  `TestForwardContainerLogsDemuxesAndParses` — first real-stdcopy coverage).
- RT-229: `HandleListFunctions` reports availableReplicas=0 instead of dropping.
- RT-230: opt-in `FAAS_ASYNC_CALLBACK_BLOCK_INTERNAL` guard reusing isBlockedIP/
  isBlockedHostname (`TestBlockInternalCallbackTarget`); default off = OpenFaaS-compatible.
- RT-231: doc corrected to 65536 / evict-oldest / `replay_cache_evicted`.
- RT-232: `Decide` holds when `IdleDuration<=0` (`TestDecideNonPositiveIdleDurationHolds`).

### RT-224 detail
Fix: `pkg/provider/docker_provider.go` `listFunctionContainers` now post-filters
the daemon result, dropping any container whose `LabelGateway` is present AND
differs from `ownerID()`. Unlabeled (legacy, pre-label) containers are retained
as this gateway's own for backward compatibility; only an explicitly-foreign
label excludes. This closes the cross-instance vector on EVERY per-function path,
not just orphan cleanup. Evidence: `TestPerFunctionListExcludesForeignButKeepsLegacy`
(same-named `shared` function across mine/legacy/foreign owners — ObservedReplicas
counts 2, ReclaimToZero removes mine+legacy and leaves foreign running).

### Smoke-test side effect (2026-07-21, disclosed)
While running the containerized faas-cli conformance smoke test with idle
scale-to-zero ENABLED, the smoke gateway's startup orphan-cleanup pass reclaimed
two function containers (`import-bundle`, `gateprobesquare`) that belonged to a
DIFFERENT docker-faas instance sharing the same Docker daemon. Impact assessed as
recoverable: docker-faas persists function metadata in its own store, so a
reclaimed function container is equivalent to a scale-to-zero and is recreated on
the next invocation (scale-from-zero); the other gateway process itself was
unaffected and remained healthy. The smoke gateway was stopped immediately after
the two log lines were observed and the stack was fully torn down. This exposed
the real RT-223 defect, now fixed (gateway-ownership label + ownership-scoped
orphan scan). The smoke test's own compose was already isolated (unique project,
ports, network, volumes); the daemon-wide orphan scan was the escape vector, which
is exactly what RT-223 closes.

### Cross-session note (2026-07-21)
A concurrent hardening session (objection prefix CV-xx, control-plane side) is
active in this working tree. Landed by it so far: CV-06 partial fix for RT-201
(dedicated isolated secret FAAS_ACTIVITY_LEASE_SECRET[_FILE], fail-closed
startup, HMAC verify-before-mutation, signed responses, 401/409/503 taxonomy)
and CV-07 fix for the RT-202 late-caller re-election (ready-state honored in
AcquireColdStart + deterministic 200-iteration test). RT-214 and RT-213 track
what those passes did NOT cover. RT-201/RT-202 narrow accordingly: RT-201's
remaining scope == RT-214; RT-202's remaining scope == RT-213 + ctx-aware
reclaim-wait + follower failed-start retry + removal of the two remaining
sleep-based gate tests.

## Detail

### RT-201 (HIGH) — unauthenticated activity-lease mutation
- Evidence: `pkg/gateway/scale_handlers.go:59-130` — `HandleActivityLease` decodes
  the body, calls `req.Validate()` (which explicitly does NOT verify signatures,
  see `pkg/faascontract/contract.go:111-127`), then calls
  `g.scale.leases.Apply(req)` — state mutation with zero authentication beyond
  the shared gateway basic-auth. `pkg/faascontract` ships full HMAC
  sign/verify (`DecodeActivityLeaseRequest`, `EncodeActivityLeaseResponse`)
  that the handler never invokes. No body limit, no skew check, no replay
  protection, no dedicated secret, unsigned response.
- Required: dedicated file-loaded secret (no reuse of AUTH_PASSWORD, no
  fallback), reject-before-mutation, constant-time verify, key-id/rotation
  handling, timestamp skew, nonce replay cache, body limit, signed responses,
  401/400/409/413/503 error contract, metrics.

### RT-202 (HIGH) — cold-start state machine
- Evidence: `pkg/scaletozero/gate.go:168-199` — after `Complete(nil)` sets
  `ready=true`, the next `AcquireColdStart` finds `cold==nil && scaleDone==nil`
  and elects a NEW leader (bumping generation) even though the function is
  ready. `pkg/scaletozero/gate_test.go:156` uses `time.Sleep(20ms)`.
- Required: deterministic generation-fenced state machine (Zero / Starting /
  Ready / Reclaiming), ready honored via generation-checked zero-observation,
  single leader, waiter fan-out, ctx cancellation for waiters AND for
  reclaim-wait, failed-start retry (followers can re-elect), stale-generation
  rejection, sleep-free tests, 100 deterministic cold-start iterations.

### RT-203 (HIGH) — restart safety
- Evidence: `pkg/scaletozero/leases.go` (in-memory only),
  `cmd/gateway/main.go:95-117` (fresh registries each boot). After restart,
  `LastActivity` is zero ⇒ decider holds (safe), but nothing proves: (a) the
  hold actually protects functions with running durable work through the
  control-plane re-lease window; (b) genuinely idle functions converge to
  reclaim after restart rather than being held forever; (c) restart during an
  in-progress reclaim/drain converges idempotently.
- Required: restart-simulation tests (fresh registries over persisted store +
  fake controller), startup activity seeding decision (recorded + adjudicated),
  reclaim-only-labeled-resources proof, idempotent reconcile proof.

### RT-205 / RT-206 / RT-207 / RT-209 / RT-212 — OpenFaaS conformance
- Evidence: `cmd/gateway/main.go:179-215` route table vs OpenFaaS gateway API;
  `pkg/gateway/handlers.go:540-568` logs handler (text/plain);
  `pkg/gateway/async_handlers.go` (no X-Callback-Url);
  `pkg/gateway/idle_controller.go:22-31` (custom labels only);
  `pkg/gateway/handlers.go:76-87` + `pkg/types/types.go` SystemInfo
  (`provider.name` vs faas-provider `provider.provider`);
  `pkg/gateway/handlers.go:213-215` (deploy pins replicas=1).
- Required: pinned spec + types + cli versions; full endpoint matrix with
  evidence; mandatory endpoints implemented and proven with the pinned
  faas-cli against a live gateway; official labels honored with custom labels
  kept as additive overrides.

### RT-208 (MED) — race detector unavailability
- Evidence: `go version` ⇒ go1.25.1 windows/amd64; `gcc` absent; Go race
  runtime requires cgo on windows/amd64. Fallback: high-iteration concurrency
  stress (`-count=`, parallel) on the lock-based packages + `-race` documented
  as CI-required on linux. Recorded; do NOT claim race-clean.
