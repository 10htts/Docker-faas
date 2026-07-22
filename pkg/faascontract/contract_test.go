package faascontract

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestGoldenRequestFixtureIsCrossRepoConformanceVector is the SZ-12 cross-repo
// evidence for the request. The committed golden JSON is BYTE-IDENTICAL to the
// AIDrivenMES copy (internal/faascontract/testdata/activity_lease_request.json);
// it must decode + verify under GoldenFixtureSecret, expose the exact canonical
// field set, and re-signing the canonical example must reproduce the committed
// signature byte-for-byte. Any drift in the wire schema or signing recipe fails
// this test in BOTH repos rather than silently disagreeing.
func TestGoldenRequestFixtureIsCrossRepoConformanceVector(t *testing.T) {
	raw := readFixture(t, "activity_lease_request.json")

	decoded, err := DecodeActivityLeaseRequest(raw, GoldenFixtureSecret)
	if err != nil {
		t.Fatalf("golden request fixture failed to decode/verify: %v", err)
	}
	if err := decoded.Validate(); err != nil {
		t.Fatalf("golden request fixture must be valid: %v", err)
	}

	want := GoldenActivityLeaseRequest()
	if decoded.Function != want.Function ||
		decoded.OrganizationID != want.OrganizationID ||
		decoded.Generation != want.Generation ||
		decoded.Admitted != want.Admitted ||
		decoded.Queued != want.Queued ||
		decoded.Running != want.Running ||
		decoded.TemporalDispatched != want.TemporalDispatched ||
		decoded.LeaseTTLSeconds != want.LeaseTTLSeconds ||
		decoded.Nonce != want.Nonce {
		t.Fatalf("golden request fixture drifted from GoldenActivityLeaseRequest: %+v", decoded)
	}
	if decoded.DurableInFlight() != 6 {
		t.Fatalf("golden request DurableInFlight = %d, want 6", decoded.DurableInFlight())
	}

	assertJSONKeys(t, raw, []string{
		"contract_version", "function", "organization_id", "generation",
		"admitted", "queued", "running", "temporal_dispatched",
		"lease_ttl_seconds", "issued_at", "last_activity_at", "nonce", "signature",
	})

	// Re-sign the canonical example and confirm the signature matches the file.
	resigned := SignActivityLeaseRequest(want, GoldenFixtureSecret)
	if resigned.Signature != decoded.Signature {
		t.Fatalf("request signature recipe drift: recomputed %q, fixture %q", resigned.Signature, decoded.Signature)
	}
	// A one-byte secret change must invalidate the fixture (proves it is actually
	// authenticated, not merely well-formed).
	if _, err := DecodeActivityLeaseRequest(raw, GoldenFixtureSecret+"x"); err == nil {
		t.Fatal("golden request fixture must not verify under a different secret")
	}
}

func TestGoldenResponseFixtureIsCrossRepoConformanceVector(t *testing.T) {
	raw := readFixture(t, "activity_lease_response.json")

	decoded, err := DecodeActivityLeaseResponse(raw, GoldenFixtureSecret)
	if err != nil {
		t.Fatalf("golden response fixture failed to decode/verify: %v", err)
	}
	if decoded.Decision != DecisionHold {
		t.Fatalf("golden response decision = %q, want %q", decoded.Decision, DecisionHold)
	}
	if decoded.TotalInFlight != decoded.GatewayInFlight+decoded.DurableInFlight {
		t.Fatalf("total_in_flight must equal gateway+durable")
	}
	if !decoded.IdleScaleToZeroSupported {
		t.Fatalf("golden response must advertise idle scale-to-zero support")
	}

	assertJSONKeys(t, raw, []string{
		"contract_version", "function", "accepted", "generation",
		"lease_expires_at", "gateway_in_flight", "durable_in_flight",
		"total_in_flight", "observed_replicas", "idle_scale_to_zero_supported",
		"decision", "decision_reason", "issued_at", "nonce", "signature",
	})

	resigned := SignActivityLeaseResponse(GoldenActivityLeaseResponse(), GoldenFixtureSecret)
	if resigned.Signature != decoded.Signature {
		t.Fatalf("response signature recipe drift: recomputed %q, fixture %q", resigned.Signature, decoded.Signature)
	}
	if _, err := DecodeActivityLeaseResponse(raw, GoldenFixtureSecret+"x"); err == nil {
		t.Fatal("golden response fixture must not verify under a different secret")
	}
}

func TestCapabilitiesFixtureKeepsKubernetesUnclaimed(t *testing.T) {
	raw := readFixture(t, "capabilities.json")

	var caps Capabilities
	if err := json.Unmarshal(raw, &caps); err != nil {
		t.Fatalf("decode capabilities fixture: %v", err)
	}
	if caps.ContractVersion != ContractVersion {
		t.Fatalf("capabilities contract_version = %q, want %q", caps.ContractVersion, ContractVersion)
	}
	if !caps.IdleScaleToZero {
		t.Fatalf("Docker provider must advertise idle_scale_to_zero=true")
	}
	if caps.Kubernetes.Supported {
		t.Fatalf("Kubernetes must stay unclaimed (supported=false) from this provider (SZ-05/SZ-10)")
	}
	if caps.Orchestration != "docker" {
		t.Fatalf("orchestration = %q, want docker", caps.Orchestration)
	}
	assertJSONKeys(t, raw, []string{
		"contract_version", "provider", "orchestration",
		"idle_scale_to_zero", "scale_from_zero", "kubernetes",
	})
}

func TestVersionCompatibility(t *testing.T) {
	cases := []struct {
		peer string
		ok   bool
	}{
		{"1.0.0", true},
		{"1.2.9", true},
		{"v1.5.0", true},
		{"2.0.0", false},
		{"0.9.0", false},
		{"", false},
		{"garbage", false},
	}
	for _, tc := range cases {
		if got := IsCompatibleVersion(tc.peer); got != tc.ok {
			t.Errorf("IsCompatibleVersion(%q) = %v, want %v", tc.peer, got, tc.ok)
		}
	}
}

func TestValidateRejectsIncompatibleVersion(t *testing.T) {
	req := ActivityLeaseRequest{ContractVersion: "2.0.0", Function: "f"}
	err := req.Validate()
	if !errors.Is(err, ErrContractVersionMismatch) {
		t.Fatalf("expected version mismatch error, got %v", err)
	}
}

func TestValidateRejectsMissingFunctionAndNegativeCounts(t *testing.T) {
	if err := (ActivityLeaseRequest{ContractVersion: ContractVersion}).Validate(); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("expected invalid request for missing function, got %v", err)
	}
	bad := ActivityLeaseRequest{ContractVersion: ContractVersion, Function: "f", Running: -1}
	if err := bad.Validate(); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("expected invalid request for negative count, got %v", err)
	}
	badTemporal := ActivityLeaseRequest{ContractVersion: ContractVersion, Function: "f", TemporalDispatched: -1}
	if err := badTemporal.Validate(); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("expected invalid request for negative temporal_dispatched, got %v", err)
	}
}

// TestActivityLeaseSignedInThisRepoVerifies is the local half of the cross-repo
// signing guarantee: a message signed here verifies here, so a message signed in
// AIDrivenMES with the same secret verifies here too (identical canonical recipe).
func TestActivityLeaseSignedInThisRepoVerifies(t *testing.T) {
	secret := "shared-deployment-secret"
	signed := SignActivityLeaseRequest(GoldenActivityLeaseRequest(), secret)
	if err := VerifyActivityLeaseRequest(signed, secret); err != nil {
		t.Fatalf("locally signed request must verify: %v", err)
	}
	if err := VerifyActivityLeaseRequest(signed, "wrong"); err == nil {
		t.Fatal("verification under a different secret must fail")
	}
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return raw
}

func assertJSONKeys(t *testing.T, raw []byte, want []string) {
	t.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("decode into map: %v", err)
	}
	for _, k := range want {
		if _, ok := m[k]; !ok {
			t.Errorf("fixture missing expected key %q", k)
		}
	}
	if len(m) != len(want) {
		t.Errorf("fixture key count = %d, want %d (keys: %v)", len(m), len(want), keysOf(m))
	}
}

func keysOf(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
