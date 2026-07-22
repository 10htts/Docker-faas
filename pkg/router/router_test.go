package router

import (
	"io"
	"sync"
	"testing"

	"github.com/docker-faas/docker-faas/pkg/types"
	"github.com/sirupsen/logrus"
)

func testRouter() *Router {
	l := logrus.New()
	l.SetOutput(io.Discard)
	return &Router{logger: l, roundRobin: make(map[string]*uint64)}
}

// TestSelectContainerConcurrentFirstRequestsNoRace hammers selectContainer with
// concurrent first-requests to many functions. Before the fix, the unsynchronized
// get-or-create of the roundRobin map was a `fatal error: concurrent map writes`
// that crashed the process; this test run under `-race` (and even without it,
// via the fatal map-write detector) proves the map access is now synchronized.
func TestSelectContainerConcurrentFirstRequestsNoRace(t *testing.T) {
	r := testRouter()
	containers := []*types.Container{
		{ID: "a", Name: "fn-0", Status: "running", IPAddress: "10.0.0.2"},
		{ID: "b", Name: "fn-1", Status: "running", IPAddress: "10.0.0.3"},
	}

	const goroutines = 64
	const iterations = 200
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(g int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				// Mix same-function contention (map read/inc) with new-function
				// creation (map write) to exercise both the get and create paths.
				fn := "shared"
				if i%5 == 0 {
					fn = "fn-" + string(rune('A'+(g%16)))
				}
				if got := r.selectContainer(fn, containers); got == nil {
					t.Errorf("selectContainer returned nil")
					return
				}
			}
		}(g)
	}
	wg.Wait()
}

// TestCounterForStableIdentity: counterFor returns the SAME counter pointer for a
// function across calls (round-robin state persists) and distinct pointers for
// distinct functions.
func TestCounterForStableIdentity(t *testing.T) {
	r := testRouter()
	a1 := r.counterFor("a")
	a2 := r.counterFor("a")
	b1 := r.counterFor("b")
	if a1 != a2 {
		t.Fatal("counterFor must return a stable pointer per function")
	}
	if a1 == b1 {
		t.Fatal("distinct functions must have distinct counters")
	}
}

// TestSelectContainerPrefersRunning: only running containers are chosen when any
// exist; the round-robin index stays in range.
func TestSelectContainerPrefersRunning(t *testing.T) {
	r := testRouter()
	containers := []*types.Container{
		{ID: "a", Name: "fn-0", Status: "exited"},
		{ID: "b", Name: "fn-1", Status: "running"},
	}
	for i := 0; i < 10; i++ {
		got := r.selectContainer("fn", containers)
		if got.Status != "running" {
			t.Fatalf("must select a running container, got %q", got.Status)
		}
	}
}
