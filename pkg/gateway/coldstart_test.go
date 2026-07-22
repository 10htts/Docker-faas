package gateway

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"

	"github.com/docker/docker/client"
	"github.com/sirupsen/logrus"

	"github.com/docker-faas/docker-faas/pkg/scaletozero"
	"github.com/docker-faas/docker-faas/pkg/secrets"
	"github.com/docker-faas/docker-faas/pkg/types"
)

// Self-contained fakes (cs* prefix) so this file never collides with fakes in
// other gateway test files.

type csFakeStore struct {
	mu       sync.Mutex
	fn       *types.FunctionMetadata
	replicas int
}

func (s *csFakeStore) ListFunctions() ([]*types.FunctionMetadata, error) {
	return []*types.FunctionMetadata{s.fn}, nil
}
func (s *csFakeStore) GetFunction(name string) (*types.FunctionMetadata, error) {
	if s.fn != nil && s.fn.Name == name {
		return s.fn, nil
	}
	return nil, errors.New("not found")
}
func (s *csFakeStore) CreateFunction(m *types.FunctionMetadata) error { return nil }
func (s *csFakeStore) UpdateFunction(m *types.FunctionMetadata) error { return nil }
func (s *csFakeStore) DeleteFunction(name string) error               { return nil }
func (s *csFakeStore) UpdateReplicas(name string, replicas int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.replicas = replicas
	return nil
}
func (s *csFakeStore) HealthCheck(ctx context.Context) error { return nil }

// csFakeProvider simulates container state: scaling succeeds or fails per an
// injected script, and GetFunctionContainers reflects the current "running"
// state.
type csFakeProvider struct {
	mu         sync.Mutex
	running    bool
	scaleCalls int
	scaleErrs  []error // consumed per ScaleFunction call; nil entry = success
}

func (p *csFakeProvider) scriptedScale() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	var err error
	if len(p.scaleErrs) > 0 {
		err = p.scaleErrs[0]
		p.scaleErrs = p.scaleErrs[1:]
	}
	p.scaleCalls++
	if err == nil {
		p.running = true
	}
	return err
}

func (p *csFakeProvider) DeployFunction(ctx context.Context, d *types.FunctionDeployment, replicas int) error {
	return p.scriptedScale()
}
func (p *csFakeProvider) UpdateFunction(ctx context.Context, d *types.FunctionDeployment, replicas int) error {
	return nil
}
func (p *csFakeProvider) RemoveFunction(ctx context.Context, functionName string) error { return nil }
func (p *csFakeProvider) ScaleFunction(ctx context.Context, d *types.FunctionDeployment, targetReplicas int) error {
	return p.scriptedScale()
}
func (p *csFakeProvider) GetFunctionContainers(ctx context.Context, functionName string) ([]*types.Container, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.running {
		return nil, nil
	}
	return []*types.Container{{ID: "c1", Name: functionName + "-1", Status: "running"}}, nil
}
func (p *csFakeProvider) GetContainerLogs(ctx context.Context, functionName string, tail int) (string, error) {
	return "", nil
}
func (p *csFakeProvider) CleanupFunctionNetwork(ctx context.Context, functionName, networkName string) error {
	return nil
}
func (p *csFakeProvider) HealthCheck(ctx context.Context) error    { return nil }
func (p *csFakeProvider) CheckNetwork(ctx context.Context) error   { return nil }
func (p *csFakeProvider) DockerClient() *client.Client             { return nil }
func (p *csFakeProvider) GetSecretManager() *secrets.SecretManager { return nil }
func (p *csFakeProvider) GetGatewayID() string                     { return "" }
func (p *csFakeProvider) CanConnectGateway() bool                  { return false }

type csFakeRouter struct{}

func (csFakeRouter) RouteRequest(ctx context.Context, functionName string, req *http.Request) (*http.Response, error) {
	return nil, errors.New("not routed in this test")
}

type csFakePolicies struct{}

func (csFakePolicies) PolicyFor(fn string) scaletozero.Policy { return scaletozero.Policy{} }

func newColdStartTestGateway(t *testing.T, p *csFakeProvider) (*Gateway, *scaletozero.GateRegistry) {
	t.Helper()
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	st := &csFakeStore{fn: &types.FunctionMetadata{Name: "fn", Image: "img", Replicas: 1}}
	gw := NewGateway(st, p, csFakeRouter{}, logger, "testnet")
	gates := scaletozero.NewGateRegistry(nil)
	leases := scaletozero.NewLeaseRegistry(nil)
	gw.SetScaleToZero(gates, leases, csFakePolicies{}, true, "coldstart-test-secret")
	return gw, gates
}

// TestEnsureReadyFromZeroLeaderFailureRetries: the leader's scale-from-zero
// fails once; ensureReadyFromZero must retry (fresh leader election) and
// succeed on the second attempt (RT-213 failed-start retry).
func TestEnsureReadyFromZeroLeaderFailureRetries(t *testing.T) {
	p := &csFakeProvider{scaleErrs: []error{errors.New("first scale fails")}}
	gw, gates := newColdStartTestGateway(t, p)

	fn := &types.FunctionMetadata{Name: "fn", Image: "img"}
	if err := gw.ensureReadyFromZero(context.Background(), fn); err != nil {
		t.Fatalf("expected retry to succeed, got %v", err)
	}

	p.mu.Lock()
	calls := p.scaleCalls
	p.mu.Unlock()
	if calls != 2 {
		t.Fatalf("expected exactly 2 scale attempts (fail then retry), got %d", calls)
	}
	if gates.Generation("fn") != 2 {
		t.Fatalf("expected generation 2 (one per leader election), got %d", gates.Generation("fn"))
	}
	if !gates.Ready("fn") {
		t.Fatalf("gate must be ready after successful retry")
	}
}

// TestEnsureReadyFromZeroCrashStaleReadyRecovers: the gate believes the
// function ready but the container died outside a reclaim. ensureReadyFromZero
// must demote the stale readiness under the generation fence and cold-start a
// fresh replica (RT-213 crash recovery).
func TestEnsureReadyFromZeroCrashStaleReadyRecovers(t *testing.T) {
	p := &csFakeProvider{}
	gw, gates := newColdStartTestGateway(t, p)

	// Establish readiness, then kill the container behind the gate's back.
	seed := gates.AcquireColdStart("fn")
	if !seed.Leader {
		t.Fatalf("seed acquire must lead")
	}
	seed.Complete(nil)
	p.mu.Lock()
	p.running = false // crash: no reclaim involved
	p.mu.Unlock()
	genStale := gates.Generation("fn")

	fn := &types.FunctionMetadata{Name: "fn", Image: "img"}
	if err := gw.ensureReadyFromZero(context.Background(), fn); err != nil {
		t.Fatalf("crash recovery must succeed, got %v", err)
	}

	p.mu.Lock()
	calls := p.scaleCalls
	running := p.running
	p.mu.Unlock()
	if calls != 1 {
		t.Fatalf("expected exactly 1 recovery scale call, got %d", calls)
	}
	if !running {
		t.Fatalf("recovery must have started a replica")
	}
	if got := gates.Generation("fn"); got != genStale+1 {
		t.Fatalf("recovery must advance the generation %d -> %d, got %d", genStale, genStale+1, got)
	}
	if !gates.Ready("fn") {
		t.Fatalf("gate must be ready after recovery")
	}
}

// TestEnsureReadyFromZeroFollowerHonorsCancellation: a follower whose own
// context is cancelled returns the context error without retrying and without
// issuing any scale calls.
func TestEnsureReadyFromZeroFollowerHonorsCancellation(t *testing.T) {
	p := &csFakeProvider{}
	gw, gates := newColdStartTestGateway(t, p)

	// Hold the leader op open so the follower parks on Wait.
	leader := gates.AcquireColdStart("fn")
	if !leader.Leader {
		t.Fatalf("expected leader")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancelled before/while waiting — deterministic either way

	fn := &types.FunctionMetadata{Name: "fn", Image: "img"}
	err := gw.ensureReadyFromZero(ctx, fn)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

	p.mu.Lock()
	calls := p.scaleCalls
	p.mu.Unlock()
	if calls != 0 {
		t.Fatalf("cancelled follower must not scale, got %d calls", calls)
	}
	leader.Complete(nil)
}
