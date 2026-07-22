# Verification log — 2026-07-21

Exact commands and results for the hardening round. Host is Windows
(go1.25.1 windows/amd64, no C toolchain → no host cgo/race). The race detector
and cgo store tests run in a pinned `golang:1.25` Linux container against the
same working tree (bind-mounted).

## Format / build / vet (host)

```
$ gofmt -l pkg/gateway pkg/scaletozero pkg/provider pkg/store pkg/config pkg/types cmd/gateway
(empty — all session-changed Go files gofmt-clean)

$ go build ./...
(ok)

$ go vet ./...
(clean)
```

Note: `gofmt -l pkg cmd` flags 13 untouched files with pre-existing CRLF line
endings plus `pkg/faascontract/contract.go` (cosmetic doc-comment delta left
unchanged to preserve cross-repo byte-identity). None were modified this session.

## Unit / integration tests (host, CGO off)

```
$ go test ./pkg/...
ok  pkg/auth         ok  pkg/builder      ok  pkg/config       ok  pkg/faascontract
ok  pkg/gateway      ok  pkg/middleware   ok  pkg/provider     ok  pkg/scaletozero
ok  pkg/router       ok  pkg/secrets      ok  pkg/store
```

The package list is authoritative; exact test counts are not frozen because each
hardening pass adds focused regressions.

## Concurrency stress (host)

```
$ go test ./pkg/scaletozero -count=50                                  → ok
$ go test ./pkg/scaletozero -run 'ColdStart|Reclaim|Storm|Restart|Lease|Generation|Demot' -count=30  → ok
$ go test ./pkg/gateway -run 'ColdStart|ActivityLease|EnsureReady|NonceCache' -count=20  → ok (9.2s)
```

## Race detector + cgo (golang:1.25 Linux container)

Command shape:
`docker run --rm -v "/$(pwd)":/src -w //src -e CGO_ENABLED=1 golang:1.25 sh -c '<cmd>'`

```
go build ./...                → BUILD_OK
go vet ./...                  → VET_OK
go test -race ./pkg/...       → all 11 tested packages ok (race-clean):
  ok auth ok builder ok config ok faascontract ok gateway ok middleware
  ok provider ok router ok scaletozero ok secrets ok store
go test -race -count=10 ./pkg/scaletozero -run 'ColdStart|Storm|Reclaim|Restart|Demot|Generation|Lease|FailedStart|Hundred'  → ok
go test ./pkg/store/... (CGO on, real sqlite)  → ok
```

The full suite is **race-clean in-container**. A store cgo test
(`TestMigration_OldSchemaGainsAnnotations`) failed on the first container race
run — a genuine latent bug (scan of NULL legacy text columns) that the host cgo
gap had hidden — and was fixed by `COALESCE`-ing the nullable text columns in
`pkg/store/store.go`; the suite then passed.

## Container + faas-cli 0.18.0 end-to-end (real Docker daemon)

Isolated compose stack (unique project `dfsmoke`, ports 18080/19090, network
`dfsmoke-fn-net`, dedicated volumes) so it never touches other stacks. Image
built from the repo Dockerfile (golang:1.25 build stage). `export
OPENFAAS_URL=http://127.0.0.1:18080`.

| Operation | Command | Result |
|---|---|---|
| health | `GET /healthz` | OK |
| login | `faas-cli login` | credentials saved |
| info | `GET /system/info` | `provider.provider=docker-faas` (+ legacy `name`) |
| namespaces | `GET /system/namespaces` | `["openfaas-fn"]` |
| deploy | `faas-cli deploy figlet --label com.openfaas.scale.min/max --annotation` | 202 Accepted |
| describe | `GET /system/function/{name}` | pinned shape + annotations + namespace |
| list | `faas-cli list` | shows function |
| invoke (sync) | `faas-cli invoke` | real figlet ASCII output |
| unknown fn | `GET /system/function/does-not-exist` | 404 |
| async | `POST /async-function/{name}` | 202 + X-Call-Id |
| async bad callback | `X-Callback-Url: ftp://…` | 400 |
| scale | `POST /system/scale-function/{name}` replicas=2 | 202 |
| scale clamp | replicas=9 with scale.max=3 | clamped to 3 |
| secret create/list/update/remove | `faas-cli secret …` | 201 / listed / 200 / removed |
| logs | `GET /system/logs?name=…` | `application/x-ndjson`, pinned message shape |
| activity-lease (unconfigured) | `POST /system/scale/activity-lease` unsigned | 503 fail-closed |
| capabilities | `GET /system/scale/capabilities` | additive JSON, idle_scale_to_zero |
| remove | `faas-cli remove` | removed; list no longer shows it |
| redeploy-after-delete (RT-216) | deploy same name → invoke | works (gate/lease forgotten) |

Idle-enabled run (secret + `IDLE_SCALE_TO_ZERO_ENABLED=true`): capability flips
to `idle_scale_to_zero:true`; the RT-223 ownership fix verified live — 0 orphan
reclamations on the shared daemon (vs 2 before the fix), own function stamped
`com.docker-faas.gateway=dfsmoke-fn-net` and invocable. Stack fully torn down
afterward (`compose down -v`, network removed, 0 function containers remain).

## Independent follow-up verification (RT-240)

The follow-up review found and fixed the stale-name ownership bypass, added
direct async body-cap/drain tests, and exposed the new safety flags through
Compose. Current-tree evidence:

```
go test -count=1 ./...                            → all packages pass
go build ./...                                    → pass
go vet ./...                                      → pass
go test -count=10 ./pkg/provider -run
  'Test(RemoveStaleContainerByNameRespectsOwnership|StrictOwnershipExcludesUnlabeledLegacy|FunctionContainerLabelsReservedWin)$'
                                                   → pass
go test -count=10 ./pkg/gateway -run
  'Test(ReadAsyncCallbackBodyBoundsAndSignalsTruncation|HandleInvokeFunctionAsyncWithoutCallbackDrainsResponse|AsyncCallbackClientRevalidatesRedirects|HandleInvokeFunctionAsync_PostsCallback)$'
                                                   → pass
docker run golang:1.25 go test -race -count=1 ./... → all packages pass
```

`docker compose config` with `FUNCTIONS_NETWORK=review-owner-net`,
`FAAS_STRICT_CONTAINER_OWNERSHIP=true`, and
`FAAS_ASYNC_CALLBACK_BLOCK_INTERNAL=true` shows all three values in the gateway
container environment. `git diff --check` is clean. The deliberate
`pkg/faascontract/contract.go` formatting exception remains unchanged.

### Post-RT-240 isolated live smoke (2026-07-21)

The exact current tree was built into a temporary image and run against the real
Docker daemon with unique `codex-rt240-*` container, network, volume, function,
and host-port names. Strict ownership and callback SSRF blocking were enabled.
The pinned `faas-cli 0.18.0` passed health, login, deploy, list, synchronous
invoke, asynchronous invoke (`202` plus `X-Call-Id`), scale to two, and remove.
Both replicas carried the expected unique gateway-owner label.

For the RT-240 path, the test created a stopped `codex-rt240-foreign-0`
container labeled with `com.docker-faas.gateway=foreign-owner`, then attempted
to deploy the same function name through `faas-cli`. The gateway returned 500
with `container ... already exists but is not owned by this gateway`; Docker
inspection confirmed the foreign container still existed with its label
unchanged. All temporary containers, networks, volumes, and the review image
were removed afterward; no active MES stack resources were touched.

The smoke harness was then hardened to reproduce that isolation by default:
`scripts/smoke-openfaas-cli.sh` now generates unique resource/function names,
uses random localhost ports, enables strict ownership, and traps every exit to
remove only resources carrying its generated owner identity. Compose exposes
the corresponding container, image, port, network, and volume substitutions.

### Live-smoke correction for secrets/logs (RT-241/RT-242)

The first run of the hardened harness disproved the prior "secrets/logs live
green" claim: `faas-cli secret remove smoke-secret` failed because the gateway
only read `?name=` on DELETE, and the logs check used a non-existent faas-cli
0.18.0 flag (`--follow=false`). Fixes: DELETE `/system/secrets` now accepts the
pinned JSON `Secret` body and returns a faas-cli-compatible 200; the harness now
uses `faas-cli logs <name> --tail=false --lines 20`.

Fresh exact-tree evidence after the fix:

```
gofmt -l pkg/gateway/secrets_handlers.go pkg/gateway/secrets_handlers_test.go -> clean
bash -n scripts/smoke-openfaas-cli.sh                                      -> pass (Git Bash)
go test -count=1 ./pkg/gateway -run 'TestHandleDeleteSecret|TestHandleGetLogs' -> pass
go test -count=10 ./pkg/gateway -run 'TestHandleDeleteSecret|TestHandleGetLogs' -> pass
go test -count=1 ./...                                                    -> all packages pass
go build ./...                                                            -> pass
go vet ./...                                                              -> pass
git diff --check                                                          -> clean
docker run golang:1.25 go test -race -count=1 ./...                       -> all packages pass
```

Live smoke:

```
SMOKE_ID=codexfix-20260721 bash scripts/smoke-openfaas-cli.sh
PASS=25 FAIL=0
```

The live run used faas-cli 0.18.0 and a random localhost gateway port. It passed
health, login, deploy, list, describe, sync invoke, async invoke (`202` +
`X-Call-Id`), scale to 2, secrets create/list/update/remove, logs parsing,
namespaces, gateway info, raw contract checks, and remove. Post-run cleanup was
verified with empty Docker container/network/volume/image queries for the
`codexfix-20260721` prefix.

## Disclosed side effect
The FIRST idle-enabled container run (pre-RT-223-fix) reclaimed two function
containers of a separate docker-faas instance on the shared daemon
(`import-bundle`, `gateprobesquare`) via the unscoped orphan sweep. Detected in
the gateway log, the smoke gateway was stopped immediately. Impact recoverable
(docker-faas persists function metadata; reclaim == scale-to-zero, self-heals on
next invocation; the other gateway process stayed healthy). This exposed RT-223,
now fixed and re-verified. See `OBJECTIONS.md` and `ADJUDICATION.md`.
