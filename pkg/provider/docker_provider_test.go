package provider

import (
	"reflect"
	"sort"
	"testing"

	"github.com/docker/docker/api/types/container"
)

func TestParseMemory(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int64
	}{
		// Docker formats
		{"Docker kilobytes", "128k", 128 * 1024},
		{"Docker megabytes", "256m", 256 * 1024 * 1024},
		{"Docker gigabytes", "2g", 2 * 1024 * 1024 * 1024},
		{"Docker uppercase", "128M", 128 * 1024 * 1024},

		// Kubernetes formats
		{"Kubernetes KiB", "128Ki", 128 * 1024},
		{"Kubernetes MiB", "256Mi", 256 * 1024 * 1024},
		{"Kubernetes GiB", "2Gi", 2 * 1024 * 1024 * 1024},
		{"Kubernetes uppercase", "128MI", 128 * 1024 * 1024},
		{"Kubernetes mixed case", "256mI", 256 * 1024 * 1024},

		// Edge cases
		{"Empty string", "", 0},
		{"Whitespace", "  256Mi  ", 256 * 1024 * 1024},
		{"Zero", "0", 0},
		{"Zero with unit", "0Mi", 0},

		// OpenFaaS common values
		{"OpenFaaS default small", "128Mi", 128 * 1024 * 1024},
		{"OpenFaaS default medium", "256Mi", 256 * 1024 * 1024},
		{"OpenFaaS default large", "512Mi", 512 * 1024 * 1024},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseMemory(tt.input)
			if result != tt.expected {
				t.Errorf("parseMemory(%q) = %d, expected %d", tt.input, result, tt.expected)
			}
		})
	}
}

func TestParseCPU(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int64
	}{
		// Docker formats (decimal cores)
		{"Docker half core", "0.5", 500000000},
		{"Docker one core", "1", 1000000000},
		{"Docker two cores", "2", 2000000000},
		{"Docker quarter core", "0.25", 250000000},

		// Kubernetes formats (millicores)
		{"Kubernetes 500 millicores", "500m", 500000000},
		{"Kubernetes 1000 millicores", "1000m", 1000000000},
		{"Kubernetes 2000 millicores", "2000m", 2000000000},
		{"Kubernetes 250 millicores", "250m", 250000000},
		{"Kubernetes 100 millicores", "100m", 100000000},
		{"Kubernetes uppercase", "500M", 500000000},

		// Edge cases
		{"Empty string", "", 0},
		{"Whitespace", "  500m  ", 500000000},
		{"Zero", "0", 0},
		{"Zero with unit", "0m", 0},

		// OpenFaaS common values
		{"OpenFaaS default small", "100m", 100000000},
		{"OpenFaaS default medium", "500m", 500000000},
		{"OpenFaaS default large", "1000m", 1000000000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseCPU(tt.input)
			if result != tt.expected {
				t.Errorf("parseCPU(%q) = %d, expected %d", tt.input, result, tt.expected)
			}
		})
	}
}

// TestResourceFormatCompatibility verifies that both Docker and Kubernetes formats
// produce the same results for equivalent resource values.
func TestResourceFormatCompatibility(t *testing.T) {
	memoryTests := []struct {
		docker     string
		kubernetes string
	}{
		{"128m", "128Mi"},
		{"256m", "256Mi"},
		{"512m", "512Mi"},
		{"1g", "1Gi"},
		{"2g", "2Gi"},
	}

	for _, tt := range memoryTests {
		dockerResult := parseMemory(tt.docker)
		kubeResult := parseMemory(tt.kubernetes)
		if dockerResult != kubeResult {
			t.Errorf("Memory format mismatch: Docker %q = %d, Kubernetes %q = %d",
				tt.docker, dockerResult, tt.kubernetes, kubeResult)
		}
	}

	cpuTests := []struct {
		docker     string
		kubernetes string
	}{
		{"0.1", "100m"},
		{"0.25", "250m"},
		{"0.5", "500m"},
		{"1", "1000m"},
		{"2", "2000m"},
	}

	for _, tt := range cpuTests {
		dockerResult := parseCPU(tt.docker)
		kubeResult := parseCPU(tt.kubernetes)
		if dockerResult != kubeResult {
			t.Errorf("CPU format mismatch: Docker %q = %d, Kubernetes %q = %d",
				tt.docker, dockerResult, tt.kubernetes, kubeResult)
		}
	}
}

func TestBuildReplicaScalePlan_RemovesStaleStoppedReplicaAndRecreatesMissingIndex(t *testing.T) {
	t.Parallel()

	plan := buildReplicaScalePlan([]container.Summary{
		{
			ID:     "stale-0",
			Names:  []string{"/import-bundle-0"},
			Labels: map[string]string{LabelReplica: "0"},
			State:  "exited",
			Status: "Exited (137) 8 days ago",
		},
	}, 1)

	if got := replicaIndices(plan.missingReplicaIndices); !reflect.DeepEqual(got, []int{0}) {
		t.Fatalf("missingReplicaIndices = %v, want [0]", got)
	}
	if got := containerNames(plan.staleToRemove); !reflect.DeepEqual(got, []string{"import-bundle-0"}) {
		t.Fatalf("staleToRemove = %v, want [import-bundle-0]", got)
	}
	if len(plan.activeToRemove) != 0 {
		t.Fatalf("expected no active containers to remove, got %v", containerNames(plan.activeToRemove))
	}
}

func TestBuildReplicaScalePlan_FillsReplicaGapsAndRemovesExcessRunningContainers(t *testing.T) {
	t.Parallel()

	plan := buildReplicaScalePlan([]container.Summary{
		{
			ID:      "run-0",
			Names:   []string{"/hello-0"},
			Labels:  map[string]string{LabelReplica: "0"},
			State:   "running",
			Status:  "Up 2 minutes",
			Created: 100,
		},
		{
			ID:      "run-2",
			Names:   []string{"/hello-2"},
			Labels:  map[string]string{LabelReplica: "2"},
			State:   "running",
			Status:  "Up 1 minute",
			Created: 200,
		},
	}, 2)

	if got := replicaIndices(plan.missingReplicaIndices); !reflect.DeepEqual(got, []int{1}) {
		t.Fatalf("missingReplicaIndices = %v, want [1]", got)
	}
	if len(plan.staleToRemove) != 0 {
		t.Fatalf("expected no stale containers to remove, got %v", containerNames(plan.staleToRemove))
	}
	if got := containerNames(plan.activeToRemove); !reflect.DeepEqual(got, []string{"hello-2"}) {
		t.Fatalf("activeToRemove = %v, want [hello-2]", got)
	}
}

func TestBuildReplicaScalePlan_KeepsNewestRunningReplicaAndRemovesDuplicates(t *testing.T) {
	t.Parallel()

	plan := buildReplicaScalePlan([]container.Summary{
		{
			ID:      "old-running",
			Names:   []string{"/echo-0"},
			Labels:  map[string]string{LabelReplica: "0"},
			State:   "running",
			Status:  "Up 10 minutes",
			Created: 100,
		},
		{
			ID:      "new-running",
			Names:   []string{"/echo-0"},
			Labels:  map[string]string{LabelReplica: "0"},
			State:   "running",
			Status:  "Up 1 minute",
			Created: 200,
		},
		{
			ID:      "stale-1",
			Names:   []string{"/echo-1"},
			Labels:  map[string]string{LabelReplica: "1"},
			State:   "dead",
			Status:  "Dead",
			Created: 50,
		},
	}, 1)

	if len(plan.missingReplicaIndices) != 0 {
		t.Fatalf("expected no missing replica indices, got %v", plan.missingReplicaIndices)
	}
	if got := containerIDs(plan.activeToRemove); !reflect.DeepEqual(got, []string{"old-running"}) {
		t.Fatalf("activeToRemove IDs = %v, want [old-running]", got)
	}
	if got := containerNames(plan.staleToRemove); !reflect.DeepEqual(got, []string{"echo-1"}) {
		t.Fatalf("staleToRemove = %v, want [echo-1]", got)
	}
}

func TestContainerReplicaIndex_FallsBackToContainerName(t *testing.T) {
	t.Parallel()

	replicaIndex, ok := containerReplicaIndex(container.Summary{
		Names: []string{"/reporter-7"},
	})
	if !ok {
		t.Fatal("expected replica index to be parsed from container name")
	}
	if replicaIndex != 7 {
		t.Fatalf("replicaIndex = %d, want 7", replicaIndex)
	}
}

func replicaIndices(indices []int) []int {
	cloned := append([]int(nil), indices...)
	sort.Ints(cloned)
	return cloned
}

func containerNames(containers []container.Summary) []string {
	names := make([]string, 0, len(containers))
	for _, summary := range containers {
		names = append(names, containerSummaryName(summary))
	}
	sort.Strings(names)
	return names
}

func containerIDs(containers []container.Summary) []string {
	ids := make([]string, 0, len(containers))
	for _, summary := range containers {
		ids = append(ids, summary.ID)
	}
	sort.Strings(ids)
	return ids
}
