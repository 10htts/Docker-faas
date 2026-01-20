package provider

import (
	"testing"
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
