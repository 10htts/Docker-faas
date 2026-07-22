package gateway

import (
	"io"
	"testing"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/docker-faas/docker-faas/pkg/store"
	"github.com/docker-faas/docker-faas/pkg/types"
)

func testIdleLogger() *logrus.Logger {
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	return logger
}

func idleControllerWithLabels(t *testing.T, labels map[string]string, defaults PolicyDefaults) *IdleController {
	t.Helper()
	encoded, err := store.EncodeMap(labels)
	if err != nil {
		t.Fatalf("failed to encode labels: %v", err)
	}
	fs := &fakeStore{functions: map[string]*types.FunctionMetadata{
		"fn": {Name: "fn", Image: "img:1", Labels: encoded},
	}}
	return NewIdleController(nil, fs, "net", defaults, testIdleLogger())
}

func TestPolicyFor_OfficialOpenFaaSLabels(t *testing.T) {
	c := idleControllerWithLabels(t, map[string]string{
		LabelOpenFaaSScaleZero:         "true",
		LabelOpenFaaSScaleZeroDuration: "15m",
		LabelOpenFaaSScaleMin:          "2",
		LabelOpenFaaSScaleMax:          "7",
	}, PolicyDefaults{Enabled: false, IdleDuration: 5 * time.Minute, MinReplicas: 0, MaxReplicas: 10})

	policy := c.PolicyFor("fn")
	if !policy.Enabled {
		t.Fatalf("expected com.openfaas.scale.zero=true to enable idle scale-to-zero")
	}
	if policy.IdleDuration != 15*time.Minute {
		t.Fatalf("expected idle duration 15m, got %v", policy.IdleDuration)
	}
	if policy.MinReplicas != 2 {
		t.Fatalf("expected min replicas 2, got %d", policy.MinReplicas)
	}
	if policy.MaxReplicas != 7 {
		t.Fatalf("expected max replicas 7, got %d", policy.MaxReplicas)
	}
}

func TestPolicyFor_OfficialZeroLabelNumericTrue(t *testing.T) {
	c := idleControllerWithLabels(t, map[string]string{
		LabelOpenFaaSScaleZero: "1",
	}, PolicyDefaults{Enabled: false, IdleDuration: 5 * time.Minute})

	if policy := c.PolicyFor("fn"); !policy.Enabled {
		t.Fatalf("expected com.openfaas.scale.zero=1 to enable idle scale-to-zero")
	}
}

func TestPolicyFor_ZeroDurationBareSeconds(t *testing.T) {
	c := idleControllerWithLabels(t, map[string]string{
		LabelOpenFaaSScaleZeroDuration: "90",
	}, PolicyDefaults{Enabled: true, IdleDuration: 5 * time.Minute})

	if policy := c.PolicyFor("fn"); policy.IdleDuration != 90*time.Second {
		t.Fatalf("expected bare-seconds duration 90s, got %v", policy.IdleDuration)
	}
}

func TestPolicyFor_InvalidOfficialValuesFallBackToDefaults(t *testing.T) {
	c := idleControllerWithLabels(t, map[string]string{
		LabelOpenFaaSScaleZero:         "sometimes",
		LabelOpenFaaSScaleZeroDuration: "soon",
		LabelOpenFaaSScaleMin:          "-2",
		LabelOpenFaaSScaleMax:          "zero",
	}, PolicyDefaults{Enabled: true, IdleDuration: 4 * time.Minute, MinReplicas: 1, MaxReplicas: 6})

	policy := c.PolicyFor("fn")
	if !policy.Enabled || policy.IdleDuration != 4*time.Minute || policy.MinReplicas != 1 || policy.MaxReplicas != 6 {
		t.Fatalf("expected defaults to survive invalid label values, got %+v", policy)
	}
}

func TestPolicyFor_CustomLabelsWinOverOfficial(t *testing.T) {
	c := idleControllerWithLabels(t, map[string]string{
		LabelOpenFaaSScaleZero:         "true",
		LabelOpenFaaSScaleZeroDuration: "15m",
		LabelOpenFaaSScaleMin:          "3",
		LabelOpenFaaSScaleMax:          "9",
		LabelIdleScaleToZero:           "false",
		LabelIdleSeconds:               "30",
		LabelMinReplicas:               "1",
		LabelMaxReplicas:               "2",
	}, PolicyDefaults{Enabled: false, IdleDuration: 5 * time.Minute, MinReplicas: 0, MaxReplicas: 10})

	policy := c.PolicyFor("fn")
	if policy.Enabled {
		t.Fatalf("expected custom idle-to-zero=false to win over official zero=true")
	}
	if policy.IdleDuration != 30*time.Second {
		t.Fatalf("expected custom idle-seconds=30 to win, got %v", policy.IdleDuration)
	}
	if policy.MinReplicas != 1 {
		t.Fatalf("expected custom min-replicas=1 to win, got %d", policy.MinReplicas)
	}
	if policy.MaxReplicas != 2 {
		t.Fatalf("expected custom max-replicas=2 to win, got %d", policy.MaxReplicas)
	}
}

func TestScaleBoundsFromLabels(t *testing.T) {
	cases := []struct {
		name    string
		labels  map[string]string
		wantMin int
		wantMax int
	}{
		{
			name:   "unset",
			labels: map[string]string{},
		},
		{
			name: "official labels",
			labels: map[string]string{
				LabelOpenFaaSScaleMin: "2",
				LabelOpenFaaSScaleMax: "8",
			},
			wantMin: 2,
			wantMax: 8,
		},
		{
			name: "custom labels win",
			labels: map[string]string{
				LabelOpenFaaSScaleMin: "2",
				LabelOpenFaaSScaleMax: "8",
				LabelMinReplicas:      "4",
				LabelMaxReplicas:      "5",
			},
			wantMin: 4,
			wantMax: 5,
		},
		{
			name: "invalid values ignored",
			labels: map[string]string{
				LabelOpenFaaSScaleMin: "lots",
				LabelOpenFaaSScaleMax: "0",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotMin, gotMax := scaleBoundsFromLabels(tc.labels)
			if gotMin != tc.wantMin || gotMax != tc.wantMax {
				t.Fatalf("scaleBoundsFromLabels(%v) = (%d, %d), want (%d, %d)",
					tc.labels, gotMin, gotMax, tc.wantMin, tc.wantMax)
			}
		})
	}
}

func TestParseScaleZeroDuration(t *testing.T) {
	if d, ok := parseScaleZeroDuration("10m30s"); !ok || d != 10*time.Minute+30*time.Second {
		t.Fatalf("expected 10m30s to parse, got (%v, %v)", d, ok)
	}
	if d, ok := parseScaleZeroDuration("45"); !ok || d != 45*time.Second {
		t.Fatalf("expected bare 45 to parse as seconds, got (%v, %v)", d, ok)
	}
	for _, invalid := range []string{"", "soon", "-5m", "0"} {
		if _, ok := parseScaleZeroDuration(invalid); ok {
			t.Fatalf("expected %q to be rejected", invalid)
		}
	}
}
