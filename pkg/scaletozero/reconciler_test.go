package scaletozero

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/docker-faas/docker-faas/pkg/faascontract"
	"github.com/sirupsen/logrus"
	logrustest "github.com/sirupsen/logrus/hooks/test"
)

// --- test doubles ---

type fakeController struct {
	observed      map[string]int
	observedFns   []string
	reclaimReport map[string]ReclaimReport
	reclaimErr    map[string]error

	reclaimed []string
	warmCalls map[string]int
}

func newFakeController() *fakeController {
	return &fakeController{
		observed:      map[string]int{},
		reclaimReport: map[string]ReclaimReport{},
		reclaimErr:    map[string]error{},
		warmCalls:     map[string]int{},
	}
}

func (f *fakeController) ObservedReplicas(_ context.Context, fn string) (int, error) {
	return f.observed[fn], nil
}

func (f *fakeController) ObservedFunctions(_ context.Context) ([]string, error) {
	return f.observedFns, nil
}

func (f *fakeController) ReclaimToZero(_ context.Context, fn string) (ReclaimReport, error) {
	if err := f.reclaimErr[fn]; err != nil {
		return ReclaimReport{}, err
	}
	f.reclaimed = append(f.reclaimed, fn)
	f.observed[fn] = 0
	return f.reclaimReport[fn], nil
}

func (f *fakeController) EnsureWarmMinimum(_ context.Context, fn string, minReplicas int) error {
	f.warmCalls[fn] = minReplicas
	f.observed[fn] = minReplicas
	return nil
}

func (f *fakeController) reclaimedCount(fn string) int {
	n := 0
	for _, r := range f.reclaimed {
		if r == fn {
			n++
		}
	}
	return n
}

type fakePolicies struct{ fns []DeclaredFunction }

func (f fakePolicies) DeclaredFunctions(context.Context) ([]DeclaredFunction, error) {
	return f.fns, nil
}

type recordingMetrics struct {
	reclamations map[string]ReclaimReport
	staleCleaned map[string]int
	observed     map[string]int
	decisions    map[string]Action
	passes       int
}

func newRecordingMetrics() *recordingMetrics {
	return &recordingMetrics{
		reclamations: map[string]ReclaimReport{},
		staleCleaned: map[string]int{},
		observed:     map[string]int{},
		decisions:    map[string]Action{},
	}
}

func (m *recordingMetrics) ObservedReplicas(fn string, replicas int)     { m.observed[fn] = replicas }
func (m *recordingMetrics) Decision(fn string, action Action)            { m.decisions[fn] = action }
func (m *recordingMetrics) IdleReclamation(fn string, r ReclaimReport)   { m.reclamations[fn] = r }
func (m *recordingMetrics) StaleGenerationsCleaned(fn string, count int) { m.staleCleaned[fn] += count }
func (m *recordingMetrics) ReconcilePass(int, int, int, int, int)        { m.passes++ }

func testLogger() *logrus.Logger {
	l := logrus.New()
	l.SetOutput(io.Discard)
	return l
}

var (
	t0 = time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	t1 = t0.Add(2 * time.Hour)
)

func newTestReconciler(ctrl ReplicaController, policies PolicySource, gates *GateRegistry, leases *LeaseRegistry, m MetricsSink) *IdleReconciler {
	return NewIdleReconciler(ReconcilerConfig{
		Controller: ctrl,
		Policies:   policies,
		Gates:      gates,
		Leases:     leases,
		Metrics:    m,
		Logger:     testLogger(),
		Clock:      func() time.Time { return t1 },
	})
}

func enabledPolicy() Policy {
	return Policy{Enabled: true, IdleDuration: 60 * time.Second}
}

// TestReconcileReclaimsIdleFunction: an idle function scales to provider-observed
// ZERO replicas and its reclaimed resources are recorded (SZ-09).
func TestReconcileReclaimsIdleFunction(t *testing.T) {
	ctrl := newFakeController()
	ctrl.observed["report"] = 1
	ctrl.reclaimReport["report"] = ReclaimReport{ContainersRemoved: 1, NetworksRemoved: 1, MemoryBytesFreed: 128 * 1024 * 1024, NanoCPUsFreed: 500_000_000}

	gates := NewGateRegistry(func() time.Time { return t0 })
	gates.MarkActivity("report", t0) // last activity 2h before reconcile time
	leases := NewLeaseRegistry(func() time.Time { return t1 })
	m := newRecordingMetrics()

	rec := newTestReconciler(ctrl, fakePolicies{[]DeclaredFunction{{Name: "report", Policy: enabledPolicy()}}}, gates, leases, m)

	res, err := rec.ReconcileOnce(context.Background())
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if res.Reclaimed != 1 {
		t.Fatalf("expected 1 reclaimed, got %+v", res)
	}
	if ctrl.observed["report"] != 0 {
		t.Fatalf("function must be at zero observed replicas, got %d", ctrl.observed["report"])
	}
	rep, ok := m.reclamations["report"]
	if !ok {
		t.Fatalf("reclamation metric not recorded")
	}
	if rep.MemoryBytesFreed != 128*1024*1024 || rep.NanoCPUsFreed != 500_000_000 || rep.ContainersRemoved != 1 || rep.NetworksRemoved != 1 {
		t.Fatalf("reclaimed-resource metrics not recorded correctly: %+v", rep)
	}
	if m.observed["report"] != 0 {
		t.Fatalf("observed-replicas gauge must reach 0, got %d", m.observed["report"])
	}
}

// TestReconcileNeverReapsInFlightGatewayWork is the load-bearing in-flight
// safety test (SZ-01): a long-running invocation counted in-flight is never
// reaped, even though the idle window has elapsed.
func TestReconcileNeverReapsInFlightGatewayWork(t *testing.T) {
	ctrl := newFakeController()
	ctrl.observed["longjob"] = 1

	gates := NewGateRegistry(func() time.Time { return t0 })
	gates.MarkActivity("longjob", t0)
	// A synchronous invocation began 2h ago and is STILL executing.
	tok := gates.BeginInvocation("longjob")
	defer tok.Release()

	leases := NewLeaseRegistry(func() time.Time { return t1 })
	m := newRecordingMetrics()

	rec := newTestReconciler(ctrl, fakePolicies{[]DeclaredFunction{{Name: "longjob", Policy: enabledPolicy()}}}, gates, leases, m)

	res, err := rec.ReconcileOnce(context.Background())
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if res.Reclaimed != 0 {
		t.Fatalf("in-flight function must NOT be reclaimed, got %+v", res)
	}
	if ctrl.reclaimedCount("longjob") != 0 {
		t.Fatalf("ReclaimToZero must not be called for an in-flight function")
	}
	if m.decisions["longjob"] != ActionHold {
		t.Fatalf("decision must be Hold, got %v", m.decisions["longjob"])
	}
}

// TestReconcileNeverReapsDurableWork (SZ-03): queued/running durable jobs that
// AIDrivenMES reports via a lease protect the function even with no gateway
// HTTP traffic.
func TestReconcileNeverReapsDurableWork(t *testing.T) {
	ctrl := newFakeController()
	ctrl.observed["batch"] = 1

	gates := NewGateRegistry(func() time.Time { return t0 })
	gates.MarkActivity("batch", t0)
	leases := NewLeaseRegistry(func() time.Time { return t1 })
	// A queued+running durable lease under the live generation (0).
	leases.Apply(faascontract.ActivityLeaseRequest{
		ContractVersion: faascontract.ContractVersion,
		Function:        "batch",
		Generation:      0,
		Queued:          3,
		Running:         2,
		LeaseTTLSeconds: 300,
	})

	m := newRecordingMetrics()
	rec := newTestReconciler(ctrl, fakePolicies{[]DeclaredFunction{{Name: "batch", Policy: enabledPolicy()}}}, gates, leases, m)

	res, err := rec.ReconcileOnce(context.Background())
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if res.Reclaimed != 0 || ctrl.reclaimedCount("batch") != 0 {
		t.Fatalf("durable work must protect the function from reclamation, got %+v", res)
	}
}

// TestReconcileKeepsWarmMinimum (SZ-04): a pinned function stays at its warm
// minimum and is never reaped.
func TestReconcileKeepsWarmMinimum(t *testing.T) {
	ctrl := newFakeController()
	ctrl.observed["critical"] = 0 // below warm minimum

	gates := NewGateRegistry(func() time.Time { return t0 })
	gates.MarkActivity("critical", t0)
	leases := NewLeaseRegistry(func() time.Time { return t1 })
	m := newRecordingMetrics()

	policy := Policy{Enabled: true, IdleDuration: 60 * time.Second, MinReplicas: 2}
	rec := newTestReconciler(ctrl, fakePolicies{[]DeclaredFunction{{Name: "critical", Policy: policy}}}, gates, leases, m)

	res, err := rec.ReconcileOnce(context.Background())
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if res.Reclaimed != 0 {
		t.Fatalf("warm-min function must not be reclaimed")
	}
	if ctrl.warmCalls["critical"] != 2 {
		t.Fatalf("expected EnsureWarmMinimum(critical, 2), got %v", ctrl.warmCalls)
	}
}

// TestReconcileRestartConvergenceCleansOrphans (SZ-07): declared vs actual is
// reconciled from the (reconstructable) policy source, and containers for
// undeclared functions are cleaned up along with their accounting state.
func TestReconcileRestartConvergenceCleansOrphans(t *testing.T) {
	ctrl := newFakeController()
	ctrl.observed["kept"] = 1
	ctrl.observed["orphan"] = 1
	ctrl.observedFns = []string{"kept", "orphan"}

	gates := NewGateRegistry(func() time.Time { return t0 })
	// "kept" is recently active so it holds; "orphan" has leftover state.
	gates.MarkActivity("kept", t1)
	gates.MarkActivity("orphan", t0)
	leases := NewLeaseRegistry(func() time.Time { return t1 })
	leases.Apply(faascontract.ActivityLeaseRequest{ContractVersion: faascontract.ContractVersion, Function: "orphan", Generation: 0, Running: 1, LeaseTTLSeconds: 300})
	m := newRecordingMetrics()

	// Only "kept" is declared; "orphan" is not.
	rec := newTestReconciler(ctrl, fakePolicies{[]DeclaredFunction{{Name: "kept", Policy: enabledPolicy()}}}, gates, leases, m)

	res, err := rec.ReconcileOnce(context.Background())
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if res.OrphansCleaned != 1 {
		t.Fatalf("expected 1 orphan cleaned, got %+v", res)
	}
	if ctrl.reclaimedCount("orphan") != 1 {
		t.Fatalf("orphan containers must be reclaimed")
	}
	if ctrl.reclaimedCount("kept") != 0 {
		t.Fatalf("declared, recently-active function must not be reclaimed")
	}
	// Orphan accounting state is forgotten (stale-generation cleanup).
	if m.staleCleaned["orphan"] == 0 {
		t.Fatalf("orphan stale generation cleanup must be recorded")
	}
	if got := leases.View("orphan", 0); got.Present {
		t.Fatalf("orphan lease must be forgotten after cleanup")
	}
}

// TestReconcileStaleGenerationLeaseCannotKillBusyContainer (SZ-08): a stale
// lease is ignored and cleaned; the busy container is protected by real
// in-flight accounting, not by the stale lease.
func TestReconcileStaleGenerationLeaseCannotKillBusyContainer(t *testing.T) {
	ctrl := newFakeController()
	ctrl.observed["svc"] = 1

	gates := NewGateRegistry(func() time.Time { return t0 })
	gates.MarkActivity("svc", t0)
	// A cold start bumped the generation to 1.
	cs := gates.AcquireColdStart("svc")
	cs.Complete(nil)
	// A real invocation is in flight against the fresh container.
	tok := gates.BeginInvocation("svc")
	defer tok.Release()

	leases := NewLeaseRegistry(func() time.Time { return t1 })
	// A stale lease sampled under generation 0 claims lots of running work.
	leases.Apply(faascontract.ActivityLeaseRequest{ContractVersion: faascontract.ContractVersion, Function: "svc", Generation: 0, Running: 99, LeaseTTLSeconds: 300})

	m := newRecordingMetrics()
	rec := newTestReconciler(ctrl, fakePolicies{[]DeclaredFunction{{Name: "svc", Policy: enabledPolicy()}}}, gates, leases, m)

	res, err := rec.ReconcileOnce(context.Background())
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if ctrl.reclaimedCount("svc") != 0 {
		t.Fatalf("busy container must not be reaped")
	}
	if res.Reclaimed != 0 {
		t.Fatalf("expected no reclamation, got %+v", res)
	}
	if m.staleCleaned["svc"] == 0 {
		t.Fatalf("stale-generation lease should be cleaned")
	}
	if leases.View("svc", gates.Generation("svc")).Present {
		t.Fatalf("stale-generation lease must be forgotten by the reconcile pass")
	}
}

// TestReconcileWarmMinimumClampedToMaxReplicas: when a policy declares
// MinReplicas > MaxReplicas, keep-warm must use MaxReplicas as the effective
// warm minimum and never scale past the cap.
func TestReconcileWarmMinimumClampedToMaxReplicas(t *testing.T) {
	ctrl := newFakeController()
	ctrl.observed["overpinned"] = 0 // below the effective minimum -> scale up to 2, not 5
	ctrl.observed["atcap"] = 3      // above the effective minimum -> leave alone

	gates := NewGateRegistry(func() time.Time { return t0 })
	gates.MarkActivity("overpinned", t0)
	gates.MarkActivity("atcap", t0)
	leases := NewLeaseRegistry(func() time.Time { return t1 })
	m := newRecordingMetrics()

	conflicted := Policy{Enabled: true, IdleDuration: 60 * time.Second, MinReplicas: 5, MaxReplicas: 2}
	rec := newTestReconciler(ctrl, fakePolicies{[]DeclaredFunction{
		{Name: "overpinned", Policy: conflicted},
		{Name: "atcap", Policy: conflicted},
	}}, gates, leases, m)

	res, err := rec.ReconcileOnce(context.Background())
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if res.KeptWarm != 2 {
		t.Fatalf("expected 2 kept warm, got %+v", res)
	}
	if got := ctrl.warmCalls["overpinned"]; got != 2 {
		t.Fatalf("EnsureWarmMinimum must be clamped to MaxReplicas: got %d, want 2", got)
	}
	if _, called := ctrl.warmCalls["atcap"]; called {
		t.Fatalf("function already at/above the effective minimum must not be scaled (would breach MaxReplicas)")
	}
	if ctrl.observed["overpinned"] != 2 {
		t.Fatalf("overpinned must be scaled to exactly the cap, got %d", ctrl.observed["overpinned"])
	}
}

// TestReconcileWarnsOnceWhenIdleWindowCloseToLeaseTTL: an enabled policy with
// IdleDuration below 3x the default lease TTL (90s) gets a one-line warning,
// rate-limited to once per function name per process, so a delayed control-plane
// lease renewal being mistaken for idleness is a visible misconfiguration.
func TestReconcileWarnsOnceWhenIdleWindowCloseToLeaseTTL(t *testing.T) {
	ctrl := newFakeController()
	ctrl.observed["short-a"] = 0
	ctrl.observed["short-b"] = 0
	ctrl.observed["long"] = 0

	gates := NewGateRegistry(func() time.Time { return t0 })
	leases := NewLeaseRegistry(func() time.Time { return t1 })

	logger, hook := logrustest.NewNullLogger()
	rec := NewIdleReconciler(ReconcilerConfig{
		Controller: ctrl,
		Policies: fakePolicies{[]DeclaredFunction{
			{Name: "short-a", Policy: Policy{Enabled: true, IdleDuration: 60 * time.Second}},
			{Name: "short-b", Policy: Policy{Enabled: true, IdleDuration: 30 * time.Second}},
			{Name: "long", Policy: Policy{Enabled: true, IdleDuration: 3 * DefaultLeaseTTL}}, // exactly 3x TTL: compliant
		}},
		Gates:  gates,
		Leases: leases,
		Logger: logger,
		Clock:  func() time.Time { return t1 },
	})

	countWindowWarnings := func(fn string) int {
		n := 0
		for _, e := range hook.AllEntries() {
			if e.Level == logrus.WarnLevel && strings.Contains(e.Message, "idle window") && strings.Contains(e.Message, fn) {
				n++
			}
		}
		return n
	}

	for pass := 0; pass < 3; pass++ {
		if _, err := rec.ReconcileOnce(context.Background()); err != nil {
			t.Fatalf("reconcile pass %d: %v", pass, err)
		}
	}

	if got := countWindowWarnings("short-a"); got != 1 {
		t.Fatalf("short-a idle-window warning count = %d, want exactly 1 across repeated passes", got)
	}
	if got := countWindowWarnings("short-b"); got != 1 {
		t.Fatalf("short-b idle-window warning count = %d, want exactly 1 across repeated passes", got)
	}
	if got := countWindowWarnings("long"); got != 0 {
		t.Fatalf("compliant policy (IdleDuration >= 3x lease TTL) must not warn, got %d warnings", got)
	}
}
