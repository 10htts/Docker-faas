package provider

// Label-scoped reclaim proof for the idle scaler (scale-to-zero red team).
//
// The Docker provider must only ever OBSERVE and RECLAIM containers that carry
// the function labels (LabelType=function / LabelFunction=<name>). There is no
// interface seam for the Docker client, so these tests inject a fake in-process
// daemon at the HTTP transport layer (client.WithHTTPClient): the REAL
// ObservedReplicas / ObservedFunctions / ReclaimToZero code paths run
// end-to-end, the fake records the exact label filters sent on the wire, and it
// emulates daemon-side label filtering honestly — so if the provider ever
// listed without the right label filter, the unlabeled bystander containers
// seeded in each world would leak into the results and fail the assertions.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/sirupsen/logrus"
)

// --- fake wire-level docker daemon ---

type daemonRequest struct {
	method string
	path   string // API-version prefix stripped
	query  url.Values
}

type fakeDaemonContainer struct {
	summary  container.Summary
	memory   int64
	nanoCPUs int64
}

type fakeDaemon struct {
	t  *testing.T
	mu sync.Mutex

	containers map[string]fakeDaemonContainer // by ID
	networks   map[string]network.Inspect     // by name

	requests        []daemonRequest
	listFilters     []filters.Args // parsed filters of every /containers/json call
	stopped         []string
	removed         []string
	removedNetworks []string
	failRemove      map[string]bool
}

func newFakeDaemon(t *testing.T) *fakeDaemon {
	return &fakeDaemon{
		t:          t,
		containers: map[string]fakeDaemonContainer{},
		networks:   map[string]network.Inspect{},
		failRemove: map[string]bool{},
	}
}

func (d *fakeDaemon) RoundTrip(req *http.Request) (*http.Response, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	path := req.URL.Path
	if strings.HasPrefix(path, "/v1.") {
		if i := strings.Index(path[1:], "/"); i >= 0 {
			path = path[1+i:]
		}
	}
	d.requests = append(d.requests, daemonRequest{method: req.Method, path: path, query: req.URL.Query()})

	switch {
	case req.Method == http.MethodGet && path == "/containers/json":
		return d.handleContainerList(req)
	case req.Method == http.MethodGet && strings.HasPrefix(path, "/containers/") && strings.HasSuffix(path, "/json"):
		return d.handleContainerInspect(strings.TrimSuffix(strings.TrimPrefix(path, "/containers/"), "/json"))
	case req.Method == http.MethodPost && strings.HasPrefix(path, "/containers/") && strings.HasSuffix(path, "/stop"):
		id := strings.TrimSuffix(strings.TrimPrefix(path, "/containers/"), "/stop")
		d.stopped = append(d.stopped, id)
		return jsonResponse(http.StatusNoContent, nil)
	case req.Method == http.MethodDelete && strings.HasPrefix(path, "/containers/"):
		id := strings.TrimPrefix(path, "/containers/")
		if d.failRemove[id] {
			return errorResponse(http.StatusInternalServerError, fmt.Sprintf("cannot remove container %s: simulated daemon failure", id))
		}
		if _, ok := d.containers[id]; !ok {
			return errorResponse(http.StatusNotFound, fmt.Sprintf("No such container: %s", id))
		}
		delete(d.containers, id)
		d.removed = append(d.removed, id)
		return jsonResponse(http.StatusNoContent, nil)
	case req.Method == http.MethodGet && strings.HasPrefix(path, "/networks/"):
		name := strings.TrimPrefix(path, "/networks/")
		nw, ok := d.networks[name]
		if !ok {
			return errorResponse(http.StatusNotFound, fmt.Sprintf("network %s not found", name))
		}
		return jsonResponse(http.StatusOK, nw)
	case req.Method == http.MethodDelete && strings.HasPrefix(path, "/networks/"):
		name := strings.TrimPrefix(path, "/networks/")
		if _, ok := d.networks[name]; !ok {
			return errorResponse(http.StatusNotFound, fmt.Sprintf("network %s not found", name))
		}
		delete(d.networks, name)
		d.removedNetworks = append(d.removedNetworks, name)
		return jsonResponse(http.StatusNoContent, nil)
	}

	d.t.Errorf("fake docker daemon: unexpected request %s %s", req.Method, path)
	return errorResponse(http.StatusInternalServerError, "unexpected request")
}

// handleContainerList emulates the daemon's label filtering honestly: a request
// with no (or an incomplete) label filter would return the bystander containers
// too, which the tests would catch. Caller holds d.mu.
func (d *fakeDaemon) handleContainerList(req *http.Request) (*http.Response, error) {
	args := filters.NewArgs()
	if raw := req.URL.Query().Get("filters"); raw != "" {
		parsed, err := filters.FromJSON(raw)
		if err != nil {
			d.t.Errorf("fake docker daemon: bad filters %q: %v", raw, err)
			return errorResponse(http.StatusInternalServerError, "bad filters")
		}
		args = parsed
	}
	d.listFilters = append(d.listFilters, args)

	labelTerms := args.Get("label")
	out := make([]container.Summary, 0)
	for _, c := range d.containers {
		if matchesLabelTerms(c.summary.Labels, labelTerms) {
			out = append(out, c.summary)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return jsonResponse(http.StatusOK, out)
}

// handleContainerInspect answers with the resource limits the container
// reserved, so ReclaimToZero's freed-capacity accounting is exercised for real.
// Caller holds d.mu.
func (d *fakeDaemon) handleContainerInspect(id string) (*http.Response, error) {
	c, ok := d.containers[id]
	if !ok {
		return errorResponse(http.StatusNotFound, fmt.Sprintf("No such container: %s", id))
	}
	resp := container.InspectResponse{
		ContainerJSONBase: &container.ContainerJSONBase{
			ID: id,
			HostConfig: &container.HostConfig{
				Resources: container.Resources{Memory: c.memory, NanoCPUs: c.nanoCPUs},
			},
			State: &container.State{
				Running: isContainerRunningSummary(c.summary),
				Status:  c.summary.State,
			},
		},
		Config: &container.Config{Labels: c.summary.Labels},
	}
	return jsonResponse(http.StatusOK, resp)
}

func matchesLabelTerms(labels map[string]string, terms []string) bool {
	for _, term := range terms {
		key, value, hasValue := strings.Cut(term, "=")
		got, ok := labels[key]
		if !ok {
			return false
		}
		if hasValue && got != value {
			return false
		}
	}
	return true
}

func jsonResponse(status int, body interface{}) (*http.Response, error) {
	buf := &bytes.Buffer{}
	if body != nil {
		if err := json.NewEncoder(buf).Encode(body); err != nil {
			return nil, err
		}
	}
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(buf),
	}, nil
}

func errorResponse(status int, message string) (*http.Response, error) {
	return jsonResponse(status, map[string]string{"message": message})
}

// --- fixtures ---

const fakeBaseNetwork = "faasnet"

// newFakeDaemonProvider builds a real DockerProvider whose SDK client talks to
// the fake daemon. Same-package access lets the test construct the provider
// without touching docker_provider.go.
func newFakeDaemonProvider(t *testing.T, d *fakeDaemon) *DockerProvider {
	t.Helper()
	cli, err := client.NewClientWithOpts(
		client.WithHost("tcp://fake-docker-daemon:2375"),
		client.WithHTTPClient(&http.Client{Transport: d}),
		client.WithVersion("1.48"),
	)
	if err != nil {
		t.Fatalf("build docker client over fake daemon: %v", err)
	}
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	return &DockerProvider{client: cli, network: fakeBaseNetwork, logger: logger}
}

func labeledContainer(id, name, fn, state, status string, mem, cpus int64) fakeDaemonContainer {
	return fakeDaemonContainer{
		summary: container.Summary{
			ID:     id,
			Names:  []string{"/" + name},
			State:  state,
			Status: status,
			Labels: map[string]string{
				LabelType:     "function",
				LabelFunction: fn,
				LabelNetwork:  FunctionNetworkName(fakeBaseNetwork, fn),
				// Owned by THIS gateway (ownerID == network == fakeBaseNetwork).
				LabelGateway: fakeBaseNetwork,
			},
		},
		memory:   mem,
		nanoCPUs: cpus,
	}
}

// foreignGatewayContainer is a function-type container owned by a DIFFERENT
// docker-faas instance sharing the daemon (RT-223). It must never appear in
// this gateway's orphan scan.
func foreignGatewayContainer(id, name, fn string) fakeDaemonContainer {
	return fakeDaemonContainer{
		summary: container.Summary{
			ID:     id,
			Names:  []string{"/" + name},
			State:  "running",
			Status: "Up 3 minutes",
			Labels: map[string]string{
				LabelType:     "function",
				LabelFunction: fn,
				LabelNetwork:  FunctionNetworkName("other-gateway-net", fn),
				LabelGateway:  "other-gateway-net",
			},
		},
	}
}

func unlabeledContainer(id, name string) fakeDaemonContainer {
	return fakeDaemonContainer{
		summary: container.Summary{
			ID:     id,
			Names:  []string{"/" + name},
			State:  "running",
			Status: "Up 5 minutes",
			Labels: map[string]string{"app": "innocent-bystander"},
		},
	}
}

func functionNetwork(fn string) network.Inspect {
	return network.Inspect{
		Name: FunctionNetworkName(fakeBaseNetwork, fn),
		Labels: map[string]string{
			LabelNetworkType:     "function",
			LabelNetworkFunction: fn,
		},
		Containers: map[string]network.EndpointResource{},
	}
}

func requireLabelFilter(t *testing.T, args filters.Args, want string) {
	t.Helper()
	for _, term := range args.Get("label") {
		if term == want {
			return
		}
	}
	t.Fatalf("daemon request is missing label filter %q; got label terms %v", want, args.Get("label"))
}

func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

// --- tests ---

// TestObservedFunctionsExcludesForeignGatewayContainers is the RT-223
// regression: orphan detection must ignore function containers owned by a
// DIFFERENT docker-faas instance on the same daemon, so the idle reconciler can
// never reclaim another gateway's functions. Without the ownership scoping the
// foreign container's name leaks into ObservedFunctions and gets reclaimed as an
// "orphan."
func TestObservedFunctionsExcludesForeignGatewayContainers(t *testing.T) {
	d := newFakeDaemon(t)
	// This gateway owns "mine"; another gateway owns "theirs".
	d.containers["m1"] = labeledContainer("m1", "mine-0", "mine", "running", "Up 1 minute", 0, 0)
	d.containers["f1"] = foreignGatewayContainer("f1", "theirs-0", "theirs")
	d.containers["byst"] = unlabeledContainer("byst", "bystander")

	p := newFakeDaemonProvider(t, d)
	fns, err := p.ObservedFunctions(context.Background())
	if err != nil {
		t.Fatalf("ObservedFunctions: %v", err)
	}
	got := sortedCopy(fns)
	if len(got) != 1 || got[0] != "mine" {
		t.Fatalf("orphan scan must return only this gateway's functions, got %v", got)
	}

	// Defense in depth: the daemon list request itself must carry the
	// gateway-ownership label filter, so a foreign container is never even
	// enumerated.
	if len(d.listFilters) == 0 {
		t.Fatal("expected at least one container-list call")
	}
	requireLabelFilter(t, d.listFilters[len(d.listFilters)-1], fmt.Sprintf("%s=%s", LabelGateway, fakeBaseNetwork))
}

// TestPerFunctionListExcludesForeignButKeepsLegacy is the RT-223 per-function
// regression: when two gateways run a SAME-NAMED function on one daemon,
// ObservedReplicas / ReclaimToZero / routing must count and act on only THIS
// gateway's containers (its own label + legacy unlabeled ones) and never a
// foreign gateway's same-named container. Without the per-function ownership
// filter, gateway A would count and reclaim gateway B's "shared" replicas.
func TestPerFunctionListExcludesForeignButKeepsLegacy(t *testing.T) {
	d := newFakeDaemon(t)
	// Same function name "shared" across three owners.
	d.containers["mine"] = labeledContainer("mine", "shared-0", "shared", "running", "Up 1 minute", 0, 0)
	// Legacy container created before the ownership label existed (no LabelGateway).
	legacy := labeledContainer("legacy", "shared-1", "shared", "running", "Up 2 minutes", 0, 0)
	delete(legacy.summary.Labels, LabelGateway)
	d.containers["legacy"] = legacy
	// Foreign gateway's same-named container — must be excluded.
	d.containers["foreign"] = foreignGatewayContainer("foreign", "shared-9", "shared")

	p := newFakeDaemonProvider(t, d)

	// ObservedReplicas: only mine + legacy (2), never the foreign one.
	got, err := p.ObservedReplicas(context.Background(), "shared")
	if err != nil {
		t.Fatalf("ObservedReplicas: %v", err)
	}
	if got != 2 {
		t.Fatalf("expected 2 owned replicas (mine + legacy), got %d (foreign container leaked in?)", got)
	}

	// ReclaimToZero must remove exactly the owned containers and leave the
	// foreign one running.
	if _, err := p.ReclaimToZero(context.Background(), "shared"); err != nil {
		t.Fatalf("ReclaimToZero: %v", err)
	}
	if _, ok := d.containers["foreign"]; !ok {
		t.Fatal("ReclaimToZero removed a FOREIGN gateway's container — cross-instance destruction")
	}
	if _, ok := d.containers["mine"]; ok {
		t.Fatal("ReclaimToZero must have removed this gateway's own container")
	}
	if _, ok := d.containers["legacy"]; ok {
		t.Fatal("ReclaimToZero must have removed this gateway's legacy (unlabeled) container")
	}
}

// TestStrictOwnershipExcludesUnlabeledLegacy is the RT-234 fix: with strict
// ownership enabled (shared-daemon mode), per-function selection must ignore
// unlabeled/legacy containers too, so one gateway can never reclaim or scale a
// DIFFERENT gateway's pre-ownership-label container. Only THIS gateway's own
// labeled container is acted on.
func TestStrictOwnershipExcludesUnlabeledLegacy(t *testing.T) {
	d := newFakeDaemon(t)
	d.containers["mine"] = labeledContainer("mine", "shared-0", "shared", "running", "Up 1 minute", 0, 0)
	legacy := labeledContainer("legacy", "shared-1", "shared", "running", "Up 2 minutes", 0, 0)
	delete(legacy.summary.Labels, LabelGateway)
	d.containers["legacy"] = legacy
	d.containers["foreign"] = foreignGatewayContainer("foreign", "shared-9", "shared")

	p := newFakeDaemonProvider(t, d)
	p.SetStrictOwnership(true)

	// Only the labeled own container counts (1), never legacy or foreign.
	got, err := p.ObservedReplicas(context.Background(), "shared")
	if err != nil {
		t.Fatalf("ObservedReplicas: %v", err)
	}
	if got != 1 {
		t.Fatalf("strict mode must count only this gateway's labeled container, got %d", got)
	}

	if _, err := p.ReclaimToZero(context.Background(), "shared"); err != nil {
		t.Fatalf("ReclaimToZero: %v", err)
	}
	if _, ok := d.containers["mine"]; ok {
		t.Fatal("strict reclaim must remove this gateway's own labeled container")
	}
	if _, ok := d.containers["legacy"]; !ok {
		t.Fatal("strict reclaim must NOT remove an unlabeled legacy container (ambiguous owner)")
	}
	if _, ok := d.containers["foreign"]; !ok {
		t.Fatal("strict reclaim must NOT remove a foreign gateway's container")
	}
}

// TestRemoveStaleContainerByNameRespectsOwnership closes the strict-ownership
// escape in the create path: global Docker names are inspected before create,
// but a stopped foreign or ambiguous legacy container must never be deleted just
// because it occupies the desired name.
func TestRemoveStaleContainerByNameRespectsOwnership(t *testing.T) {
	tests := []struct {
		name        string
		strict      bool
		owner       string
		wantRemoved bool
	}{
		{name: "owned strict", strict: true, owner: fakeBaseNetwork, wantRemoved: true},
		{name: "foreign lenient", strict: false, owner: "other-gateway-net", wantRemoved: false},
		{name: "legacy strict", strict: true, owner: "", wantRemoved: false},
		{name: "legacy lenient", strict: false, owner: "", wantRemoved: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newFakeDaemon(t)
			c := labeledContainer("shared-0", "shared-0", "shared", "exited", "Exited (0)", 0, 0)
			if tt.owner == "" {
				delete(c.summary.Labels, LabelGateway)
			} else {
				c.summary.Labels[LabelGateway] = tt.owner
			}
			d.containers["shared-0"] = c

			p := newFakeDaemonProvider(t, d)
			p.SetStrictOwnership(tt.strict)
			err := p.removeStaleContainerByName(context.Background(), "shared-0")

			_, remains := d.containers["shared-0"]
			if tt.wantRemoved {
				if err != nil {
					t.Fatalf("owned/compatible stale container should be removable: %v", err)
				}
				if remains {
					t.Fatal("expected stale container to be removed")
				}
				return
			}

			if err == nil {
				t.Fatal("foreign or ambiguous stale container must be rejected")
			}
			if !remains {
				t.Fatal("ownership rejection must leave the existing container untouched")
			}
		})
	}
}

// TestObservedReplicasIsScopedToFunctionLabel: the replica observation must ask
// the daemon ONLY for containers labeled with the function name, and count only
// the running ones. The bystander and other-function containers prove the
// scoping: an unfiltered list would include them.
func TestObservedReplicasIsScopedToFunctionLabel(t *testing.T) {
	d := newFakeDaemon(t)
	d.containers["aaa1"] = labeledContainer("aaa1", "fnA-0", "fnA", "running", "Up 2 minutes", 0, 0)
	d.containers["aaa2"] = labeledContainer("aaa2", "fnA-1", "fnA", "exited", "Exited (0) 1 minute ago", 0, 0)
	d.containers["bbb1"] = labeledContainer("bbb1", "fnB-0", "fnB", "running", "Up 9 minutes", 0, 0)
	d.containers["zzz1"] = unlabeledContainer("zzz1", "innocent")

	p := newFakeDaemonProvider(t, d)

	got, err := p.ObservedReplicas(context.Background(), "fnA")
	if err != nil {
		t.Fatalf("ObservedReplicas: %v", err)
	}
	if got != 1 {
		t.Fatalf("ObservedReplicas(fnA) = %d, want 1 (only the RUNNING fnA replica)", got)
	}

	if len(d.listFilters) != 1 {
		t.Fatalf("expected exactly one container list, got %d", len(d.listFilters))
	}
	requireLabelFilter(t, d.listFilters[0], LabelFunction+"=fnA")
	if d.requests[0].query.Get("all") != "1" {
		t.Fatalf("observation must list ALL containers (including stopped) for the function; query=%v", d.requests[0].query)
	}
}

// TestObservedFunctionsIsScopedToFunctionTypeLabel: orphan detection must list
// only containers labeled as function type and derive distinct names from the
// function label, skipping any container that lacks the name label.
func TestObservedFunctionsIsScopedToFunctionTypeLabel(t *testing.T) {
	d := newFakeDaemon(t)
	d.containers["aaa1"] = labeledContainer("aaa1", "fnA-0", "fnA", "running", "Up 2 minutes", 0, 0)
	d.containers["aaa2"] = labeledContainer("aaa2", "fnA-1", "fnA", "exited", "Exited (0) 1 minute ago", 0, 0)
	d.containers["bbb1"] = labeledContainer("bbb1", "fnB-0", "fnB", "running", "Up 9 minutes", 0, 0)
	d.containers["zzz1"] = unlabeledContainer("zzz1", "innocent")
	// Type-labeled but nameless: the daemon-side filter matches it, the code
	// must skip it rather than invent a function.
	d.containers["ccc1"] = fakeDaemonContainer{summary: container.Summary{
		ID:     "ccc1",
		Names:  []string{"/nameless"},
		State:  "running",
		Status: "Up 1 minute",
		Labels: map[string]string{LabelType: "function"},
	}}

	p := newFakeDaemonProvider(t, d)

	fns, err := p.ObservedFunctions(context.Background())
	if err != nil {
		t.Fatalf("ObservedFunctions: %v", err)
	}
	if got, want := sortedCopy(fns), []string{"fnA", "fnB"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("ObservedFunctions = %v, want %v (distinct labeled functions only)", got, want)
	}

	if len(d.listFilters) != 1 {
		t.Fatalf("expected exactly one container list, got %d", len(d.listFilters))
	}
	requireLabelFilter(t, d.listFilters[0], LabelType+"=function")
}

// TestReclaimToZeroRemovesOnlyLabelScopedContainers is the load-bearing scoping
// proof: reclaim stops/removes EXACTLY the containers the label-filtered list
// returned — never the bystander, never another function's replicas — accounts
// the freed capacity from those containers only, and removes only the managed
// per-function network (never the shared base network, never another
// function's network).
func TestReclaimToZeroRemovesOnlyLabelScopedContainers(t *testing.T) {
	d := newFakeDaemon(t)
	d.containers["aaa1"] = labeledContainer("aaa1", "fnA-0", "fnA", "running", "Up 2 minutes", 128*1024*1024, 500_000_000)
	d.containers["aaa2"] = labeledContainer("aaa2", "fnA-1", "fnA", "exited", "Exited (0) 1 minute ago", 64*1024*1024, 250_000_000)
	d.containers["bbb1"] = labeledContainer("bbb1", "fnB-0", "fnB", "running", "Up 9 minutes", 0, 0)
	d.containers["zzz1"] = unlabeledContainer("zzz1", "innocent")
	d.networks[fakeBaseNetwork] = network.Inspect{
		Name:   fakeBaseNetwork,
		Labels: map[string]string{"com.docker-faas.network": "true", LabelNetworkType: "base"},
	}
	d.networks[FunctionNetworkName(fakeBaseNetwork, "fnA")] = functionNetwork("fnA")
	d.networks[FunctionNetworkName(fakeBaseNetwork, "fnB")] = functionNetwork("fnB")

	p := newFakeDaemonProvider(t, d)

	res, err := p.ReclaimToZero(context.Background(), "fnA")
	if err != nil {
		t.Fatalf("ReclaimToZero: %v", err)
	}

	if res.ContainersRemoved != 2 {
		t.Fatalf("ContainersRemoved = %d, want 2", res.ContainersRemoved)
	}
	if want := int64(192 * 1024 * 1024); res.MemoryBytesFreed != want {
		t.Fatalf("MemoryBytesFreed = %d, want %d (sum of the reclaimed containers' limits)", res.MemoryBytesFreed, want)
	}
	if want := int64(750_000_000); res.NanoCPUsFreed != want {
		t.Fatalf("NanoCPUsFreed = %d, want %d", res.NanoCPUsFreed, want)
	}
	if res.NetworksRemoved != 1 {
		t.Fatalf("NetworksRemoved = %d, want 1 (the managed per-function network)", res.NetworksRemoved)
	}

	// Exactly the label-filtered list result was removed: nothing more.
	if got, want := sortedCopy(d.removed), []string{"aaa1", "aaa2"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("removed containers = %v, want exactly %v", got, want)
	}
	for _, id := range d.removed {
		// Structural restatement of the invariant: every removed ID carried the
		// function's own label (the fake daemon only listed such IDs).
		if id != "aaa1" && id != "aaa2" {
			t.Fatalf("removed container %s was not part of the label-scoped list", id)
		}
	}
	if got, want := sortedCopy(d.stopped), []string{"aaa1"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("stopped containers = %v, want %v (only the running replica needs a stop)", got, want)
	}
	if _, alive := d.containers["bbb1"]; !alive {
		t.Fatalf("another function's replica must never be touched by reclaim")
	}
	if _, alive := d.containers["zzz1"]; !alive {
		t.Fatalf("an unlabeled bystander container must never be touched by reclaim")
	}

	// The wire-level filter of the reclaim's list is the function label.
	if len(d.listFilters) != 1 {
		t.Fatalf("expected exactly one container list during reclaim, got %d", len(d.listFilters))
	}
	requireLabelFilter(t, d.listFilters[0], LabelFunction+"=fnA")

	// Network scoping: only fnA's managed network went away.
	if got, want := sortedCopy(d.removedNetworks), []string{FunctionNetworkName(fakeBaseNetwork, "fnA")}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("removed networks = %v, want %v", got, want)
	}
	if _, alive := d.networks[fakeBaseNetwork]; !alive {
		t.Fatalf("the shared base network must never be removed by reclaim")
	}
	if _, alive := d.networks[FunctionNetworkName(fakeBaseNetwork, "fnB")]; !alive {
		t.Fatalf("another function's network must never be removed by reclaim")
	}
}

// TestReclaimToZeroIdempotentOnEmptyFunction: reclaiming a function with no
// containers and no network is a no-op returning a ZERO report — in particular
// it must not fabricate NetworksRemoved=1 out of the network never having
// existed.
func TestReclaimToZeroIdempotentOnEmptyFunction(t *testing.T) {
	d := newFakeDaemon(t)
	d.containers["bbb1"] = labeledContainer("bbb1", "fnB-0", "fnB", "running", "Up 9 minutes", 0, 0)
	d.networks[fakeBaseNetwork] = network.Inspect{Name: fakeBaseNetwork}

	p := newFakeDaemonProvider(t, d)

	res, err := p.ReclaimToZero(context.Background(), "fnA")
	if err != nil {
		t.Fatalf("ReclaimToZero on empty function: %v", err)
	}
	if res != (ReclaimResult{}) {
		t.Fatalf("reclaiming an already-empty function must return a zero report, got %+v", res)
	}
	if len(d.removed) != 0 || len(d.stopped) != 0 || len(d.removedNetworks) != 0 {
		t.Fatalf("no-op reclaim must not remove anything: removed=%v stopped=%v networks=%v", d.removed, d.stopped, d.removedNetworks)
	}
}

// TestReclaimToZeroPropagatesRemoveFailure: a daemon failure mid-reclaim
// surfaces as an error with a truthful partial report, and the per-function
// network is left alone so a later retry can finish the job.
func TestReclaimToZeroPropagatesRemoveFailure(t *testing.T) {
	d := newFakeDaemon(t)
	d.containers["aaa1"] = labeledContainer("aaa1", "fnA-0", "fnA", "running", "Up 2 minutes", 0, 0)
	d.containers["aaa2"] = labeledContainer("aaa2", "fnA-1", "fnA", "exited", "Exited (0) 1 minute ago", 0, 0)
	d.networks[FunctionNetworkName(fakeBaseNetwork, "fnA")] = functionNetwork("fnA")
	d.failRemove["aaa2"] = true

	p := newFakeDaemonProvider(t, d)

	res, err := p.ReclaimToZero(context.Background(), "fnA")
	if err == nil {
		t.Fatalf("expected the daemon remove failure to propagate")
	}
	if !strings.Contains(err.Error(), "failed to remove container during reclaim") {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ContainersRemoved != 1 {
		t.Fatalf("partial report must count only the successful removal, got %+v", res)
	}
	if got, want := sortedCopy(d.removed), []string{"aaa1"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("removed = %v, want %v", got, want)
	}
	if _, alive := d.networks[FunctionNetworkName(fakeBaseNetwork, "fnA")]; !alive {
		t.Fatalf("network must be left for the retry after a failed container removal")
	}
}
