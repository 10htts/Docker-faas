package gateway

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/docker-faas/docker-faas/pkg/provider"
	"github.com/docker-faas/docker-faas/pkg/scaletozero"
	"github.com/docker-faas/docker-faas/pkg/store"
	"github.com/docker-faas/docker-faas/pkg/types"
)

// Idle-policy labels. AIDrivenMES is the authoritative policy owner (SZ-11) and
// delivers per-function policy to the provider as deployment labels; the
// provider reconstructs policy from these labels + config defaults at reconcile
// time, so desired state survives a gateway restart (SZ-07).
const (
	// LabelIdleScaleToZero: "true"/"false" enables idle scale-to-zero.
	LabelIdleScaleToZero = "com.docker-faas.scale.idle-to-zero"
	// LabelIdleSeconds: integer idle window in seconds.
	LabelIdleSeconds = "com.docker-faas.scale.idle-seconds"
	// LabelMinReplicas: warm-minimum replicas (SZ-04). > 0 pins the function warm.
	LabelMinReplicas = "com.docker-faas.scale.min-replicas"
	// LabelMaxReplicas: maximum replicas.
	LabelMaxReplicas = "com.docker-faas.scale.max-replicas"
)

// Official OpenFaaS scale labels, pinned against docs.openfaas.com/architecture/
// autoscaling (fetched 2026-07-21) and honored for stack.yml compatibility.
//
// Precedence (documented in redteam/CONFORMANCE_MATRIX.md): when both a custom
// com.docker-faas.scale.* label and its official com.openfaas.scale.*
// counterpart are present, the custom label wins (custom = additive override);
// otherwise the official label applies; otherwise config defaults.
const (
	// LabelOpenFaaSScaleZero: "true"/"1" enables idle scale-to-zero for the
	// function (boolean, parsed with strconv.ParseBool).
	LabelOpenFaaSScaleZero = "com.openfaas.scale.zero"
	// LabelOpenFaaSScaleZeroDuration: idle window before scale-to-zero. Upstream
	// format is a Go duration string ("15m", "10m30s"); bare integers are also
	// accepted as seconds.
	LabelOpenFaaSScaleZeroDuration = "com.openfaas.scale.zero-duration"
	// LabelOpenFaaSScaleMin: minimum replicas (integer >= 1).
	LabelOpenFaaSScaleMin = "com.openfaas.scale.min"
	// LabelOpenFaaSScaleMax: maximum replicas (integer >= 1).
	LabelOpenFaaSScaleMax = "com.openfaas.scale.max"
)

// parseLabelBool parses a boolean label value ("true"/"false"/"1"/"0").
func parseLabelBool(v string) (bool, bool) {
	b, err := strconv.ParseBool(strings.TrimSpace(v))
	if err != nil {
		return false, false
	}
	return b, true
}

// parseLabelInt parses an integer label value, requiring n >= floor.
func parseLabelInt(v string, floor int) (int, bool) {
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n < floor {
		return 0, false
	}
	return n, true
}

// parseScaleZeroDuration parses the com.openfaas.scale.zero-duration value:
// a Go duration string ("15m") or a bare integer number of seconds ("900").
func parseScaleZeroDuration(v string) (time.Duration, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, false
	}
	if d, err := time.ParseDuration(v); err == nil && d > 0 {
		return d, true
	}
	if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
		return time.Duration(secs) * time.Second, true
	}
	return 0, false
}

// scaleBoundsFromLabels resolves the min/max replica bounds for a function from
// its labels. 0 means "unset". Precedence: custom com.docker-faas.scale.*
// labels win over official com.openfaas.scale.* labels.
func scaleBoundsFromLabels(labels map[string]string) (minReplicas, maxReplicas int) {
	if v, ok := labels[LabelOpenFaaSScaleMin]; ok {
		if n, ok := parseLabelInt(v, 1); ok {
			minReplicas = n
		}
	}
	if v, ok := labels[LabelMinReplicas]; ok {
		if n, ok := parseLabelInt(v, 1); ok {
			minReplicas = n
		}
	}
	if v, ok := labels[LabelOpenFaaSScaleMax]; ok {
		if n, ok := parseLabelInt(v, 1); ok {
			maxReplicas = n
		}
	}
	if v, ok := labels[LabelMaxReplicas]; ok {
		if n, ok := parseLabelInt(v, 1); ok {
			maxReplicas = n
		}
	}
	return minReplicas, maxReplicas
}

// PolicyDefaults are the provider-global fallback idle-policy values applied
// when a function carries no per-function label override.
type PolicyDefaults struct {
	// Enabled is the default for idle scale-to-zero when a function does not
	// set the label. The provider ships this OFF by default; enabling is a
	// deliberate operator / control-plane act (see config.IdleScaleToZeroEnabled).
	Enabled      bool
	IdleDuration time.Duration
	MinReplicas  int
	MaxReplicas  int
}

// idleProvider is the low-level provider surface the controller drives. The
// concrete *provider.DockerProvider satisfies it; tests may substitute a fake.
type idleProvider interface {
	ObservedReplicas(ctx context.Context, fn string) (int, error)
	ObservedFunctions(ctx context.Context) ([]string, error)
	ReclaimToZero(ctx context.Context, fn string) (provider.ReclaimResult, error)
	ScaleFunction(ctx context.Context, deployment *types.FunctionDeployment, targetReplicas int) error
}

// IdleController implements scaletozero.ReplicaController and
// scaletozero.PolicySource by combining the provider's Docker operations with
// the function store. It is the single point where the idle reconciler reaches
// the provider; it adds no host-control surface of its own (SZ-06).
type IdleController struct {
	provider idleProvider
	store    Store
	logger   *logrus.Logger
	network  string
	defaults PolicyDefaults
}

var (
	_ scaletozero.ReplicaController = (*IdleController)(nil)
	_ scaletozero.PolicySource      = (*IdleController)(nil)
)

// NewIdleController builds an IdleController.
func NewIdleController(p idleProvider, st Store, network string, defaults PolicyDefaults, logger *logrus.Logger) *IdleController {
	if logger == nil {
		logger = logrus.New()
	}
	return &IdleController{provider: p, store: st, logger: logger, network: network, defaults: defaults}
}

// ObservedReplicas implements scaletozero.ReplicaController.
func (c *IdleController) ObservedReplicas(ctx context.Context, fn string) (int, error) {
	return c.provider.ObservedReplicas(ctx, fn)
}

// ObservedFunctions implements scaletozero.ReplicaController.
func (c *IdleController) ObservedFunctions(ctx context.Context) ([]string, error) {
	return c.provider.ObservedFunctions(ctx)
}

// ReclaimToZero implements scaletozero.ReplicaController.
func (c *IdleController) ReclaimToZero(ctx context.Context, fn string) (scaletozero.ReclaimReport, error) {
	res, err := c.provider.ReclaimToZero(ctx, fn)
	if err != nil {
		return scaletozero.ReclaimReport{}, err
	}
	return scaletozero.ReclaimReport{
		ContainersRemoved: res.ContainersRemoved,
		NetworksRemoved:   res.NetworksRemoved,
		MemoryBytesFreed:  res.MemoryBytesFreed,
		NanoCPUsFreed:     res.NanoCPUsFreed,
	}, nil
}

// EnsureWarmMinimum implements scaletozero.ReplicaController by rebuilding the
// deployment from stored metadata and scaling up to the warm minimum (SZ-04).
func (c *IdleController) EnsureWarmMinimum(ctx context.Context, fn string, minReplicas int) error {
	if minReplicas <= 0 {
		return nil
	}
	metadata, err := c.store.GetFunction(fn)
	if err != nil {
		return err
	}
	deployment := deploymentFromMetadata(metadata)
	if err := c.provider.ScaleFunction(ctx, deployment, minReplicas); err != nil {
		return err
	}
	if err := c.store.UpdateReplicas(fn, minReplicas); err != nil {
		c.logger.Warnf("warm minimum: update replicas for %s: %v", fn, err)
	}
	return nil
}

// DeclaredFunctions implements scaletozero.PolicySource. It reconstructs each
// declared function's idle policy from its labels and the configured defaults,
// so no second policy store is needed for restart convergence (SZ-07).
func (c *IdleController) DeclaredFunctions(ctx context.Context) ([]scaletozero.DeclaredFunction, error) {
	functions, err := c.store.ListFunctions()
	if err != nil {
		return nil, err
	}
	out := make([]scaletozero.DeclaredFunction, 0, len(functions))
	for _, fn := range functions {
		out = append(out, scaletozero.DeclaredFunction{
			Name:   fn.Name,
			Policy: c.policyFor(fn),
		})
	}
	return out, nil
}

// PolicyFor resolves the idle policy for a function by name, falling back to
// configured defaults when the function is unknown. It lets the activity-lease
// handler describe the current decision without duplicating policy derivation.
func (c *IdleController) PolicyFor(fn string) scaletozero.Policy {
	metadata, err := c.store.GetFunction(fn)
	if err != nil {
		return scaletozero.Policy{
			Enabled:      c.defaults.Enabled,
			IdleDuration: c.defaultIdleDuration(),
			MinReplicas:  c.defaults.MinReplicas,
			MaxReplicas:  c.defaults.MaxReplicas,
		}
	}
	return c.policyFor(metadata)
}

func (c *IdleController) defaultIdleDuration() time.Duration {
	if c.defaults.IdleDuration > 0 {
		return c.defaults.IdleDuration
	}
	return 5 * time.Minute
}

// policyFor derives the idle policy for a function from its labels and the
// configured defaults.
//
// Label precedence (documented in redteam/CONFORMANCE_MATRIX.md): official
// com.openfaas.scale.* labels are applied first, then custom
// com.docker-faas.scale.* labels override them when both are present, then any
// remaining unset values fall back to config defaults (already seeded below).
func (c *IdleController) policyFor(fn *types.FunctionMetadata) scaletozero.Policy {
	labels := store.DecodeMap(fn.Labels)

	policy := scaletozero.Policy{
		Enabled:      c.defaults.Enabled,
		IdleDuration: c.defaults.IdleDuration,
		MinReplicas:  c.defaults.MinReplicas,
		MaxReplicas:  c.defaults.MaxReplicas,
	}

	// Official OpenFaaS labels first (lower precedence).
	if v, ok := labels[LabelOpenFaaSScaleZero]; ok {
		if b, ok := parseLabelBool(v); ok {
			policy.Enabled = b
		}
	}
	if v, ok := labels[LabelOpenFaaSScaleZeroDuration]; ok {
		if d, ok := parseScaleZeroDuration(v); ok {
			policy.IdleDuration = d
		}
	}
	if v, ok := labels[LabelOpenFaaSScaleMin]; ok {
		if n, ok := parseLabelInt(v, 1); ok {
			policy.MinReplicas = n
		}
	}
	if v, ok := labels[LabelOpenFaaSScaleMax]; ok {
		if n, ok := parseLabelInt(v, 1); ok {
			policy.MaxReplicas = n
		}
	}

	// Custom com.docker-faas labels win when both are present.
	if v, ok := labels[LabelIdleScaleToZero]; ok {
		if b, err := strconv.ParseBool(strings.TrimSpace(v)); err == nil {
			policy.Enabled = b
		}
	}
	if v, ok := labels[LabelIdleSeconds]; ok {
		if secs, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && secs > 0 {
			policy.IdleDuration = time.Duration(secs) * time.Second
		}
	}
	if v, ok := labels[LabelMinReplicas]; ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n >= 0 {
			policy.MinReplicas = n
		}
	}
	if v, ok := labels[LabelMaxReplicas]; ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 0 {
			policy.MaxReplicas = n
		}
	}

	if policy.IdleDuration <= 0 {
		policy.IdleDuration = 5 * time.Minute
	}
	return policy
}

// deploymentFromMetadata rebuilds a FunctionDeployment from stored metadata.
func deploymentFromMetadata(fn *types.FunctionMetadata) *types.FunctionDeployment {
	deployment := &types.FunctionDeployment{
		Service:                fn.Name,
		Image:                  fn.Image,
		Network:                fn.Network,
		EnvProcess:             fn.EnvProcess,
		EnvVars:                store.DecodeMap(fn.EnvVars),
		Labels:                 store.DecodeMap(fn.Labels),
		Annotations:            store.DecodeMap(fn.Annotations),
		Secrets:                store.DecodeSlice(fn.Secrets),
		ReadOnlyRootFilesystem: fn.ReadOnly,
		Debug:                  fn.Debug,
	}
	if fn.Limits != "" {
		var limits types.FunctionLimits
		if err := json.Unmarshal([]byte(fn.Limits), &limits); err == nil {
			deployment.Limits = &limits
		}
	}
	if fn.Requests != "" {
		var requests types.FunctionResources
		if err := json.Unmarshal([]byte(fn.Requests), &requests); err == nil {
			deployment.Requests = &requests
		}
	}
	return deployment
}
