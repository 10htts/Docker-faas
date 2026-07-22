// Package faascontract defines the ONE canonical, versioned, authenticated wire
// contract shared byte-for-byte between the AIDrivenMES control plane and the
// Docker-faas provider for idle function scale-to-zero coordination (redteam
// objection SZ-12).
//
// Neither repository can safely infer the other's work: AIDrivenMES owns the
// durable-queue state (admitted / queued / running / Temporal-dispatched jobs)
// while Docker-faas owns the authoritative gateway HTTP in-flight count. The idle
// reaper must combine BOTH, so this package carries the exchange as three neutral,
// versioned messages both sides marshal identically:
//
//   - ActivityLeaseRequest  — control-plane -> provider: durable counts + fence.
//   - ActivityLeaseResponse — provider -> control-plane: combined in-flight
//                             accounting, observed replicas, and idle decision.
//   - Capabilities          — provider -> control-plane: capability discovery.
//
// The two authenticated messages (request, response) are signed with HMAC-SHA256
// over a canonical, field-labelled, newline-delimited encoding (NOT the JSON
// bytes, so field ordering / whitespace can never change a signature). A version
// mismatch fails CLOSED: a peer speaking a different MAJOR version must fail
// readiness (the HTTP handler answers 409), never silently disable scale.
//
// This file is byte-identical in both repositories (github.com/docker-faas and
// definitionapp). The committed golden fixtures under testdata/ — signed with
// GoldenFixtureSecret — are the cross-repo conformance vectors: the same bytes
// decode + verify in both repos, or the golden test fails on both.
package faascontract

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ContractVersion is this build's semantic contract version. Compatibility is by
// MAJOR component: a peer with a different major is incompatible and must fail
// readiness (SZ-12).
const ContractVersion = "1.0.0"

// GoldenFixtureSecret is the well-known secret that signs the committed golden
// fixtures (testdata/*.json). It exists ONLY for cross-repo conformance testing;
// production uses a deployment secret. Both repos sign/verify the fixtures with
// this exact value, which is why the same fixture bytes validate on either side.
const GoldenFixtureSecret = "faascontract-v1-golden-fixture-secret"

// Domain-separation tags prefix every canonical signing input so a signature for
// one message type can never be replayed as another.
const (
	domainTagActivityLeaseRequest  = "faascontract-activity-lease-request-v1"
	domainTagActivityLeaseResponse = "faascontract-activity-lease-response-v1"
)

var (
	// ErrContractVersionMismatch is returned when a peer speaks an incompatible
	// contract MAJOR version. Callers must treat it as a readiness failure (409).
	ErrContractVersionMismatch = errors.New("faascontract: incompatible contract version")
	// ErrInvalidRequest is returned when a lease request is structurally invalid.
	ErrInvalidRequest = errors.New("faascontract: invalid activity-lease request")
	// ErrSignatureInvalid is returned when the HMAC does not verify.
	ErrSignatureInvalid = errors.New("faascontract: signature verification failed")
)

// Decision enumerates the provider's idle decision for a function, returned to
// AIDrivenMES so operators can see why a function was (not) reclaimed.
type Decision string

const (
	// DecisionHold means work is in flight (durable or gateway) or the idle
	// window has not elapsed; the function must not be reclaimed.
	DecisionHold Decision = "hold"
	// DecisionScaleToZero means the function is idle and safe to reclaim.
	DecisionScaleToZero Decision = "scale_to_zero"
	// DecisionKeepWarm means an explicit minimum-replica policy pins the function
	// warm (SZ-04); it is never reclaimed to zero.
	DecisionKeepWarm Decision = "keep_warm"
)

// ActivityLeaseRequest is sent by the AIDrivenMES control plane to the provider's
// activity-lease endpoint. It carries the durable-queue state only AIDrivenMES
// can know, plus the generation fence the counts were sampled under (SZ-01/SZ-12).
type ActivityLeaseRequest struct {
	ContractVersion    string    `json:"contract_version"`
	Function           string    `json:"function"`
	OrganizationID     string    `json:"organization_id"`
	Generation         uint64    `json:"generation"`
	Admitted           int       `json:"admitted"`
	Queued             int       `json:"queued"`
	Running            int       `json:"running"`
	TemporalDispatched int       `json:"temporal_dispatched"`
	LeaseTTLSeconds    int       `json:"lease_ttl_seconds"`
	IssuedAt           time.Time `json:"issued_at"`
	LastActivityAt     time.Time `json:"last_activity_at,omitempty"`
	Nonce              string    `json:"nonce"`
	Signature          string    `json:"signature"`
}

// DurableInFlight returns the durable work AIDrivenMES reports admitted, queued,
// or running (temporal-dispatched work is tracked separately by the control
// plane and is not folded into this provider-facing durable count).
func (r ActivityLeaseRequest) DurableInFlight() int {
	return r.Admitted + r.Queued + r.Running
}

// Validate checks structural invariants and version compatibility. It does NOT
// check the signature (use VerifyActivityLeaseRequest / DecodeActivityLeaseRequest
// for authenticated decode).
func (r ActivityLeaseRequest) Validate() error {
	if !IsCompatibleVersion(r.ContractVersion) {
		return fmt.Errorf("%w: peer=%q local=%q", ErrContractVersionMismatch, r.ContractVersion, ContractVersion)
	}
	if strings.TrimSpace(r.Function) == "" {
		return fmt.Errorf("%w: function is required", ErrInvalidRequest)
	}
	if r.Admitted < 0 || r.Queued < 0 || r.Running < 0 || r.TemporalDispatched < 0 {
		return fmt.Errorf("%w: counts must be non-negative", ErrInvalidRequest)
	}
	if r.LeaseTTLSeconds < 0 {
		return fmt.Errorf("%w: lease_ttl_seconds must be non-negative", ErrInvalidRequest)
	}
	return nil
}

// ActivityLeaseResponse is the provider's reply. It echoes the accepted
// generation, the combined in-flight accounting (durable + gateway HTTP), the
// provider-observed replica count, and the resulting idle decision (SZ-12).
type ActivityLeaseResponse struct {
	ContractVersion          string    `json:"contract_version"`
	Function                 string    `json:"function"`
	Accepted                 bool      `json:"accepted"`
	Generation               uint64    `json:"generation"`
	LeaseExpiresAt           time.Time `json:"lease_expires_at,omitempty"`
	GatewayInFlight          int       `json:"gateway_in_flight"`
	DurableInFlight          int       `json:"durable_in_flight"`
	TotalInFlight            int       `json:"total_in_flight"`
	ObservedReplicas         int       `json:"observed_replicas"`
	IdleScaleToZeroSupported bool      `json:"idle_scale_to_zero_supported"`
	Decision                 Decision  `json:"decision"`
	DecisionReason           string    `json:"decision_reason"`
	IssuedAt                 time.Time `json:"issued_at"`
	Nonce                    string    `json:"nonce"`
	Signature                string    `json:"signature"`
}

// Capabilities is returned by the capability-discovery endpoint. Readiness on the
// AIDrivenMES side depends on this declaration rather than an OpenFaaS-compatible
// API name (SZ-05/SZ-10). It is public capability discovery behind endpoint auth
// and is therefore not signed.
type Capabilities struct {
	ContractVersion string               `json:"contract_version"`
	Provider        string               `json:"provider"`
	Orchestration   string               `json:"orchestration"`
	IdleScaleToZero bool                 `json:"idle_scale_to_zero"`
	ScaleFromZero   bool                 `json:"scale_from_zero"`
	Kubernetes      KubernetesCapability `json:"kubernetes"`
}

// KubernetesCapability records the deliberate non-selection of a Kubernetes
// provider. It must never advertise Supported=true from the Docker provider.
type KubernetesCapability struct {
	Supported bool   `json:"supported"`
	Reason    string `json:"reason"`
}

// IsCompatibleVersion reports whether a peer's semantic version shares this
// contract's MAJOR component (same-major is compatible; different-major fails).
func IsCompatibleVersion(peer string) bool {
	peerMajor, ok := majorOf(peer)
	if !ok {
		return false
	}
	localMajor, _ := majorOf(ContractVersion)
	return peerMajor == localMajor
}

func majorOf(v string) (int, bool) {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	if v == "" {
		return 0, false
	}
	parts := strings.SplitN(v, ".", 2)
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, false
	}
	return major, true
}

// canonicalTime renders a timestamp for the signing input: RFC3339Nano in UTC,
// or the empty string for the zero time (so an unset optional timestamp signs
// deterministically on both sides).
func canonicalTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

// canonicalActivityLeaseRequest is the exact signing input for a request:
// domain tag first, then every field except the signature, in struct order,
// '\n'-joined with NO trailing newline.
func canonicalActivityLeaseRequest(r ActivityLeaseRequest) string {
	return strings.Join([]string{
		domainTagActivityLeaseRequest,
		"contract_version=" + r.ContractVersion,
		"function=" + r.Function,
		"organization_id=" + r.OrganizationID,
		"generation=" + strconv.FormatUint(r.Generation, 10),
		"admitted=" + strconv.Itoa(r.Admitted),
		"queued=" + strconv.Itoa(r.Queued),
		"running=" + strconv.Itoa(r.Running),
		"temporal_dispatched=" + strconv.Itoa(r.TemporalDispatched),
		"lease_ttl_seconds=" + strconv.Itoa(r.LeaseTTLSeconds),
		"issued_at=" + canonicalTime(r.IssuedAt),
		"last_activity_at=" + canonicalTime(r.LastActivityAt),
		"nonce=" + r.Nonce,
	}, "\n")
}

// canonicalActivityLeaseResponse is the exact signing input for a response:
// domain tag first, then every field except the signature, in struct order.
func canonicalActivityLeaseResponse(r ActivityLeaseResponse) string {
	return strings.Join([]string{
		domainTagActivityLeaseResponse,
		"contract_version=" + r.ContractVersion,
		"function=" + r.Function,
		"accepted=" + strconv.FormatBool(r.Accepted),
		"generation=" + strconv.FormatUint(r.Generation, 10),
		"lease_expires_at=" + canonicalTime(r.LeaseExpiresAt),
		"gateway_in_flight=" + strconv.Itoa(r.GatewayInFlight),
		"durable_in_flight=" + strconv.Itoa(r.DurableInFlight),
		"total_in_flight=" + strconv.Itoa(r.TotalInFlight),
		"observed_replicas=" + strconv.Itoa(r.ObservedReplicas),
		"idle_scale_to_zero_supported=" + strconv.FormatBool(r.IdleScaleToZeroSupported),
		"decision=" + string(r.Decision),
		"decision_reason=" + r.DecisionReason,
		"issued_at=" + canonicalTime(r.IssuedAt),
		"nonce=" + r.Nonce,
	}, "\n")
}

func computeHMAC(secret, message string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}

// SignActivityLeaseRequest stamps ContractVersion, normalizes timestamps to UTC,
// and computes the signature in place, returning the signed copy.
func SignActivityLeaseRequest(r ActivityLeaseRequest, secret string) ActivityLeaseRequest {
	r.ContractVersion = ContractVersion
	r.IssuedAt = r.IssuedAt.UTC()
	if !r.LastActivityAt.IsZero() {
		r.LastActivityAt = r.LastActivityAt.UTC()
	}
	r.Signature = computeHMAC(secret, canonicalActivityLeaseRequest(r))
	return r
}

// SignActivityLeaseResponse stamps ContractVersion, normalizes timestamps to UTC,
// and computes the signature in place.
func SignActivityLeaseResponse(r ActivityLeaseResponse, secret string) ActivityLeaseResponse {
	r.ContractVersion = ContractVersion
	r.IssuedAt = r.IssuedAt.UTC()
	if !r.LeaseExpiresAt.IsZero() {
		r.LeaseExpiresAt = r.LeaseExpiresAt.UTC()
	}
	r.Signature = computeHMAC(secret, canonicalActivityLeaseResponse(r))
	return r
}

// VerifyActivityLeaseRequest checks version compatibility (fail closed) and the
// HMAC of an already-decoded request.
func VerifyActivityLeaseRequest(r ActivityLeaseRequest, secret string) error {
	if !IsCompatibleVersion(r.ContractVersion) {
		return fmt.Errorf("%w: got %q want %q", ErrContractVersionMismatch, r.ContractVersion, ContractVersion)
	}
	expected := computeHMAC(secret, canonicalActivityLeaseRequest(r))
	if !hmac.Equal([]byte(expected), []byte(r.Signature)) {
		return ErrSignatureInvalid
	}
	return nil
}

// VerifyActivityLeaseResponse checks version compatibility (fail closed) and HMAC.
func VerifyActivityLeaseResponse(r ActivityLeaseResponse, secret string) error {
	if !IsCompatibleVersion(r.ContractVersion) {
		return fmt.Errorf("%w: got %q want %q", ErrContractVersionMismatch, r.ContractVersion, ContractVersion)
	}
	expected := computeHMAC(secret, canonicalActivityLeaseResponse(r))
	if !hmac.Equal([]byte(expected), []byte(r.Signature)) {
		return ErrSignatureInvalid
	}
	return nil
}

// EncodeActivityLeaseRequest signs and JSON-marshals a request.
func EncodeActivityLeaseRequest(r ActivityLeaseRequest, secret string) ([]byte, error) {
	return json.Marshal(SignActivityLeaseRequest(r, secret))
}

// EncodeActivityLeaseResponse signs and JSON-marshals a response.
func EncodeActivityLeaseResponse(r ActivityLeaseResponse, secret string) ([]byte, error) {
	return json.Marshal(SignActivityLeaseResponse(r, secret))
}

// DecodeActivityLeaseRequest unmarshals, then fails closed on version mismatch or
// bad signature.
func DecodeActivityLeaseRequest(data []byte, secret string) (ActivityLeaseRequest, error) {
	var r ActivityLeaseRequest
	if err := json.Unmarshal(data, &r); err != nil {
		return ActivityLeaseRequest{}, fmt.Errorf("faascontract: decode activity-lease request: %w", err)
	}
	if err := VerifyActivityLeaseRequest(r, secret); err != nil {
		return ActivityLeaseRequest{}, err
	}
	return r, nil
}

// DecodeActivityLeaseResponse unmarshals, then fails closed on version mismatch or
// bad signature.
func DecodeActivityLeaseResponse(data []byte, secret string) (ActivityLeaseResponse, error) {
	var r ActivityLeaseResponse
	if err := json.Unmarshal(data, &r); err != nil {
		return ActivityLeaseResponse{}, fmt.Errorf("faascontract: decode activity-lease response: %w", err)
	}
	if err := VerifyActivityLeaseResponse(r, secret); err != nil {
		return ActivityLeaseResponse{}, err
	}
	return r, nil
}

// GoldenActivityLeaseRequest is the canonical (unsigned) example the golden
// request fixture encodes. Both repositories construct this exact struct so the
// committed testdata/activity_lease_request.json — signed with GoldenFixtureSecret
// — is byte-reproducible on either side.
func GoldenActivityLeaseRequest() ActivityLeaseRequest {
	return ActivityLeaseRequest{
		ContractVersion:    ContractVersion,
		Function:           "org7f3a-fn-report-nightly",
		OrganizationID:     "7a1c9d2e-0000-4000-8000-000000000001",
		Generation:         42,
		Admitted:           1,
		Queued:             3,
		Running:            2,
		TemporalDispatched: 1,
		LeaseTTLSeconds:    30,
		IssuedAt:           time.Date(2026, 7, 21, 10, 0, 0, 0, time.UTC),
		LastActivityAt:     time.Date(2026, 7, 21, 9, 59, 58, 0, time.UTC),
		Nonce:              "golden-nonce-req-0001",
	}
}

// GoldenActivityLeaseResponse is the canonical (unsigned) example the golden
// response fixture encodes.
func GoldenActivityLeaseResponse() ActivityLeaseResponse {
	return ActivityLeaseResponse{
		ContractVersion:          ContractVersion,
		Function:                 "org7f3a-fn-report-nightly",
		Accepted:                 true,
		Generation:               42,
		LeaseExpiresAt:           time.Date(2026, 7, 21, 10, 0, 30, 0, time.UTC),
		GatewayInFlight:          0,
		DurableInFlight:          6,
		TotalInFlight:            6,
		ObservedReplicas:         1,
		IdleScaleToZeroSupported: true,
		Decision:                 DecisionHold,
		DecisionReason:           "durable work in flight: admitted=1 queued=3 running=2 temporal=1",
		IssuedAt:                 time.Date(2026, 7, 21, 10, 0, 0, 0, time.UTC),
		Nonce:                    "golden-nonce-resp-0001",
	}
}

// GoldenCapabilities is the canonical example the golden capabilities fixture
// encodes.
func GoldenCapabilities() Capabilities {
	return Capabilities{
		ContractVersion: ContractVersion,
		Provider:        "docker-faas",
		Orchestration:   "docker",
		IdleScaleToZero: true,
		ScaleFromZero:   true,
		Kubernetes: KubernetesCapability{
			Supported: false,
			Reason:    "not selected; Docker is the required first target and no Kubernetes production claim is made from this provider",
		},
	}
}
