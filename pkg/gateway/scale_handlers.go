package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/docker-faas/docker-faas/pkg/faascontract"
	"github.com/docker-faas/docker-faas/pkg/metrics"
	"github.com/docker-faas/docker-faas/pkg/scaletozero"
)

// PolicyResolver yields the idle policy for a function by name.
type PolicyResolver interface {
	PolicyFor(fn string) scaletozero.Policy
}

// scaleToZeroDeps bundles the idle scale-to-zero collaborators the gateway
// needs for the activity-lease and capability endpoints. It stays nil until
// SetScaleToZero is called, so existing deployments and tests are unaffected.
type scaleToZeroDeps struct {
	gates    *scaletozero.GateRegistry
	leases   *scaletozero.LeaseRegistry
	policies PolicyResolver
	decider  scaletozero.IdleDecider
	capable  bool
	now      func() time.Time
	// leaseSecret is the ISOLATED provider-side HMAC secret shared with the
	// AIDrivenMES control plane. It authenticates incoming activity-lease
	// requests and signs outgoing responses (CV-06 / SZ-12). Empty means the
	// endpoint is unconfigured and must fail closed.
	leaseSecret string
	// auth carries the RT-214 transport hardening: body limit, timestamp skew,
	// nonce replay cache, and previous-secret rotation overlap. Zero value =
	// hardening off (bare test wiring); main always wires it from config.
	auth leaseAuthPolicy
}

// SetLeaseAuthPolicy applies the RT-214 activity-lease hardening policy. Call
// after SetScaleToZero. prevSecret allows a rotation overlap window (verify
// with active OR previous; sign with active only); maxSkew bounds
// |now-IssuedAt| (0 disables the skew check — emergency escape hatch, logged
// by main); bodyLimit bounds the request body (<=0 uses the 64 KiB default; it
// cannot be disabled); replayCap bounds the nonce cache (<=0 uses the 65536
// default; at capacity the oldest entry is evicted, never a rejection —
// RT-217).
func (g *Gateway) SetLeaseAuthPolicy(prevSecret string, maxSkew time.Duration, bodyLimit int64, replayCap int) {
	if g.scale == nil {
		if g.logger != nil {
			g.logger.Warn("SetLeaseAuthPolicy called before SetScaleToZero: activity-lease hardening NOT applied")
		}
		return
	}
	g.scale.auth = leaseAuthPolicy{
		prevSecret: prevSecret,
		maxSkew:    maxSkew,
		bodyLimit:  bodyLimit,
		replay:     newNonceCache(replayCap, g.scale.now),
	}
}

// SetScaleToZero wires the idle scale-to-zero collaborators into the gateway.
// capable advertises whether the provider supports idle scale-to-zero (SZ-05).
// leaseSecret is the isolated shared secret used to HMAC-verify activity-lease
// requests and sign responses (CV-06); it must be a dedicated secret, never the
// application DB secret and never hardcoded.
func (g *Gateway) SetScaleToZero(gates *scaletozero.GateRegistry, leases *scaletozero.LeaseRegistry, policies PolicyResolver, capable bool, leaseSecret string) {
	g.scale = &scaleToZeroDeps{
		gates:       gates,
		leases:      leases,
		policies:    policies,
		capable:     capable,
		now:         time.Now,
		leaseSecret: leaseSecret,
	}
}

// scaleReady reports whether the idle scale-to-zero endpoints are wired.
func (g *Gateway) scaleReady() bool {
	return g.scale != nil && g.scale.gates != nil && g.scale.leases != nil
}

// HandleActivityLease handles POST /system/scale/activity-lease.
//
// It is the versioned, authenticated activity-lease endpoint (SZ-12): it
// accepts AIDrivenMES's admitted/queued/running durable counts plus the
// generation fence, combines them with the provider's OWN authoritative gateway
// in-flight HTTP accounting, and returns the resulting idle decision. A
// contract MAJOR-version mismatch is rejected with 409 so the mismatch fails
// readiness on the caller rather than silently disabling scale.
//
// Authentication (CV-06): the request's HMAC signature is verified with the
// isolated provider-side shared secret BEFORE any lease state is applied. An
// unsigned, wrong-secret, or tampered request is rejected with 401 and never
// reaches leases.Apply, so it can never produce a scale side effect. The
// response is signed with the SAME secret so the control plane can verify it.
func (g *Gateway) HandleActivityLease(w http.ResponseWriter, r *http.Request) {
	if !g.scaleReady() {
		http.Error(w, "idle scale-to-zero not enabled", http.StatusServiceUnavailable)
		return
	}

	secret := g.scale.leaseSecret
	if secret == "" {
		// Fail closed: with no isolated signing secret we cannot authenticate the
		// control plane, so we must never accept an (unauthenticated) lease. This
		// is a provider misconfiguration; startup already refuses to enable idle
		// scale-to-zero without the secret, so this is defense in depth.
		metrics.RecordActivityLease("unconfigured")
		http.Error(w, "activity-lease signing secret not configured", http.StatusServiceUnavailable)
		return
	}

	// RT-214: bound the body BEFORE decoding. The lease message is a small flat
	// JSON object; anything past the limit is rejected with 413 pre-auth and,
	// like every rejection in this handler, before any state mutation.
	r.Body = http.MaxBytesReader(w, r.Body, g.scale.auth.effectiveBodyLimit())

	var req faascontract.ActivityLeaseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			metrics.RecordActivityLease("too_large")
			http.Error(w, "activity-lease request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		metrics.RecordActivityLease("invalid")
		http.Error(w, "invalid activity-lease request body", http.StatusBadRequest)
		return
	}

	// Verify the HMAC over the request EXACTLY as received, before normalizing or
	// applying anything (CV-06). A major-version mismatch is a readiness failure
	// (409); a bad/missing signature is an auth failure (401). Either way we
	// return before leases.Apply, so a rejected request has zero scale effect and
	// issues zero Docker commands. Rotation overlap (RT-214): a request signed
	// with the PREVIOUS secret verifies while both are configured; responses are
	// always signed with the active secret.
	if err := faascontract.VerifyActivityLeaseRequest(req, secret); err != nil {
		if errors.Is(err, faascontract.ErrContractVersionMismatch) {
			metrics.RecordActivityLease("version_mismatch")
			g.writeJSON(w, http.StatusConflict, map[string]string{
				"error":                     "contract version mismatch",
				"provider_contract_version": faascontract.ContractVersion,
				"peer_contract_version":     req.ContractVersion,
			})
			return
		}
		prev := g.scale.auth.prevSecret
		if prev == "" || faascontract.VerifyActivityLeaseRequest(req, prev) != nil {
			metrics.RecordActivityLease("unauthenticated")
			http.Error(w, "activity-lease signature verification failed", http.StatusUnauthorized)
			return
		}
	}

	// RT-214: timestamp skew. A signed request must carry an IssuedAt within
	// now±skew; outside that window it is stale (a capture-replay across the
	// window, or unacceptable clock drift) and is rejected pre-mutation.
	if skew := g.scale.auth.maxSkew; skew > 0 {
		now := g.scale.now()
		if req.IssuedAt.IsZero() || req.IssuedAt.Before(now.Add(-skew)) || req.IssuedAt.After(now.Add(skew)) {
			metrics.RecordActivityLease("skew")
			http.Error(w, "activity-lease issued_at outside acceptable window", http.StatusUnauthorized)
			return
		}
	}

	req.Function = normalizeFunctionName(req.Function)

	if err := req.Validate(); err != nil {
		if errors.Is(err, faascontract.ErrContractVersionMismatch) {
			metrics.RecordActivityLease("version_mismatch")
			// 409 Conflict: the peer speaks an incompatible contract version.
			g.writeJSON(w, http.StatusConflict, map[string]string{
				"error":                     "contract version mismatch",
				"provider_contract_version": faascontract.ContractVersion,
				"peer_contract_version":     req.ContractVersion,
			})
			return
		}
		metrics.RecordActivityLease("invalid")
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// The lease path applies the same function-name charset/length rules as
	// deploy: even a secret-holding peer cannot store leases under arbitrary
	// byte strings or explode metric label cardinality (RT-220).
	if err := validateFunctionName(req.Function); err != nil {
		metrics.RecordActivityLease("invalid")
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// RT-214: nonce replay protection. Within the replay window each nonce is
	// accepted at most once; a captured signed request cannot be re-posted to
	// re-assert a stale lease. Only reached by authenticated requests, so the
	// bounded cache cannot be filled by an attacker without the secret. Runs
	// AFTER structural validation so a signed-but-invalid request does not
	// burn its nonce (RT-220); it remains ahead of every state mutation. The
	// TTL exceeds the inclusive freshness window by a second so a nonce can
	// never expire while its timestamp is still acceptable (RT-220).
	if cache := g.scale.auth.replay; cache != nil {
		if req.Nonce == "" {
			metrics.RecordActivityLease("missing_nonce")
			http.Error(w, "activity-lease nonce is required", http.StatusBadRequest)
			return
		}
		ttl := 2*g.scale.auth.maxSkew + time.Second
		if g.scale.auth.maxSkew <= 0 {
			ttl = 2*defaultLeaseSkew + time.Second
		}
		replayed, evicted := cache.remember(req.Nonce, ttl)
		if replayed {
			metrics.RecordActivityLease("replay")
			http.Error(w, "activity-lease nonce already used", http.StatusUnauthorized)
			return
		}
		if evicted {
			// Accepted, but the cache hit capacity and evicted its oldest
			// entry — observability only (RT-217): rejecting legitimate
			// renewals would cascade into lease expiry and wrongful reclaim.
			metrics.RecordActivityLease("replay_cache_evicted")
		}
	}

	now := g.scale.now()
	leaseExpires := g.scale.leases.Apply(req)

	gen := g.scale.gates.Generation(req.Function)
	view := g.scale.leases.View(req.Function, gen)
	accepted := !view.StaleGeneration

	if accepted {
		// Fold the renewal into the gate's authoritative activity anchor NOW,
		// not only at the next reconcile pass. A lease renewal is an assertion of
		// current activity (RT-221); capturing it here means the idle window is
		// always measured from the last renewal even if this function's stored
		// lease is later pruned by another function's Apply (the opportunistic
		// expired-lease prune, RT-220) before the reconciler could fold it —
		// closing the premature-reclaim window the prune would otherwise open.
		g.scale.gates.MarkActivity(req.Function, now)
	}

	observed := g.observedReplicas(r.Context(), req.Function)
	policy := g.scale.policies.PolicyFor(req.Function)

	snap := scaletozero.ActivitySnapshot{
		GatewayInFlight:  g.scale.gates.InFlight(req.Function),
		DurableInFlight:  view.DurableInFlight,
		LastActivity:     laterTime(g.scale.gates.LastActivity(req.Function), view.LastActivity),
		ObservedReplicas: observed,
	}
	decision := g.scale.decider.Decide(now, policy, snap)

	if accepted {
		metrics.RecordActivityLease("accepted")
	} else {
		metrics.RecordActivityLease("stale_generation")
	}
	metrics.RecordScaleDecision(req.Function, decision.Action.String())

	resp := faascontract.ActivityLeaseResponse{
		ContractVersion:          faascontract.ContractVersion,
		Function:                 req.Function,
		Accepted:                 accepted,
		Generation:               gen,
		LeaseExpiresAt:           leaseExpires,
		GatewayInFlight:          snap.GatewayInFlight,
		DurableInFlight:          snap.DurableInFlight,
		TotalInFlight:            snap.TotalInFlight(),
		ObservedReplicas:         observed,
		IdleScaleToZeroSupported: g.scale.capable,
		Decision:                 decision.Action.AsContractDecision(),
		DecisionReason:           decision.Reason,
		IssuedAt:                 now.UTC(),
		// Echo the request nonce so the response is cryptographically bound to
		// THIS request: a captured signed response for an older lease cannot be
		// replayed as the answer to a fresh one (RT-214). Falls back to a fresh
		// id when the peer sent no nonce (replay enforcement then also off).
		Nonce: req.Nonce,
	}
	if resp.Nonce == "" {
		resp.Nonce = generateCallID()
	}
	// Sign the response with the SAME isolated secret so the control plane can
	// authenticate it (CV-06). SignActivityLeaseResponse stamps the contract
	// version, normalizes timestamps to UTC, and fills Signature in place.
	signed := faascontract.SignActivityLeaseResponse(resp, secret)
	g.writeJSON(w, http.StatusOK, signed)
}

// HandleScaleCapabilities handles GET /system/scale/capabilities. Readiness on
// the AIDrivenMES side depends on this declaration rather than an
// OpenFaaS-compatible API name (SZ-05/SZ-10). Kubernetes stays explicitly
// unclaimed.
func (g *Gateway) HandleScaleCapabilities(w http.ResponseWriter, r *http.Request) {
	capable := g.scale != nil && g.scale.capable
	caps := faascontract.Capabilities{
		ContractVersion: faascontract.ContractVersion,
		Provider:        "docker-faas",
		Orchestration:   "docker",
		IdleScaleToZero: capable,
		ScaleFromZero:   true,
		Kubernetes: faascontract.KubernetesCapability{
			Supported: false,
			Reason:    "not selected; Docker is the required first target and no Kubernetes production claim is made from this provider",
		},
	}
	g.writeJSON(w, http.StatusOK, caps)
}

// observedReplicas counts the running containers for a function using the
// existing provider surface (no new provider capability required).
func (g *Gateway) observedReplicas(ctx context.Context, functionName string) int {
	containers, err := g.provider.GetFunctionContainers(ctx, functionName)
	if err != nil {
		g.logger.Debugf("activity-lease: get containers for %s: %v", functionName, err)
		return 0
	}
	running := 0
	for _, c := range containers {
		if strings.Contains(c.Status, "running") || strings.Contains(c.Status, "Up") {
			running++
		}
	}
	return running
}

func laterTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

// Cold-start coordination helpers (trackInvocation, ensureReadyFromZero) live
// in coldstart.go.
