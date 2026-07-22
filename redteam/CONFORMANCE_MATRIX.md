# OpenFaaS Provider-Contract Conformance Matrix

Owner: builder-conf. This file is the single honest inventory of the
docker-faas gateway surface versus the pinned upstream OpenFaaS contract.
Every "supported" row cites evidence (code + test); rows without evidence are
marked unproven. Do not claim support here without a citation.

## Pinned upstream versions

| Component | Exact tag | Source URL | Fetched |
|---|---|---|---|
| openfaas/faas-provider (types + logs contract) | `v0.25.12` (released 2026-05-13) | https://github.com/openfaas/faas-provider/tree/v0.25.12 (`types/function_status.go`, `types/function_deployment.go`, `types/requests.go`, `logs/logs.go`, `logs/handler.go`) | 2026-07-21 |
| openfaas/faas (gateway + OpenAPI spec) | `0.27.13` (released 2025-08-29) | https://github.com/openfaas/faas/blob/0.27.13/api-docs/spec.openapi.yml, `gateway/types/inforequest.go`, `gateway/handlers/callid_middleware.go`, `gateway/handlers/queue_proxy.go` | 2026-07-21 |
| openfaas/faas-cli (client behavior) | `0.18.0` (binary installed locally; sources at tag) | https://github.com/openfaas/faas-cli/tree/0.18.0 (`proxy/utils.go`, `proxy/describe.go`, `proxy/logs.go`, `proxy/secret.go`, `proxy/namespaces.go`, `proxy/version.go`, `commands/logs.go`, `commands/secret_remove.go`) | 2026-07-21 |
| openfaas/nats-queue-worker (async callback contract) | `0.14.2` | https://github.com/openfaas/nats-queue-worker/blob/0.14.2/main.go (`postResult`) | 2026-07-21 |
| openfaas/faasd (single-node reference: default namespace + invalid-namespace behavior) | `master` (read 2026-07-21; no namespace-behavior tag pin needed — behavior stable) | https://github.com/openfaas/faasd/blob/master/pkg/provider/handlers/read.go | 2026-07-21 |
| OpenFaaS autoscaling label reference | docs.openfaas.com/architecture/autoscaling | https://docs.openfaas.com/architecture/autoscaling/ | 2026-07-21 |

Key pinned facts used throughout:

- `FunctionStatus` JSON keys (faas-provider v0.25.12): `name, image, namespace,
  envProcess, envVars, constraints, secrets, labels, annotations, limits,
  requests, readOnlyRootFilesystem, invocationCount, replicas,
  availableReplicas, createdAt, usage`.
- `ProviderInfo` emits the provider name under key **`provider`** (not `name`);
  `VersionInfo` = `{sha, release, commit_message?}`; gateway `/system/info` =
  `{provider: ProviderInfo, version: VersionInfo, arch}`.
- Log `Message` (faas-provider v0.25.12 `logs/logs.go`): `{name, namespace,
  instance, timestamp, text}`, streamed as NDJSON with
  `Content-Type: application/x-ndjson`, `Connection: Keep-Alive`,
  `Transfer-Encoding: chunked`, flush per message; query parse errors → 422.
- Async callback headers (nats-queue-worker 0.14.2): function response headers
  copied, plus `X-Call-Id` (when present), `X-Function-Name`,
  `X-Function-Status` (`%d`), `X-Duration-Seconds` (`%f`); single attempt, no
  retry, log on failure. Gateway rejects unparsable `X-Callback-Url` with 400.
- Sync invoke stamping (faas 0.27.13 `callid_middleware.go`): request+response
  get `X-Call-Id` (existing value preserved) and `X-Start-Time` (UTC UnixNano
  string); response gets an `X-Served-By` marker.
- `/system/namespaces` returns a JSON `[]string` (faas-cli 0.18.0
  `proxy/namespaces.go` decodes exactly that).
- Default namespace: `openfaas-fn` (faasd default; matches the
  `.openfaas-fn` suffix `normalizeFunctionName` strips). faasd answers 400
  `"namespace not valid"` for any other namespace.

## End-to-end verification against the real faas-cli 0.18.0 binary

Initial conformance checks used a throwaway harness (scratchpad, not committed)
that wired the real gateway handlers to in-memory fakes on `127.0.0.1:18085`.
The final mandatory surface was also exercised against a containerized gateway
on the real Docker daemon by `scripts/smoke-openfaas-cli.sh`. Verified on
2026-07-21/2026-07-22 with the locally installed faas-cli 0.18.0:

- `faas-cli list` / `list --verbose` — renders name/image/replicas.
- `faas-cli describe hello` — renders status incl. **Labels and Annotations**.
- `faas-cli version` — parses gateway release/sha and provider
  `name: docker-faas`, `orchestration: docker` from the pinned keys.
- `faas-cli namespace list` — prints `openfaas-fn`.
- `faas-cli logs hello --tail=false [--instance --name]` — parses the NDJSON
  stream and formats timestamps/instance.
- `faas-cli invoke hello` (sync, echoes body) and `invoke --async` (202).
- `faas-cli deploy --label com.openfaas.scale.min=2 --annotation topic=orders`
  → 202; `describe` shows `Replicas: 2`; `GET /system/function/echoer` returns
  `"annotations":{"topic":"orders"}`.
- `faas-cli remove echoer` — 202, function deleted.
- `faas-cli secret create/list/update/remove` - create 201, list renders the
  secret, update 200, remove succeeds with the JSON-body DELETE contract.

## Inventory

Legend — status: `supported` (implemented + tested), `partial`, `unsupported`,
`extension` (additive, not in the pinned contract). "ours vs pinned" compares
status codes with the pinned OpenAPI spec (faas 0.27.13).

### Mandatory / standard OpenFaaS surface

| Operation | Method + path | Contract | Our status | Status codes ours vs pinned | Evidence (file:line approx + test) | Notes |
|---|---|---|---|---|---|---|
| List functions | GET `/system/functions` | mandatory | supported | 200/500 vs 200 | `pkg/gateway/handlers.go` `HandleListFunctions`; tests `TestHandleListFunctions_IncludesNamespaceAndAnnotations`, `TestHandleListFunctions_UnknownNamespace400`; faas-cli `list` verified | Emits pinned FunctionStatus keys incl. `namespace: "openfaas-fn"` and persisted `annotations`. `?namespace=` validated (unknown → 400 "namespace not valid", faasd behavior). |
| Deploy function | POST `/system/functions` | mandatory | supported | 202/400/409/500 vs 202/400/500 | `HandleDeployFunction`; tests `TestHandleDeployFunction_CreatesFunction`, `_PersistsAnnotations`, `_HonorsOpenFaaSScaleMinLabel`, `_CustomMinLabelWinsOverOfficial`, `_ClampsMinToConfigMax`, `_ClampsMinToDefaultMaxWithoutConfig`, `_NamespaceValidation`; faas-cli `deploy` verified | 409 on duplicate is an additive divergence (pinned spec has no explicit duplicate-deploy code; faas-cli treats non-2xx as error and tells the user). Initial replicas = clamp(scale.min label, 1, config MaxReplicas; default max 10). |
| Update function | PUT `/system/functions` | mandatory | supported | **202** vs 200/400/404/500 | `HandleUpdateFunction`; test `TestHandleUpdateFunction_PersistsAnnotations`; 404/400 paths shared with deploy validation | DIVERGENCE (kept deliberately): we answer 202, pinned spec documents 200. faas-cli 0.18.0 accepts 200/201/202 for deploy/update, so no client breakage. Not silently changed to avoid breaking existing consumers mid-flight; flagged for a follow-up alignment decision. |
| Delete function | DELETE `/system/functions` | mandatory | supported | **202**/400/404/500 vs 200/400/404/500 | `HandleDeleteFunction`; test `TestHandleDeleteFunction_UnknownNamespace400`; faas-cli `remove` verified | Body is pinned `DeleteFunctionRequest {functionName, namespace}` (+ legacy `service` accepted additively). Same 202-vs-200 divergence as PUT; faas-cli accepts both. |
| Get one function | GET `/system/function/{name}` | mandatory | supported | 200/400/404/500 vs 200/404/500 | `HandleGetFunction` in `pkg/gateway/handlers.go`; route in `cmd/gateway/main.go`; tests `TestHandleGetFunction_ReturnsPinnedStatusShape`, `_NormalizesNamespaceSuffix`, `_UnknownFunction404` (plain-text), `_UnknownNamespace400`; faas-cli `describe` verified | `?namespace=` accepts only `openfaas-fn`/empty; unknown → 400 "namespace not valid" (faasd behavior; pinned spec only documents 404, recorded decision). Name suffix `.openfaas-fn` normalized. |
| Scale function | POST `/system/scale-function/{name}` | mandatory | supported | 202/400/404/500 vs 200/202/404/500 | `HandleScaleFunction`; tests `TestHandleScaleFunction_UpdatesReplicas`, `_ClampsToMaxLabel`, `_CustomMaxLabelWins`, `_AllowsExplicitScaleToZero`, `_UnknownNamespace400` | 202 is within the pinned 200-or-202 set. Requests above `com.openfaas.scale.max` are **clamped** (logged); upstream clamp-vs-reject is ambiguous at the pin → clamp chosen, recorded here. Explicit scale to 0 allowed (pause). |
| System info | GET `/system/info` | mandatory | supported | 200 vs 200/500 | `HandleSystemInfo` + `pkg/types/types.go` (`SystemInfo`, `ProviderInfo`, `VersionInfo`); test `TestHandleSystemInfo_PinnedProviderInfoShape`; faas-cli `version` verified | Provider name now under pinned key `provider` (legacy `name` kept additively with same value); version objects `{sha, release}`. |
| Health | GET `/healthz` | mandatory | supported | 200/503 vs 200/500 | `HandleHealthz` (pre-existing) | DIVERGENCE: unhealthy answers 503, pinned spec says 500. 503 is the more semantically correct code; flagged, not changed (out of this change's need; both are "non-200" to probes). No test asserts the unhealthy code. |
| Namespaces | GET `/system/namespaces` | mandatory (for multi-ns aware clients) | supported | 200 vs 200/500 | `HandleListNamespaces`; route in `cmd/gateway/main.go`; test `TestHandleListNamespaces_ReturnsDefaultNamespaceList`; faas-cli `namespace list` verified | Returns `["openfaas-fn"]` as plain `[]string`, the shape faas-cli 0.18.0 decodes. |
| Function logs | GET `/system/logs` | mandatory | supported | 200/400/404/422/500 vs 200/404/500 | `pkg/gateway/logs_handlers.go` `HandleGetLogs` + `pkg/provider/logs_stream.go` `StreamFunctionLogs`; tests `TestHandleGetLogs_StreamsPinnedNDJSON`, `_FiltersByInstance`, `_ParseFailures422`, `_UnknownFunction404`, `_UnknownNamespace400`, `_NormalizesNameSuffix`, `_FallbackParsesLegacyBlob`, `TestParseDockerLogLine`, `TestStripDockerFrameHeader`; faas-cli `logs` verified | REPLACED the old text/plain blob. Output is the pinned NDJSON Message stream (`application/x-ndjson`, flush per line). Query: `name` (required), `namespace`, `instance`, `since` RFC3339, `tail` int, `follow` bool; parse errors → 422 (pinned). Docker streaming demuxes stdcopy frames, parses timestamps, fans in all containers (instance = container name), honors follow/tail/since. Providers without the optional streaming capability fall back to a parsed one-shot blob (tail defaults to 100 there). Missing `name` → 422 (pinned handler leaves this provider-defined; recorded decision). Docker-daemon streaming path itself is exercised only in real deployments (no daemon-bound unit test) — parsing/demux helpers are unit-tested. |
| Sync invoke | ANY `/function/{name}` | mandatory | supported | 200 (function's code)/400/404/500 vs 200/404/500 | `HandleInvokeFunction`; tests `TestHandleInvokeFunction_RoutesRequest`, `_StampsCallHeaders`, `_PreservesCallerCallID`; faas-cli `invoke` verified | Now stamps `X-Call-Id` (preserving caller value) + `X-Start-Time` (UnixNano) on request and response, `X-Served-By: docker-faas/<release>` — mirrors pinned callid middleware. |
| Sync invoke (ns suffix) | ANY `/function/{name}.{namespace}` | mandatory | supported | as above | `normalizeFunctionName` strips `.openfaas-fn`/`.openfaas`; test `TestHandleGetFunction_NormalizesNamespaceSuffix` (same normalizer); route `/function/{name}` matches the dotted form | Unknown-namespace suffix (e.g. `fn.prod`) fails function lookup → 404, matching pinned expectations. |
| Async invoke | POST `/async-function/{name}` | mandatory | supported | 202/400/404/500 vs 202/404/500 | `pkg/gateway/async_handlers.go` `HandleInvokeFunctionAsync`; tests `TestHandleInvokeFunctionAsync_PostsCallback`, `_RejectsInvalidCallbackURL`; faas-cli `invoke --async` verified | Responds 202 + `X-Call-Id`. NEW: `X-Callback-Url` honored — after the function responds, its body+headers are POSTed to the callback with `X-Call-Id`, `X-Function-Name`, `X-Function-Status`, `X-Duration-Seconds` (pinned queue-worker 0.14.2 set). No retry; failures logged; callback wait bounded by a 30s client timeout. Invalid callback URL → 400 (pinned gateway); scheme restricted to http/https (our hardening, stricter than pin — recorded). Router errors are reported to the callback as status 500 with the error text (queue-worker behavior). JSON `{status, callId}` response body is an additive extension (pinned gateway sends no body). |
| Secrets list | GET `/system/secrets` | mandatory (secret-capable providers) | supported | 200/400/500 vs 200 | `pkg/gateway/secrets_handlers.go` `HandleListSecrets`; faas-cli `secret list` verified in the isolated live smoke | Returns name-only JSON objects, which faas-cli decodes into `types.Secret`. `?namespace=` accepts only empty/`openfaas-fn`; unknown namespace → 400. |
| Secret create/update/delete | POST/PUT/DELETE `/system/secrets` | mandatory (secret-capable providers) | supported | 201/200/**200**/400/404/500 vs 201/200/**204**/400/404/500 | `pkg/gateway/secrets_handlers.go`; tests `TestHandleDeleteSecret_AcceptsFaasCLIJSONBody`, `_KeepsLegacyQueryName`, `_RejectsUnknownNamespace`, `_RequiresName`; faas-cli `secret create/list/update/remove` verified live | Create/update accept pinned `Secret` body `{name, namespace, value, rawValue}`. DELETE accepts the pinned JSON body sent by faas-cli plus legacy `?name=`. DIVERGENCE: delete returns 200, not spec 204, because faas-cli 0.18.0 treats 204 as unexpected and accepts only 200/202. |
| Secret value read | GET `/system/secrets/{name}` | **NOT in OpenFaaS** | extension — **flagged** | n/a | `pkg/gateway/secrets_handlers.go` `HandleGetSecret` (out of scope); route in `cmd/gateway/main.go:219` | RED FLAG: OpenFaaS **never returns secret values** over the API (list returns names only). This route exposes secret material behind gateway auth. Recommendation: remove, or gate behind an explicit opt-in config flag + audit logging. Out of builder-conf's edit scope — needs owner action. |

### Additive extensions (not in the pinned contract; safe to keep, must never shadow standard routes)

| Operation | Method + path | Our status | Evidence | Notes |
|---|---|---|---|---|
| Build from source | POST `/system/builds`, GET `/system/builds`, DELETE `/system/builds`, POST `/system/builds/inspect`, GET `/system/builds/stream`, GET `/system/builds/{id}` | extension | `pkg/gateway/build_handlers.go`, `builds_handlers.go` (out of scope) | docker-faas-specific. |
| Scale-to-zero capability discovery | GET `/system/scale/capabilities` | extension | `pkg/gateway/scale_handlers.go` `HandleScaleCapabilities` | Versioned faascontract shape, deliberately NOT OpenFaaS-named. |
| Activity lease | POST `/system/scale/activity-lease` | extension | `scale_handlers.go` `HandleActivityLease` (HMAC-authenticated) | Cross-repo faascontract v1. |
| Function containers | GET `/system/function/{name}/containers` | extension | `handlers.go` `HandleFunctionContainers` | Registered before `/system/function/{name}`; gorilla-mux matches the more specific path — no shadowing. |
| Config view | GET `/system/config` | extension | `auth_handlers.go` (out of scope) | |
| Metrics | GET `/system/metrics` (+ `:9090/metrics`) | extension | `cmd/gateway/main.go` promhttp | |
| Legacy async alias | POST `/system/function-async/{name}` | extension | `cmd/gateway/main.go:206` | Same handler as `/async-function/{name}`. |
| UI/docs | `/ui/*`, `/docs/*`, `/` redirect | extension | `cmd/gateway/main.go` | |
| Auth | POST `/auth/login`, `/auth/logout` | extension | `auth_handlers.go` (out of scope) | OpenFaaS uses basic-auth/OIDC plugins instead; ours is additive. |

### Scale-label conformance (labels, not routes)

| Label | Pinned meaning | Our handling | Evidence |
|---|---|---|---|
| `com.openfaas.scale.zero` | "true" enables scale-to-zero for the function | honored (bool, `ParseBool` so `1`/`true` work) | `idle_controller.go` `policyFor`; `TestPolicyFor_OfficialOpenFaaSLabels`, `TestPolicyFor_OfficialZeroLabelNumericTrue` |
| `com.openfaas.scale.zero-duration` | Go duration idle window ("15m") | honored; Go duration AND bare seconds accepted | `parseScaleZeroDuration`; `TestPolicyFor_ZeroDurationBareSeconds`, `TestParseScaleZeroDuration` |
| `com.openfaas.scale.min` | min replicas (int ≥ 1) | honored for idle policy AND initial deploy replicas (clamped to config max) | `scaleBoundsFromLabels`, `initialReplicas`; `TestHandleDeployFunction_HonorsOpenFaaSScaleMinLabel`, `_ClampsMinToConfigMax` |
| `com.openfaas.scale.max` | max replicas (int ≥ 1) | honored for idle policy AND explicit scale clamp (logged) | `HandleScaleFunction`; `TestHandleScaleFunction_ClampsToMaxLabel` |
| Precedence | n/a | custom `com.docker-faas.scale.*` label wins over `com.openfaas.scale.*` when both present (custom = additive override), else official label, else config defaults | documented in `idle_controller.go`; `TestPolicyFor_CustomLabelsWinOverOfficial`, `TestScaleBoundsFromLabels/custom_labels_win`, `TestHandleDeployFunction_CustomMinLabelWinsOverOfficial`, `TestHandleScaleFunction_CustomMaxLabelWins` |

### Data-shape conformance

| Shape | Pinned source | Our status | Evidence |
|---|---|---|---|
| `FunctionStatus` | faas-provider v0.25.12 `types/function_status.go` | keys match; additive extras `network`, `debug`, `updatedAt`; `constraints`/`usage` fields present (usage currently never populated); `invocationCount` always 0 (no per-function counter store) — honest gap | `pkg/types/types.go`; `TestHandleGetFunction_ReturnsPinnedStatusShape` |
| `FunctionDeployment` | v0.25.12 `types/function_deployment.go` | keys accepted incl. `annotations`, `namespace`, `constraints` (constraints accepted but ignored by the Docker provider — recorded) | `pkg/types/types.go` |
| `ScaleServiceRequest` | v0.25.12 `types/requests.go` | `{serviceName, replicas, namespace}` accepted (+ legacy `service`/`functionName` aliases) | `pkg/types/types.go`; `TestHandleScaleFunction_UnknownNamespace400` |
| `DeleteFunctionRequest` | v0.25.12 `types/requests.go` | `{functionName, namespace}` accepted (+ legacy `service`) | `HandleDeleteFunction` |
| log `Message` | v0.25.12 `logs/logs.go` | exact keys, no omitempty | `logs_handlers.go` `logMessage`; `TestHandleGetLogs_StreamsPinnedNDJSON` |
| `Secret` | v0.25.12 `types/secret.go` | `{name, namespace, value, rawValue}` accepted for create/update/delete; list returns name-only objects | `secrets_handlers.go`; `TestHandleDeleteSecret_AcceptsFaasCLIJSONBody`; live faas-cli secrets CRUD |
| `ProviderInfo`/`VersionInfo`/gateway info | v0.25.12 `types/requests.go` + faas 0.27.13 `inforequest.go` | pinned keys + legacy additive `name`, `commit_date` | `pkg/types/types.go`; `TestHandleSystemInfo_PinnedProviderInfoShape` |
| Annotations persistence | n/a (storage detail) | TEXT column, JSON encoded; guarded migration v3 (version-tracked `ALTER TABLE ADD COLUMN`, old DBs open cleanly) | `pkg/store/migrations.go`, `pkg/store/store.go`; `TestMigration_OldSchemaGainsAnnotations` (cgo/CI), `TestMigrations_RegisterAnnotationsMigration` (always runs) |

## Decisions taken where upstream was ambiguous

1. **Explicit scale beyond `com.openfaas.scale.max`: clamp, not reject.**
   The pinned CE gateway forwards scale requests to the provider and provider
   behavior differs by implementation; docs only say max "limits" replicas.
   We clamp to the label max and log. Explicit scale to 0 remains allowed.
2. **Unknown namespace → 400 `"namespace not valid"`** on list/status/deploy/
   update/delete/scale/logs, matching faasd (reference single-node provider),
   even though the pinned OpenAPI only documents 404 for function-level routes.
3. **`/system/logs` missing/invalid `name` → 422** (the pinned faas-provider
   handler 422s all query parse errors; it leaves empty-name semantics to the
   provider requester — we fold it into the 422 class).
4. **Log streaming uses `r.Context()`** for disconnect detection instead of the
   deprecated `http.CloseNotifier` the pinned handler still uses; requires
   `http.Flusher` exactly like the pin (non-flushable writer → 404).
5. **`X-Callback-Url` scheme restricted to http/https** (pinned gateway only
   requires parseability). Prevents scheme-smuggling; recorded as deliberate
   hardening.
6. **PUT/DELETE `/system/functions` keep answering 202** (pinned spec: 200).
   faas-cli 0.18.0 accepts 200/201/202, so nothing breaks; changing it is a
   one-line follow-up if exact spec-code parity is later required.
7. **`X-Served-By: docker-faas/<release>`** — pinned gateway sends
   `openfaas-ce/<version>`; we do not impersonate OpenFaaS CE.
8. **Async 202 body** keeps the additive JSON `{status, callId}` (pinned
   gateway sends an empty body); header `X-Call-Id` matches the pin.
9. **DELETE `/system/secrets` returns 200, not 204.** The pinned OpenAPI says
   204, but faas-cli 0.18.0 `proxy.RemoveSecret` accepts only 200/202. We choose
   standard-client compatibility and document the spec-code divergence.

## Known gaps / out-of-scope findings (not fixed here)

- `GET /system/secrets/{name}` returns secret **values** — contract violation
  by OpenFaaS norms; needs removal/gating (out of builder-conf scope).
- `invocationCount` is always 0; a per-function invocation counter (store or
  metrics-backed) would make `faas-cli list` counts real.
- `/healthz` unhealthy answers 503 vs pinned 500 (see row).
- `constraints` accepted on deploy but ignored by the Docker provider.
- `usage` (CPU/RAM) never populated in FunctionStatus (`?usage=1` from
  faas-cli describe is accepted and ignored).
- `pkg/gateway/coldstart_test.go` fails `gofmt -l` (CRLF line endings); file is
  outside builder-conf scope — owner should run gofmt on it.
