package scaletozero

import (
	"testing"
	"time"
)

func TestIdleDecider(t *testing.T) {
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	enabled := Policy{Enabled: true, IdleDuration: 60 * time.Second}

	cases := []struct {
		name   string
		policy Policy
		snap   ActivitySnapshot
		want   Action
	}{
		{
			name:   "disabled never reaps",
			policy: Policy{Enabled: false, IdleDuration: time.Second},
			snap:   ActivitySnapshot{LastActivity: now.Add(-time.Hour), ObservedReplicas: 1},
			want:   ActionHold,
		},
		{
			name:   "warm minimum keeps warm",
			policy: Policy{Enabled: true, IdleDuration: 60 * time.Second, MinReplicas: 2},
			snap:   ActivitySnapshot{LastActivity: now.Add(-time.Hour), ObservedReplicas: 0},
			want:   ActionKeepWarm,
		},
		{
			name:   "warm minimum above max replicas is clamped to max",
			policy: Policy{Enabled: true, IdleDuration: 60 * time.Second, MinReplicas: 5, MaxReplicas: 2},
			snap:   ActivitySnapshot{LastActivity: now.Add(-time.Hour), ObservedReplicas: 0},
			want:   ActionKeepWarm,
		},
		{
			name:   "warm minimum within max replicas is unchanged",
			policy: Policy{Enabled: true, IdleDuration: 60 * time.Second, MinReplicas: 2, MaxReplicas: 5},
			snap:   ActivitySnapshot{LastActivity: now.Add(-time.Hour), ObservedReplicas: 0},
			want:   ActionKeepWarm,
		},
		{
			name:   "gateway in-flight holds (SZ-01)",
			policy: enabled,
			snap:   ActivitySnapshot{GatewayInFlight: 1, LastActivity: now.Add(-time.Hour), ObservedReplicas: 1},
			want:   ActionHold,
		},
		{
			name:   "durable in-flight holds (SZ-03)",
			policy: enabled,
			snap:   ActivitySnapshot{DurableInFlight: 4, LastActivity: now.Add(-time.Hour), ObservedReplicas: 1},
			want:   ActionHold,
		},
		{
			name:   "recent activity holds - no thrash under burst (SZ-08)",
			policy: enabled,
			snap:   ActivitySnapshot{LastActivity: now.Add(-10 * time.Second), ObservedReplicas: 1},
			want:   ActionHold,
		},
		{
			name:   "no activity history holds conservatively",
			policy: enabled,
			snap:   ActivitySnapshot{ObservedReplicas: 1},
			want:   ActionHold,
		},
		{
			name:   "idle beyond window with no work reaps",
			policy: enabled,
			snap:   ActivitySnapshot{LastActivity: now.Add(-5 * time.Minute), ObservedReplicas: 1},
			want:   ActionScaleToZero,
		},
	}

	var d IdleDecider
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := d.Decide(now, tc.policy, tc.snap)
			if got.Action != tc.want {
				t.Fatalf("Decide() action = %v (%q), want %v", got.Action, got.Reason, tc.want)
			}
			if tc.want == ActionKeepWarm && got.DesiredReplicas != tc.policy.EffectiveWarmMinimum() {
				t.Fatalf("keep-warm DesiredReplicas = %d, want %d", got.DesiredReplicas, tc.policy.EffectiveWarmMinimum())
			}
			if tc.want == ActionScaleToZero && got.DesiredReplicas != 0 {
				t.Fatalf("scale-to-zero DesiredReplicas = %d, want 0", got.DesiredReplicas)
			}
		})
	}
}

// TestPolicyEffectiveWarmMinimum pins the MinReplicas/MaxReplicas clamp used by
// both the decider and the reconciler's EnsureWarmMinimum flow: keep-warm must
// never scale a function past its own replica cap.
func TestPolicyEffectiveWarmMinimum(t *testing.T) {
	cases := []struct {
		name             string
		min, max, expect int
	}{
		{"no warm minimum", 0, 4, 0},
		{"negative warm minimum treated as none", -1, 4, 0},
		{"no cap keeps declared minimum", 3, 0, 3},
		{"minimum within cap unchanged", 2, 5, 2},
		{"minimum equal to cap unchanged", 5, 5, 5},
		{"minimum above cap clamped to cap", 5, 2, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := Policy{Enabled: true, MinReplicas: tc.min, MaxReplicas: tc.max}
			if got := p.EffectiveWarmMinimum(); got != tc.expect {
				t.Fatalf("EffectiveWarmMinimum(min=%d,max=%d) = %d, want %d", tc.min, tc.max, got, tc.expect)
			}
		})
	}

	// The clamped decision must also surface the clamp in its reason so the
	// misconfiguration is visible to operators.
	d := IdleDecider{}
	got := d.Decide(time.Now(), Policy{Enabled: true, MinReplicas: 5, MaxReplicas: 2}, ActivitySnapshot{})
	if got.Action != ActionKeepWarm || got.DesiredReplicas != 2 {
		t.Fatalf("clamped keep-warm = %+v, want KeepWarm with 2 replicas", got)
	}
}

// TestDeciderInFlightBeatsElapsedIdle is the crux of SZ-01: even when the idle
// window has long elapsed, ANY in-flight work must hold the function.
func TestDeciderInFlightBeatsElapsedIdle(t *testing.T) {
	now := time.Now()
	d := IdleDecider{}
	p := Policy{Enabled: true, IdleDuration: time.Second}

	// A long-running invocation: last activity was hours ago (the request has
	// been executing the whole time) but it is still counted in-flight.
	snap := ActivitySnapshot{GatewayInFlight: 1, LastActivity: now.Add(-3 * time.Hour), ObservedReplicas: 1}
	if got := d.Decide(now, p, snap); got.Action != ActionHold {
		t.Fatalf("long-running invocation must hold, got %v (%q)", got.Action, got.Reason)
	}
}
