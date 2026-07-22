package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/docker/docker/client"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"

	"github.com/docker-faas/docker-faas/pkg/faascontract"
	"github.com/docker-faas/docker-faas/pkg/scaletozero"
	"github.com/docker-faas/docker-faas/pkg/secrets"
	"github.com/docker-faas/docker-faas/pkg/types"
)

// --- concurrency-safe fakes for the scale-to-zero gateway tests ---

type syncStore struct {
	mu        sync.Mutex
	functions map[string]*types.FunctionMetadata
}

func (s *syncStore) ListFunctions() ([]*types.FunctionMetadata, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*types.FunctionMetadata, 0, len(s.functions))
	for _, fn := range s.functions {
		out = append(out, fn)
	}
	return out, nil
}

func (s *syncStore) GetFunction(name string) (*types.FunctionMetadata, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if fn, ok := s.functions[name]; ok {
		clone := *fn
		return &clone, nil
	}
	return nil, io.EOF
}

func (s *syncStore) CreateFunction(m *types.FunctionMetadata) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.functions[m.Name] = m
	return nil
}

func (s *syncStore) UpdateFunction(m *types.FunctionMetadata) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.functions[m.Name] = m
	return nil
}

func (s *syncStore) DeleteFunction(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.functions, name)
	return nil
}

func (s *syncStore) UpdateReplicas(name string, replicas int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if fn, ok := s.functions[name]; ok {
		fn.Replicas = replicas
	}
	return nil
}

func (s *syncStore) HealthCheck(context.Context) error { return nil }

// coldStartProvider counts container-mutating calls and flips containers to
// running once scaled, so cold-start behavior is observable and race-free.
type coldStartProvider struct {
	mu         sync.Mutex
	scaled     bool
	scaleCalls int
	scaleDelay time.Duration

	// mutation counters (SZ-06 no-host-control assertions)
	deployCalls  int
	updateCalls  int
	removeCalls  int
	cleanupCalls int
}

func (p *coldStartProvider) DeployFunction(context.Context, *types.FunctionDeployment, int) error {
	p.mu.Lock()
	p.deployCalls++
	p.mu.Unlock()
	return nil
}

func (p *coldStartProvider) UpdateFunction(context.Context, *types.FunctionDeployment, int) error {
	p.mu.Lock()
	p.updateCalls++
	p.mu.Unlock()
	return nil
}

func (p *coldStartProvider) RemoveFunction(context.Context, string) error {
	p.mu.Lock()
	p.removeCalls++
	p.mu.Unlock()
	return nil
}

func (p *coldStartProvider) ScaleFunction(_ context.Context, _ *types.FunctionDeployment, target int) error {
	p.mu.Lock()
	p.scaleCalls++
	delay := p.scaleDelay
	p.mu.Unlock()
	if delay > 0 {
		time.Sleep(delay)
	}
	p.mu.Lock()
	if target > 0 {
		p.scaled = true
	} else {
		p.scaled = false
	}
	p.mu.Unlock()
	return nil
}

func (p *coldStartProvider) GetFunctionContainers(_ context.Context, name string) ([]*types.Container, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.scaled {
		return []*types.Container{}, nil
	}
	return []*types.Container{{Name: name + "-0", Status: "running", IPAddress: "10.0.0.2"}}, nil
}

func (p *coldStartProvider) GetContainerLogs(context.Context, string, int) (string, error) {
	return "", nil
}

func (p *coldStartProvider) CleanupFunctionNetwork(context.Context, string, string) error {
	p.mu.Lock()
	p.cleanupCalls++
	p.mu.Unlock()
	return nil
}

func (p *coldStartProvider) HealthCheck(context.Context) error        { return nil }
func (p *coldStartProvider) CheckNetwork(context.Context) error       { return nil }
func (p *coldStartProvider) DockerClient() *client.Client             { return nil }
func (p *coldStartProvider) GetSecretManager() *secrets.SecretManager { return nil }
func (p *coldStartProvider) GetGatewayID() string                     { return "" }
func (p *coldStartProvider) CanConnectGateway() bool                  { return false }

func (p *coldStartProvider) mutationCalls() (deploy, update, remove, cleanup, scale int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.deployCalls, p.updateCalls, p.removeCalls, p.cleanupCalls, p.scaleCalls
}

type stubResolver struct{ policy scaletozero.Policy }

func (s stubResolver) PolicyFor(string) scaletozero.Policy { return s.policy }

// testLeaseSecret is the ISOLATED provider-side activity-lease HMAC secret the
// tests share between the (simulated) control plane and the gateway. It stands
// in for FAAS_ACTIVITY_LEASE_SECRET and is deliberately not the golden fixture
// secret, so the tests exercise a real deployment secret (CV-06).
const testLeaseSecret = "test-provider-lease-secret-cv06"

func newScaleTestGateway(store Store, provider Provider) (*Gateway, *scaletozero.GateRegistry, *scaletozero.LeaseRegistry) {
	return newScaleTestGatewayWithSecret(store, provider, testLeaseSecret)
}

func newScaleTestGatewayWithSecret(store Store, provider Provider, secret string) (*Gateway, *scaletozero.GateRegistry, *scaletozero.LeaseRegistry) {
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	gw := NewGateway(store, provider, &okRouter{}, logger, "docker-faas-net")
	gates := scaletozero.NewGateRegistry(nil)
	leases := scaletozero.NewLeaseRegistry(nil)
	gw.SetScaleToZero(gates, leases, stubResolver{scaletozero.Policy{Enabled: true, IdleDuration: 60 * time.Second}}, true, secret)
	return gw, gates, leases
}

// signedLeaseBody signs an activity-lease request with the test secret and
// returns its JSON wire bytes, mirroring what the AIDrivenMES control plane
// sends (EncodeActivityLeaseRequest).
func signedLeaseBody(t *testing.T, req faascontract.ActivityLeaseRequest) []byte {
	t.Helper()
	body, err := faascontract.EncodeActivityLeaseRequest(req, testLeaseSecret)
	if err != nil {
		t.Fatalf("encode signed lease request: %v", err)
	}
	return body
}

type okRouter struct{}

func (okRouter) RouteRequest(_ context.Context, _ string, _ *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader("ok")),
	}, nil
}

// TestConcurrentColdStartCreatesSingleContainer is SZ-02 at the gateway level:
// N simultaneous requests to a zero-replica function must trigger exactly ONE
// scale-from-zero; all callers wait on readiness and then route successfully.
func TestConcurrentColdStartCreatesSingleContainer(t *testing.T) {
	store := &syncStore{functions: map[string]*types.FunctionMetadata{
		"cold": {Name: "cold", Image: "example/cold:latest", Network: "docker-faas-net", Replicas: 0},
	}}
	provider := &coldStartProvider{scaleDelay: 40 * time.Millisecond}
	gw, _, _ := newScaleTestGateway(store, provider)

	const n = 20
	var wg sync.WaitGroup
	wg.Add(n)
	codes := make([]int, n)

	start := make(chan struct{})
	for i := 0; i < n; i++ {
		go func(idx int) {
			defer wg.Done()
			<-start
			req := httptest.NewRequest(http.MethodPost, "/function/cold", strings.NewReader("x"))
			req = mux.SetURLVars(req, map[string]string{"name": "cold"})
			rec := httptest.NewRecorder()
			gw.HandleInvokeFunction(rec, req)
			codes[idx] = rec.Code
		}(i)
	}
	close(start)
	wg.Wait()

	_, _, _, _, scaleCalls := provider.mutationCalls()
	if scaleCalls != 1 {
		t.Fatalf("expected exactly 1 scale-from-zero, got %d", scaleCalls)
	}
	for i, code := range codes {
		if code != http.StatusOK {
			t.Fatalf("request %d got status %d, want 200", i, code)
		}
	}
}

// TestConcurrentColdStartWithLateArrivalsCreatesSingleContainer is the CV-07 fix
// exercised through the HANDLER path (not just the gate unit): an initial burst
// of concurrent requests to a zero-replica function plus a wave of LATE arrivals
// that show up after the first request has been serviced must together trigger
// exactly ONE scale-from-zero. Before the fix, a late caller could elect a
// second cold-start leader; here the whole run must create a single container.
// The run is repeated so a lingering race would surface as a >1 scale count.
func TestConcurrentColdStartWithLateArrivalsCreatesSingleContainer(t *testing.T) {
	const iterations = 20
	for iter := 0; iter < iterations; iter++ {
		store := &syncStore{functions: map[string]*types.FunctionMetadata{
			"cold": {Name: "cold", Image: "example/cold:latest", Network: "docker-faas-net", Replicas: 0},
		}}
		provider := &coldStartProvider{scaleDelay: 20 * time.Millisecond}
		gw, _, _ := newScaleTestGateway(store, provider)

		invoke := func() int {
			req := httptest.NewRequest(http.MethodPost, "/function/cold", strings.NewReader("x"))
			req = mux.SetURLVars(req, map[string]string{"name": "cold"})
			rec := httptest.NewRecorder()
			gw.HandleInvokeFunction(rec, req)
			return rec.Code
		}

		const firstWave = 16
		const lateWave = 12
		codes := make([]int, 0, firstWave+lateWave)
		var codesMu sync.Mutex
		record := func(c int) {
			codesMu.Lock()
			codes = append(codes, c)
			codesMu.Unlock()
		}

		// First wave: concurrent burst against the zero-replica function.
		var firstWG sync.WaitGroup
		firstWG.Add(firstWave)
		start := make(chan struct{})
		for i := 0; i < firstWave; i++ {
			go func() {
				defer firstWG.Done()
				<-start
				record(invoke())
			}()
		}
		close(start)
		firstWG.Wait()

		// Late wave: arrives AFTER the first wave finished (replica now ready).
		// These callers must observe readiness and never elect a new leader.
		var lateWG sync.WaitGroup
		lateWG.Add(lateWave)
		lateStart := make(chan struct{})
		for i := 0; i < lateWave; i++ {
			go func() {
				defer lateWG.Done()
				<-lateStart
				record(invoke())
			}()
		}
		close(lateStart)
		lateWG.Wait()

		if _, _, _, _, scaleCalls := provider.mutationCalls(); scaleCalls != 1 {
			t.Fatalf("iter %d: expected exactly 1 scale-from-zero across concurrent + late arrivals, got %d", iter, scaleCalls)
		}
		for i, code := range codes {
			if code != http.StatusOK {
				t.Fatalf("iter %d: request %d got status %d, want 200", iter, i, code)
			}
		}
	}
}

// TestActivityLeaseCombinesDurableAndGatewayWork is SZ-12: the endpoint accepts
// the control plane's durable counts and combines them with the provider's own
// gateway in-flight accounting to produce the decision.
func TestActivityLeaseCombinesDurableAndGatewayWork(t *testing.T) {
	store := &syncStore{functions: map[string]*types.FunctionMetadata{
		"batch": {Name: "batch", Image: "img", Network: "n", Replicas: 1},
	}}
	provider := &coldStartProvider{scaled: true} // one running container
	gw, gates, _ := newScaleTestGateway(store, provider)

	// A gateway HTTP request is in flight in addition to durable work.
	tok := gates.BeginInvocation("batch")
	defer tok.Release()

	body := signedLeaseBody(t, faascontract.ActivityLeaseRequest{
		ContractVersion: faascontract.ContractVersion,
		Function:        "batch",
		Generation:      0,
		Queued:          2,
		Running:         1,
		LeaseTTLSeconds: 60,
	})
	req := httptest.NewRequest(http.MethodPost, "/system/scale/activity-lease", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	gw.HandleActivityLease(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp faascontract.ActivityLeaseResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	// The response must verify under the SAME shared secret (CV-06 interop).
	if err := faascontract.VerifyActivityLeaseResponse(resp, testLeaseSecret); err != nil {
		t.Fatalf("response must verify under the shared secret: %v", err)
	}
	if !resp.Accepted {
		t.Fatalf("lease under live generation should be accepted")
	}
	if resp.DurableInFlight != 3 {
		t.Fatalf("durable in-flight = %d, want 3", resp.DurableInFlight)
	}
	if resp.GatewayInFlight != 1 {
		t.Fatalf("gateway in-flight = %d, want 1", resp.GatewayInFlight)
	}
	if resp.TotalInFlight != 4 {
		t.Fatalf("total in-flight = %d, want 4", resp.TotalInFlight)
	}
	if resp.Decision != faascontract.DecisionHold {
		t.Fatalf("decision = %q, want hold", resp.Decision)
	}
	if !resp.IdleScaleToZeroSupported {
		t.Fatalf("response must advertise idle scale-to-zero support")
	}
}

// TestActivityLeaseRejectsIncompatibleVersion is SZ-12: a contract major-version
// mismatch fails readiness (409) rather than silently disabling scale.
func TestActivityLeaseRejectsIncompatibleVersion(t *testing.T) {
	store := &syncStore{functions: map[string]*types.FunctionMetadata{}}
	gw, _, _ := newScaleTestGateway(store, &coldStartProvider{})

	body := []byte(`{"contract_version":"2.0.0","function":"f","generation":1}`)
	req := httptest.NewRequest(http.MethodPost, "/system/scale/activity-lease", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	gw.HandleActivityLease(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 for version mismatch, got %d", rec.Code)
	}
}

// TestActivityLeaseGrantsNoDockerControl is the SZ-06 no-host-control assertion:
// posting an activity lease (the ONLY new control-plane-facing endpoint) never
// triggers any container-mutating provider operation. The lease endpoint
// accepts data and returns a decision; it issues no Docker commands.
func TestActivityLeaseGrantsNoDockerControl(t *testing.T) {
	store := &syncStore{functions: map[string]*types.FunctionMetadata{
		"svc": {Name: "svc", Image: "img", Network: "n", Replicas: 1},
	}}
	provider := &coldStartProvider{scaled: true}
	gw, _, _ := newScaleTestGateway(store, provider)

	body := signedLeaseBody(t, faascontract.ActivityLeaseRequest{
		ContractVersion: faascontract.ContractVersion,
		Function:        "svc",
		Generation:      0,
		Running:         1,
		LeaseTTLSeconds: 60,
	})
	req := httptest.NewRequest(http.MethodPost, "/system/scale/activity-lease", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	gw.HandleActivityLease(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	deploy, update, remove, cleanup, scale := provider.mutationCalls()
	if deploy+update+remove+cleanup+scale != 0 {
		t.Fatalf("activity-lease must not mutate containers: deploy=%d update=%d remove=%d cleanup=%d scale=%d",
			deploy, update, remove, cleanup, scale)
	}
}

// TestActivityLeaseRejectsUnsignedRequest is CV-06: a structurally valid but
// UNSIGNED lease is rejected with 401 BEFORE any lease state is applied, and
// issues zero Docker commands. This proves the endpoint no longer accepts
// unauthenticated leases (leases.Apply is never reached) and preserves the
// no-host-control boundary on a rejected request.
func TestActivityLeaseRejectsUnsignedRequest(t *testing.T) {
	store := &syncStore{functions: map[string]*types.FunctionMetadata{
		"svc": {Name: "svc", Image: "img", Network: "n", Replicas: 1},
	}}
	provider := &coldStartProvider{scaled: true}
	gw, gates, leases := newScaleTestGateway(store, provider)

	// Unsigned wire bytes (no HMAC), version valid.
	body, _ := json.Marshal(faascontract.ActivityLeaseRequest{
		ContractVersion: faascontract.ContractVersion,
		Function:        "svc",
		Generation:      0,
		Running:         1,
		LeaseTTLSeconds: 60,
	})
	req := httptest.NewRequest(http.MethodPost, "/system/scale/activity-lease", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	gw.HandleActivityLease(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unsigned lease must be rejected with 401, got %d: %s", rec.Code, rec.Body.String())
	}
	// Apply must NOT have been called: no lease is stored for the function.
	if view := leases.View("svc", gates.Generation("svc")); view.Present {
		t.Fatalf("unsigned lease must not reach leases.Apply (no lease should be stored)")
	}
	deploy, update, remove, cleanup, scale := provider.mutationCalls()
	if deploy+update+remove+cleanup+scale != 0 {
		t.Fatalf("rejected lease must issue zero Docker commands: deploy=%d update=%d remove=%d cleanup=%d scale=%d",
			deploy, update, remove, cleanup, scale)
	}
}

// TestActivityLeaseRejectsWrongSecret is CV-06: a request signed with a secret
// other than the provider's isolated secret is rejected with 401 and does not
// reach Apply.
func TestActivityLeaseRejectsWrongSecret(t *testing.T) {
	store := &syncStore{functions: map[string]*types.FunctionMetadata{
		"svc": {Name: "svc", Image: "img", Network: "n", Replicas: 1},
	}}
	provider := &coldStartProvider{scaled: true}
	gw, gates, leases := newScaleTestGateway(store, provider)

	// Signed with a DIFFERENT secret than the gateway holds.
	body, err := faascontract.EncodeActivityLeaseRequest(faascontract.ActivityLeaseRequest{
		ContractVersion: faascontract.ContractVersion,
		Function:        "svc",
		Generation:      0,
		Running:         1,
		LeaseTTLSeconds: 60,
	}, "an-attacker-secret-not-the-providers")
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/system/scale/activity-lease", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	gw.HandleActivityLease(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong-secret lease must be rejected with 401, got %d", rec.Code)
	}
	if view := leases.View("svc", gates.Generation("svc")); view.Present {
		t.Fatalf("wrong-secret lease must not reach leases.Apply")
	}
	if deploy, update, remove, cleanup, scale := provider.mutationCalls(); deploy+update+remove+cleanup+scale != 0 {
		t.Fatalf("rejected lease must issue zero Docker commands")
	}
}

// TestActivityLeaseRejectsTamperedField is CV-06: mutating any signed field
// after signing invalidates the HMAC, so the request is rejected with 401.
func TestActivityLeaseRejectsTamperedField(t *testing.T) {
	store := &syncStore{functions: map[string]*types.FunctionMetadata{
		"svc": {Name: "svc", Image: "img", Network: "n", Replicas: 1},
	}}
	provider := &coldStartProvider{scaled: true}
	gw, gates, leases := newScaleTestGateway(store, provider)

	// Sign a request, then tamper with the counts/generation after signing.
	signed := faascontract.SignActivityLeaseRequest(faascontract.ActivityLeaseRequest{
		ContractVersion: faascontract.ContractVersion,
		Function:        "svc",
		Generation:      1,
		Running:         1,
		LeaseTTLSeconds: 60,
	}, testLeaseSecret)
	signed.Running = 999  // tamper with in-flight count
	signed.Generation = 7 // tamper with the fence
	body, _ := json.Marshal(signed)

	req := httptest.NewRequest(http.MethodPost, "/system/scale/activity-lease", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	gw.HandleActivityLease(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("tampered lease must be rejected with 401, got %d", rec.Code)
	}
	if view := leases.View("svc", gates.Generation("svc")); view.Present {
		t.Fatalf("tampered lease must not reach leases.Apply")
	}
	if deploy, update, remove, cleanup, scale := provider.mutationCalls(); deploy+update+remove+cleanup+scale != 0 {
		t.Fatalf("rejected lease must issue zero Docker commands")
	}
}

// TestActivityLeaseRejectsWhenSecretUnconfigured is CV-06 fail-closed: if the
// provider has no isolated secret, the endpoint refuses (503) even a correctly
// structured request, rather than falling open to accept unsigned leases.
func TestActivityLeaseRejectsWhenSecretUnconfigured(t *testing.T) {
	store := &syncStore{functions: map[string]*types.FunctionMetadata{
		"svc": {Name: "svc", Image: "img", Network: "n", Replicas: 1},
	}}
	provider := &coldStartProvider{scaled: true}
	gw, _, leases := newScaleTestGatewayWithSecret(store, provider, "") // no secret

	// Even a request signed with the empty string must not be honored.
	body, _ := faascontract.EncodeActivityLeaseRequest(faascontract.ActivityLeaseRequest{
		ContractVersion: faascontract.ContractVersion,
		Function:        "svc",
		Generation:      0,
		Running:         1,
		LeaseTTLSeconds: 60,
	}, "")
	req := httptest.NewRequest(http.MethodPost, "/system/scale/activity-lease", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	gw.HandleActivityLease(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("unconfigured secret must fail closed with 503, got %d", rec.Code)
	}
	if view := leases.View("svc", 0); view.Present {
		t.Fatalf("unconfigured endpoint must not reach leases.Apply")
	}
	if deploy, update, remove, cleanup, scale := provider.mutationCalls(); deploy+update+remove+cleanup+scale != 0 {
		t.Fatalf("unconfigured endpoint must issue zero Docker commands")
	}
}

// TestActivityLeaseAcceptsSignedAndResponseVerifies is CV-06 interop: a
// correctly-signed request is accepted (200), its lease is applied, and the
// response verifies under the SAME shared secret. This is the positive
// end-to-end contract the AIDrivenMES provider-client depends on.
func TestActivityLeaseAcceptsSignedAndResponseVerifies(t *testing.T) {
	store := &syncStore{functions: map[string]*types.FunctionMetadata{
		"report": {Name: "report", Image: "img", Network: "n", Replicas: 1},
	}}
	provider := &coldStartProvider{scaled: true}
	gw, gates, leases := newScaleTestGateway(store, provider)

	body := signedLeaseBody(t, faascontract.ActivityLeaseRequest{
		ContractVersion: faascontract.ContractVersion,
		Function:        "report",
		Generation:      0,
		Queued:          2,
		Running:         1,
		LeaseTTLSeconds: 60,
	})
	req := httptest.NewRequest(http.MethodPost, "/system/scale/activity-lease", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	gw.HandleActivityLease(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("signed lease must be accepted with 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp faascontract.ActivityLeaseResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if err := faascontract.VerifyActivityLeaseResponse(resp, testLeaseSecret); err != nil {
		t.Fatalf("signed response must verify under the shared secret: %v", err)
	}
	if resp.Signature == "" {
		t.Fatalf("response must carry a signature")
	}
	// The accepted lease reached Apply: durable state is now recorded.
	if view := leases.View("report", gates.Generation("report")); !view.Present || view.DurableInFlight != 3 {
		t.Fatalf("accepted lease must be applied: present=%v durable=%d (want present, 3)", view.Present, view.DurableInFlight)
	}
}

// TestScaleCapabilitiesKeepsKubernetesUnclaimed is SZ-05/SZ-10: the provider
// advertises Docker idle scale-to-zero and explicitly does NOT claim Kubernetes.
func TestScaleCapabilitiesKeepsKubernetesUnclaimed(t *testing.T) {
	gw, _, _ := newScaleTestGateway(&syncStore{functions: map[string]*types.FunctionMetadata{}}, &coldStartProvider{})

	req := httptest.NewRequest(http.MethodGet, "/system/scale/capabilities", nil)
	rec := httptest.NewRecorder()
	gw.HandleScaleCapabilities(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var caps faascontract.Capabilities
	if err := json.Unmarshal(rec.Body.Bytes(), &caps); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !caps.IdleScaleToZero {
		t.Fatalf("must advertise idle_scale_to_zero=true")
	}
	if caps.Kubernetes.Supported {
		t.Fatalf("Kubernetes must remain unclaimed")
	}
}
