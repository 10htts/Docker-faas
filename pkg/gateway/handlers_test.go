package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/client"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"

	"github.com/docker-faas/docker-faas/pkg/provider"
	"github.com/docker-faas/docker-faas/pkg/secrets"
	"github.com/docker-faas/docker-faas/pkg/store"
	"github.com/docker-faas/docker-faas/pkg/types"
)

type fakeStore struct {
	functions         map[string]*types.FunctionMetadata
	createErr         error
	updateErr         error
	updateReplicasErr error
	listErr           error
	getErr            error
	deleteErr         error

	lastCreated *types.FunctionMetadata
}

func (s *fakeStore) ListFunctions() ([]*types.FunctionMetadata, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	results := make([]*types.FunctionMetadata, 0, len(s.functions))
	for _, fn := range s.functions {
		results = append(results, fn)
	}
	return results, nil
}

func (s *fakeStore) GetFunction(name string) (*types.FunctionMetadata, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	if fn, ok := s.functions[name]; ok {
		return fn, nil
	}
	return nil, errors.New("not found")
}

func (s *fakeStore) CreateFunction(metadata *types.FunctionMetadata) error {
	if s.createErr != nil {
		return s.createErr
	}
	if s.functions == nil {
		s.functions = make(map[string]*types.FunctionMetadata)
	}
	s.functions[metadata.Name] = metadata
	s.lastCreated = metadata
	return nil
}

func (s *fakeStore) UpdateFunction(metadata *types.FunctionMetadata) error {
	if s.updateErr != nil {
		return s.updateErr
	}
	if s.functions == nil {
		s.functions = make(map[string]*types.FunctionMetadata)
	}
	s.functions[metadata.Name] = metadata
	return nil
}

func (s *fakeStore) DeleteFunction(name string) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}
	delete(s.functions, name)
	return nil
}

func (s *fakeStore) UpdateReplicas(name string, replicas int) error {
	if s.updateReplicasErr != nil {
		return s.updateReplicasErr
	}
	fn, ok := s.functions[name]
	if !ok {
		return errors.New("not found")
	}
	fn.Replicas = replicas
	return nil
}

func (s *fakeStore) HealthCheck(ctx context.Context) error {
	return nil
}

type fakeProvider struct {
	deployErr     error
	updateErr     error
	removeErr     error
	scaleErr      error
	getLogsErr    error
	containersErr error
	cleanupErr    error
	healthErr     error
	networkErr    error
	containers    []*types.Container
	logs          string

	deployCalled       bool
	scaleCalled        bool
	lastDeploy         *types.FunctionDeployment
	lastDeployReplicas int
	lastScale          *types.FunctionDeployment
	lastScaleReplicas  int
}

func (p *fakeProvider) DeployFunction(ctx context.Context, deployment *types.FunctionDeployment, replicas int) error {
	p.deployCalled = true
	p.lastDeploy = deployment
	p.lastDeployReplicas = replicas
	return p.deployErr
}

func (p *fakeProvider) UpdateFunction(ctx context.Context, deployment *types.FunctionDeployment, replicas int) error {
	return p.updateErr
}

func (p *fakeProvider) RemoveFunction(ctx context.Context, functionName string) error {
	return p.removeErr
}

func (p *fakeProvider) ScaleFunction(ctx context.Context, deployment *types.FunctionDeployment, targetReplicas int) error {
	p.scaleCalled = true
	p.lastScale = deployment
	p.lastScaleReplicas = targetReplicas
	return p.scaleErr
}

func (p *fakeProvider) GetFunctionContainers(ctx context.Context, functionName string) ([]*types.Container, error) {
	if p.containersErr != nil {
		return nil, p.containersErr
	}
	if p.containers != nil {
		return p.containers, nil
	}
	return []*types.Container{}, nil
}

func (p *fakeProvider) GetContainerLogs(ctx context.Context, functionName string, tail int) (string, error) {
	if p.getLogsErr != nil {
		return "", p.getLogsErr
	}
	return p.logs, nil
}

func (p *fakeProvider) CleanupFunctionNetwork(ctx context.Context, functionName, networkName string) error {
	return p.cleanupErr
}

func (p *fakeProvider) HealthCheck(ctx context.Context) error {
	return p.healthErr
}

func (p *fakeProvider) CheckNetwork(ctx context.Context) error {
	return p.networkErr
}

func (p *fakeProvider) DockerClient() *client.Client {
	return nil
}

func (p *fakeProvider) GetSecretManager() *secrets.SecretManager {
	return nil
}

func (p *fakeProvider) GetGatewayID() string {
	return ""
}

func (p *fakeProvider) CanConnectGateway() bool {
	return false
}

type fakeRouter struct {
	resp         *http.Response
	err          error
	lastFunction string
	lastRequest  *http.Request
}

func (r *fakeRouter) RouteRequest(ctx context.Context, functionName string, req *http.Request) (*http.Response, error) {
	r.lastFunction = functionName
	r.lastRequest = req
	if r.err != nil {
		return nil, r.err
	}
	return r.resp, nil
}

func newTestGateway(store Store, provider Provider, router Router) *Gateway {
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	return NewGateway(store, provider, router, logger, "docker-faas-net")
}

func TestHandleDeployFunction_CreatesFunction(t *testing.T) {
	fs := &fakeStore{functions: make(map[string]*types.FunctionMetadata)}
	fp := &fakeProvider{}
	fr := &fakeRouter{}
	gw := newTestGateway(fs, fp, fr)

	deployment := types.FunctionDeployment{
		Service: "hello",
		Image:   "example/hello:latest",
		EnvVars: map[string]string{"A": "B"},
		Labels:  map[string]string{"team": "dev"},
		Secrets: []string{"secret-1"},
	}
	body, err := json.Marshal(deployment)
	if err != nil {
		t.Fatalf("failed to marshal deployment: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/system/functions", bytes.NewReader(body))
	recorder := httptest.NewRecorder()

	gw.HandleDeployFunction(recorder, req)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, recorder.Code)
	}
	if !fp.deployCalled {
		t.Fatalf("expected provider deploy to be called")
	}
	expectedNetwork := provider.FunctionNetworkName("docker-faas-net", "hello")
	if fp.lastDeploy == nil || fp.lastDeploy.Network != expectedNetwork {
		t.Fatalf("expected network %q, got %#v", expectedNetwork, fp.lastDeploy)
	}
	if fs.lastCreated == nil || fs.lastCreated.Name != "hello" {
		t.Fatalf("expected function metadata to be created")
	}
}

func TestHandleScaleFunction_UpdatesReplicas(t *testing.T) {
	fs := &fakeStore{
		functions: map[string]*types.FunctionMetadata{
			"hello": {
				Name:     "hello",
				Image:    "example/hello:latest",
				Network:  "network",
				Replicas: 1,
			},
		},
	}
	fp := &fakeProvider{}
	gw := newTestGateway(fs, fp, &fakeRouter{})

	payload := []byte(`{"serviceName":"hello","replicas":3}`)
	req := httptest.NewRequest(http.MethodPost, "/system/scale-function/hello", bytes.NewReader(payload))
	recorder := httptest.NewRecorder()

	gw.HandleScaleFunction(recorder, req)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, recorder.Code)
	}
	if !fp.scaleCalled {
		t.Fatalf("expected provider scale to be called")
	}
	if fp.lastScaleReplicas != 3 {
		t.Fatalf("expected scale replicas to be 3, got %d", fp.lastScaleReplicas)
	}
	if fs.functions["hello"].Replicas != 3 {
		t.Fatalf("expected store replicas to be updated to 3")
	}
}

func TestHandleSystemInfo_PinnedProviderInfoShape(t *testing.T) {
	gw := newTestGateway(&fakeStore{}, &fakeProvider{}, &fakeRouter{})

	req := httptest.NewRequest(http.MethodGet, "/system/info", nil)
	recorder := httptest.NewRecorder()
	gw.HandleSystemInfo(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode system info: %v", err)
	}

	providerObj, ok := payload["provider"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected provider object, got %#v", payload["provider"])
	}
	// Pinned faas-provider v0.25.12 ProviderInfo: name under key "provider".
	if providerObj["provider"] != "docker-faas" {
		t.Fatalf("expected provider.provider %q, got %#v", "docker-faas", providerObj["provider"])
	}
	// Legacy additive key keeps older docker-faas clients working.
	if providerObj["name"] != "docker-faas" {
		t.Fatalf("expected provider.name %q, got %#v", "docker-faas", providerObj["name"])
	}
	if providerObj["orchestration"] != "docker" {
		t.Fatalf("expected provider.orchestration %q, got %#v", "docker", providerObj["orchestration"])
	}
	providerVersion, ok := providerObj["version"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected provider.version object, got %#v", providerObj["version"])
	}
	if providerVersion["release"] == "" || providerVersion["release"] == nil {
		t.Fatalf("expected provider.version.release to be set, got %#v", providerVersion["release"])
	}

	version, ok := payload["version"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected top-level version object, got %#v", payload["version"])
	}
	if _, ok := version["release"]; !ok {
		t.Fatalf("expected version.release key")
	}
	if _, ok := version["sha"]; !ok {
		t.Fatalf("expected version.sha key")
	}
	if payload["arch"] != "x86_64" {
		t.Fatalf("expected arch x86_64, got %#v", payload["arch"])
	}
}

func TestHandleListNamespaces_ReturnsDefaultNamespaceList(t *testing.T) {
	gw := newTestGateway(&fakeStore{}, &fakeProvider{}, &fakeRouter{})

	req := httptest.NewRequest(http.MethodGet, "/system/namespaces", nil)
	recorder := httptest.NewRecorder()
	gw.HandleListNamespaces(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	var namespaces []string
	if err := json.Unmarshal(recorder.Body.Bytes(), &namespaces); err != nil {
		t.Fatalf("failed to decode namespaces list: %v", err)
	}
	if len(namespaces) != 1 || namespaces[0] != DefaultFunctionNamespace {
		t.Fatalf("expected [%q], got %v", DefaultFunctionNamespace, namespaces)
	}
}

func newGetFunctionFixture(t *testing.T) (*fakeStore, *fakeProvider, *Gateway) {
	t.Helper()
	labels, err := store.EncodeMap(map[string]string{"team": "dev"})
	if err != nil {
		t.Fatalf("failed to encode labels: %v", err)
	}
	annotations, err := store.EncodeMap(map[string]string{"topic": "events"})
	if err != nil {
		t.Fatalf("failed to encode annotations: %v", err)
	}
	fs := &fakeStore{functions: map[string]*types.FunctionMetadata{
		"hello": {
			Name:        "hello",
			Image:       "example/hello:latest",
			Replicas:    2,
			Labels:      labels,
			Annotations: annotations,
		},
	}}
	fp := &fakeProvider{containers: []*types.Container{{Name: "hello-0", Status: "Up 3 minutes"}}}
	return fs, fp, newTestGateway(fs, fp, &fakeRouter{})
}

func TestHandleGetFunction_ReturnsPinnedStatusShape(t *testing.T) {
	_, _, gw := newGetFunctionFixture(t)

	req := httptest.NewRequest(http.MethodGet, "/system/function/hello", nil)
	req = mux.SetURLVars(req, map[string]string{"name": "hello"})
	recorder := httptest.NewRecorder()
	gw.HandleGetFunction(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, recorder.Code, recorder.Body.String())
	}

	var status map[string]interface{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &status); err != nil {
		t.Fatalf("failed to decode function status: %v", err)
	}
	if status["name"] != "hello" {
		t.Fatalf("expected name hello, got %#v", status["name"])
	}
	if status["image"] != "example/hello:latest" {
		t.Fatalf("expected image, got %#v", status["image"])
	}
	if status["namespace"] != DefaultFunctionNamespace {
		t.Fatalf("expected namespace %q, got %#v", DefaultFunctionNamespace, status["namespace"])
	}
	if status["replicas"] != float64(2) {
		t.Fatalf("expected replicas 2, got %#v", status["replicas"])
	}
	if status["availableReplicas"] != float64(1) {
		t.Fatalf("expected availableReplicas 1, got %#v", status["availableReplicas"])
	}
	labels, ok := status["labels"].(map[string]interface{})
	if !ok || labels["team"] != "dev" {
		t.Fatalf("expected labels.team dev, got %#v", status["labels"])
	}
	annotations, ok := status["annotations"].(map[string]interface{})
	if !ok || annotations["topic"] != "events" {
		t.Fatalf("expected annotations.topic events, got %#v", status["annotations"])
	}
}

func TestHandleGetFunction_NormalizesNamespaceSuffix(t *testing.T) {
	_, _, gw := newGetFunctionFixture(t)

	req := httptest.NewRequest(http.MethodGet, "/system/function/hello.openfaas-fn?namespace=openfaas-fn", nil)
	req = mux.SetURLVars(req, map[string]string{"name": "hello.openfaas-fn"})
	recorder := httptest.NewRecorder()
	gw.HandleGetFunction(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
}

func TestHandleGetFunction_UnknownFunction404(t *testing.T) {
	gw := newTestGateway(&fakeStore{functions: map[string]*types.FunctionMetadata{}}, &fakeProvider{}, &fakeRouter{})

	req := httptest.NewRequest(http.MethodGet, "/system/function/nope", nil)
	req = mux.SetURLVars(req, map[string]string{"name": "nope"})
	recorder := httptest.NewRecorder()
	gw.HandleGetFunction(recorder, req)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, recorder.Code)
	}
	if ct := recorder.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Fatalf("expected plain-text 404 error, got content-type %q", ct)
	}
}

func TestHandleGetFunction_UnknownNamespace400(t *testing.T) {
	_, _, gw := newGetFunctionFixture(t)

	req := httptest.NewRequest(http.MethodGet, "/system/function/hello?namespace=prod", nil)
	req = mux.SetURLVars(req, map[string]string{"name": "hello"})
	recorder := httptest.NewRecorder()
	gw.HandleGetFunction(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "namespace not valid") {
		t.Fatalf("expected 'namespace not valid' error, got %q", recorder.Body.String())
	}
}

func TestHandleListFunctions_UnknownNamespace400(t *testing.T) {
	_, _, gw := newGetFunctionFixture(t)

	req := httptest.NewRequest(http.MethodGet, "/system/functions?namespace=prod", nil)
	recorder := httptest.NewRecorder()
	gw.HandleListFunctions(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, recorder.Code)
	}
}

func TestHandleListFunctions_IncludesNamespaceAndAnnotations(t *testing.T) {
	_, _, gw := newGetFunctionFixture(t)

	req := httptest.NewRequest(http.MethodGet, "/system/functions?namespace=openfaas-fn", nil)
	recorder := httptest.NewRecorder()
	gw.HandleListFunctions(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	var statuses []types.FunctionStatus
	if err := json.Unmarshal(recorder.Body.Bytes(), &statuses); err != nil {
		t.Fatalf("failed to decode list: %v", err)
	}
	if len(statuses) != 1 {
		t.Fatalf("expected one function, got %d", len(statuses))
	}
	if statuses[0].Namespace != DefaultFunctionNamespace {
		t.Fatalf("expected namespace %q, got %q", DefaultFunctionNamespace, statuses[0].Namespace)
	}
	if statuses[0].Annotations["topic"] != "events" {
		t.Fatalf("expected annotations to round-trip, got %#v", statuses[0].Annotations)
	}
}

func deployRequest(t *testing.T, deployment types.FunctionDeployment) *http.Request {
	t.Helper()
	body, err := json.Marshal(deployment)
	if err != nil {
		t.Fatalf("failed to marshal deployment: %v", err)
	}
	return httptest.NewRequest(http.MethodPost, "/system/functions", bytes.NewReader(body))
}

func TestHandleDeployFunction_HonorsOpenFaaSScaleMinLabel(t *testing.T) {
	fs := &fakeStore{functions: make(map[string]*types.FunctionMetadata)}
	fp := &fakeProvider{}
	gw := newTestGateway(fs, fp, &fakeRouter{})

	recorder := httptest.NewRecorder()
	gw.HandleDeployFunction(recorder, deployRequest(t, types.FunctionDeployment{
		Service: "hello",
		Image:   "example/hello:latest",
		Labels:  map[string]string{LabelOpenFaaSScaleMin: "3"},
	}))

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d: %s", http.StatusAccepted, recorder.Code, recorder.Body.String())
	}
	if fp.lastDeployReplicas != 3 {
		t.Fatalf("expected initial replicas 3 from com.openfaas.scale.min, got %d", fp.lastDeployReplicas)
	}
	if fs.lastCreated == nil || fs.lastCreated.Replicas != 3 {
		t.Fatalf("expected stored replicas 3, got %#v", fs.lastCreated)
	}
}

func TestHandleDeployFunction_CustomMinLabelWinsOverOfficial(t *testing.T) {
	fs := &fakeStore{functions: make(map[string]*types.FunctionMetadata)}
	fp := &fakeProvider{}
	gw := newTestGateway(fs, fp, &fakeRouter{})

	recorder := httptest.NewRecorder()
	gw.HandleDeployFunction(recorder, deployRequest(t, types.FunctionDeployment{
		Service: "hello",
		Image:   "example/hello:latest",
		Labels: map[string]string{
			LabelOpenFaaSScaleMin: "4",
			LabelMinReplicas:      "2",
		},
	}))

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, recorder.Code)
	}
	if fp.lastDeployReplicas != 2 {
		t.Fatalf("expected custom min-replicas=2 to win, got %d", fp.lastDeployReplicas)
	}
}

func TestHandleDeployFunction_ClampsMinToConfigMax(t *testing.T) {
	fs := &fakeStore{functions: make(map[string]*types.FunctionMetadata)}
	fp := &fakeProvider{}
	gw := newTestGateway(fs, fp, &fakeRouter{})
	gw.SetConfigView(&ConfigView{MaxReplicas: 4})

	recorder := httptest.NewRecorder()
	gw.HandleDeployFunction(recorder, deployRequest(t, types.FunctionDeployment{
		Service: "hello",
		Image:   "example/hello:latest",
		Labels:  map[string]string{LabelOpenFaaSScaleMin: "50"},
	}))

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, recorder.Code)
	}
	if fp.lastDeployReplicas != 4 {
		t.Fatalf("expected initial replicas clamped to config max 4, got %d", fp.lastDeployReplicas)
	}
}

func TestHandleDeployFunction_ClampsMinToDefaultMaxWithoutConfig(t *testing.T) {
	fs := &fakeStore{functions: make(map[string]*types.FunctionMetadata)}
	fp := &fakeProvider{}
	gw := newTestGateway(fs, fp, &fakeRouter{})

	recorder := httptest.NewRecorder()
	gw.HandleDeployFunction(recorder, deployRequest(t, types.FunctionDeployment{
		Service: "hello",
		Image:   "example/hello:latest",
		Labels:  map[string]string{LabelOpenFaaSScaleMin: "50"},
	}))

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, recorder.Code)
	}
	if fp.lastDeployReplicas != defaultMaxReplicas {
		t.Fatalf("expected initial replicas clamped to default max %d, got %d", defaultMaxReplicas, fp.lastDeployReplicas)
	}
}

func TestHandleDeployFunction_PersistsAnnotations(t *testing.T) {
	fs := &fakeStore{functions: make(map[string]*types.FunctionMetadata)}
	fp := &fakeProvider{}
	gw := newTestGateway(fs, fp, &fakeRouter{})

	recorder := httptest.NewRecorder()
	gw.HandleDeployFunction(recorder, deployRequest(t, types.FunctionDeployment{
		Service:     "hello",
		Image:       "example/hello:latest",
		Annotations: map[string]string{"topic": "events", "com.example/tier": "gold"},
	}))

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, recorder.Code)
	}
	if fs.lastCreated == nil {
		t.Fatalf("expected function metadata to be created")
	}
	decoded := store.DecodeMap(fs.lastCreated.Annotations)
	if decoded["topic"] != "events" || decoded["com.example/tier"] != "gold" {
		t.Fatalf("expected annotations to persist, got %#v", decoded)
	}
}

func TestHandleDeployFunction_NamespaceValidation(t *testing.T) {
	fs := &fakeStore{functions: make(map[string]*types.FunctionMetadata)}
	gw := newTestGateway(fs, &fakeProvider{}, &fakeRouter{})

	recorder := httptest.NewRecorder()
	gw.HandleDeployFunction(recorder, deployRequest(t, types.FunctionDeployment{
		Service:   "hello",
		Image:     "example/hello:latest",
		Namespace: "prod",
	}))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d for unknown namespace, got %d", http.StatusBadRequest, recorder.Code)
	}

	recorder = httptest.NewRecorder()
	gw.HandleDeployFunction(recorder, deployRequest(t, types.FunctionDeployment{
		Service:   "hello",
		Image:     "example/hello:latest",
		Namespace: DefaultFunctionNamespace,
	}))
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("expected status %d for default namespace, got %d", http.StatusAccepted, recorder.Code)
	}
}

func TestHandleUpdateFunction_PersistsAnnotations(t *testing.T) {
	fs := &fakeStore{functions: map[string]*types.FunctionMetadata{
		"hello": {Name: "hello", Image: "example/hello:1", Network: "net", Replicas: 1},
	}}
	gw := newTestGateway(fs, &fakeProvider{}, &fakeRouter{})

	deployment := types.FunctionDeployment{
		Service:     "hello",
		Image:       "example/hello:2",
		Annotations: map[string]string{"topic": "orders"},
	}
	body, err := json.Marshal(deployment)
	if err != nil {
		t.Fatalf("failed to marshal deployment: %v", err)
	}
	req := httptest.NewRequest(http.MethodPut, "/system/functions", bytes.NewReader(body))
	recorder := httptest.NewRecorder()
	gw.HandleUpdateFunction(recorder, req)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d: %s", http.StatusAccepted, recorder.Code, recorder.Body.String())
	}
	decoded := store.DecodeMap(fs.functions["hello"].Annotations)
	if decoded["topic"] != "orders" {
		t.Fatalf("expected updated annotations, got %#v", decoded)
	}
}

func TestHandleDeleteFunction_UnknownNamespace400(t *testing.T) {
	fs := &fakeStore{functions: map[string]*types.FunctionMetadata{
		"hello": {Name: "hello", Image: "example/hello:1", Network: "net", Replicas: 1},
	}}
	gw := newTestGateway(fs, &fakeProvider{}, &fakeRouter{})

	payload := []byte(`{"functionName":"hello","namespace":"prod"}`)
	req := httptest.NewRequest(http.MethodDelete, "/system/functions", bytes.NewReader(payload))
	recorder := httptest.NewRecorder()
	gw.HandleDeleteFunction(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, recorder.Code)
	}
	if _, ok := fs.functions["hello"]; !ok {
		t.Fatalf("expected function to survive rejected delete")
	}
}

func scaleFixture(t *testing.T, labels map[string]string) (*fakeStore, *fakeProvider, *Gateway) {
	t.Helper()
	encoded, err := store.EncodeMap(labels)
	if err != nil {
		t.Fatalf("failed to encode labels: %v", err)
	}
	fs := &fakeStore{functions: map[string]*types.FunctionMetadata{
		"hello": {Name: "hello", Image: "example/hello:latest", Network: "net", Replicas: 1, Labels: encoded},
	}}
	fp := &fakeProvider{}
	return fs, fp, newTestGateway(fs, fp, &fakeRouter{})
}

func TestHandleScaleFunction_ClampsToMaxLabel(t *testing.T) {
	fs, fp, gw := scaleFixture(t, map[string]string{LabelOpenFaaSScaleMax: "2"})

	payload := []byte(`{"serviceName":"hello","replicas":5}`)
	req := httptest.NewRequest(http.MethodPost, "/system/scale-function/hello", bytes.NewReader(payload))
	recorder := httptest.NewRecorder()
	gw.HandleScaleFunction(recorder, req)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d: %s", http.StatusAccepted, recorder.Code, recorder.Body.String())
	}
	if fp.lastScaleReplicas != 2 {
		t.Fatalf("expected replicas clamped to label max 2, got %d", fp.lastScaleReplicas)
	}
	if fs.functions["hello"].Replicas != 2 {
		t.Fatalf("expected stored replicas 2, got %d", fs.functions["hello"].Replicas)
	}
}

func TestHandleScaleFunction_CustomMaxLabelWins(t *testing.T) {
	_, fp, gw := scaleFixture(t, map[string]string{
		LabelOpenFaaSScaleMax: "8",
		LabelMaxReplicas:      "3",
	})

	payload := []byte(`{"serviceName":"hello","replicas":5}`)
	req := httptest.NewRequest(http.MethodPost, "/system/scale-function/hello", bytes.NewReader(payload))
	recorder := httptest.NewRecorder()
	gw.HandleScaleFunction(recorder, req)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, recorder.Code)
	}
	if fp.lastScaleReplicas != 3 {
		t.Fatalf("expected custom max-replicas=3 to win, got %d", fp.lastScaleReplicas)
	}
}

func TestHandleScaleFunction_AllowsExplicitScaleToZero(t *testing.T) {
	fs, fp, gw := scaleFixture(t, map[string]string{LabelOpenFaaSScaleMax: "2"})

	payload := []byte(`{"serviceName":"hello","replicas":0}`)
	req := httptest.NewRequest(http.MethodPost, "/system/scale-function/hello", bytes.NewReader(payload))
	recorder := httptest.NewRecorder()
	gw.HandleScaleFunction(recorder, req)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, recorder.Code)
	}
	if !fp.scaleCalled || fp.lastScaleReplicas != 0 {
		t.Fatalf("expected explicit scale to zero to pass through, got %d", fp.lastScaleReplicas)
	}
	if fs.functions["hello"].Replicas != 0 {
		t.Fatalf("expected stored replicas 0, got %d", fs.functions["hello"].Replicas)
	}
}

func TestHandleScaleFunction_UnknownNamespace400(t *testing.T) {
	_, _, gw := scaleFixture(t, nil)

	payload := []byte(`{"serviceName":"hello","replicas":2,"namespace":"prod"}`)
	req := httptest.NewRequest(http.MethodPost, "/system/scale-function/hello", bytes.NewReader(payload))
	recorder := httptest.NewRecorder()
	gw.HandleScaleFunction(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, recorder.Code)
	}
}

func TestHandleInvokeFunction_StampsCallHeaders(t *testing.T) {
	response := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader("pong")),
	}
	fs := &fakeStore{functions: map[string]*types.FunctionMetadata{
		"hello": {Name: "hello", Image: "alpine:latest", Replicas: 1},
	}}
	fp := &fakeProvider{containers: []*types.Container{{Name: "hello", Status: "running"}}}
	fr := &fakeRouter{resp: response}
	gw := newTestGateway(fs, fp, fr)

	req := httptest.NewRequest(http.MethodPost, "/function/hello", strings.NewReader("ping"))
	req = mux.SetURLVars(req, map[string]string{"name": "hello"})
	recorder := httptest.NewRecorder()
	gw.HandleInvokeFunction(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	callID := recorder.Header().Get("X-Call-Id")
	if callID == "" {
		t.Fatalf("expected X-Call-Id response header")
	}
	if fr.lastRequest == nil || fr.lastRequest.Header.Get("X-Call-Id") != callID {
		t.Fatalf("expected X-Call-Id to be stamped on the upstream request")
	}
	startTime := recorder.Header().Get("X-Start-Time")
	if _, err := strconv.ParseInt(startTime, 10, 64); err != nil {
		t.Fatalf("expected X-Start-Time to be UnixNano, got %q", startTime)
	}
	if fr.lastRequest.Header.Get("X-Start-Time") != startTime {
		t.Fatalf("expected X-Start-Time to be stamped on the upstream request")
	}
	if served := recorder.Header().Get("X-Served-By"); !strings.HasPrefix(served, "docker-faas/") {
		t.Fatalf("expected X-Served-By docker-faas/<version>, got %q", served)
	}
}

func TestHandleInvokeFunction_PreservesCallerCallID(t *testing.T) {
	response := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader("pong")),
	}
	fs := &fakeStore{functions: map[string]*types.FunctionMetadata{
		"hello": {Name: "hello", Image: "alpine:latest", Replicas: 1},
	}}
	fp := &fakeProvider{containers: []*types.Container{{Name: "hello", Status: "running"}}}
	fr := &fakeRouter{resp: response}
	gw := newTestGateway(fs, fp, fr)

	req := httptest.NewRequest(http.MethodPost, "/function/hello", strings.NewReader("ping"))
	req.Header.Set("X-Call-Id", "caller-supplied-id")
	req = mux.SetURLVars(req, map[string]string{"name": "hello"})
	recorder := httptest.NewRecorder()
	gw.HandleInvokeFunction(recorder, req)

	if got := recorder.Header().Get("X-Call-Id"); got != "caller-supplied-id" {
		t.Fatalf("expected caller-supplied X-Call-Id to be preserved, got %q", got)
	}
}

func TestHandleInvokeFunctionAsync_PostsCallback(t *testing.T) {
	type callbackCapture struct {
		headers http.Header
		body    []byte
	}
	captured := make(chan callbackCapture, 1)
	callbackServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		captured <- callbackCapture{headers: r.Header.Clone(), body: body}
	}))
	defer callbackServer.Close()

	response := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"X-Fn-Header": []string{"kept"}},
		Body:       io.NopCloser(strings.NewReader("pong")),
	}
	fs := &fakeStore{functions: map[string]*types.FunctionMetadata{
		"hello": {Name: "hello", Image: "alpine:latest", Replicas: 1},
	}}
	fp := &fakeProvider{containers: []*types.Container{{Name: "hello", Status: "running"}}}
	fr := &fakeRouter{resp: response}
	gw := newTestGateway(fs, fp, fr)

	req := httptest.NewRequest(http.MethodPost, "/async-function/hello", strings.NewReader("ping"))
	req.Header.Set("X-Callback-Url", callbackServer.URL)
	req = mux.SetURLVars(req, map[string]string{"name": "hello"})
	recorder := httptest.NewRecorder()
	gw.HandleInvokeFunctionAsync(recorder, req)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d: %s", http.StatusAccepted, recorder.Code, recorder.Body.String())
	}
	callID := recorder.Header().Get("X-Call-Id")
	if callID == "" {
		t.Fatalf("expected X-Call-Id response header")
	}

	var capture callbackCapture
	select {
	case capture = <-captured:
	case <-time.After(10 * time.Second):
		t.Fatalf("callback was not delivered")
	}

	if got := capture.headers.Get("X-Call-Id"); got != callID {
		t.Fatalf("expected callback X-Call-Id %q, got %q", callID, got)
	}
	if got := capture.headers.Get("X-Function-Name"); got != "hello" {
		t.Fatalf("expected callback X-Function-Name hello, got %q", got)
	}
	if got := capture.headers.Get("X-Function-Status"); got != "200" {
		t.Fatalf("expected callback X-Function-Status 200, got %q", got)
	}
	duration := capture.headers.Get("X-Duration-Seconds")
	if parsed, err := strconv.ParseFloat(duration, 64); err != nil || parsed < 0 {
		t.Fatalf("expected callback X-Duration-Seconds float, got %q (%v)", duration, err)
	}
	if got := capture.headers.Get("X-Fn-Header"); got != "kept" {
		t.Fatalf("expected function response headers copied to callback, got %q", got)
	}
	if string(capture.body) != "pong" {
		t.Fatalf("expected callback body %q, got %q", "pong", string(capture.body))
	}
}

func TestHandleInvokeFunctionAsync_RejectsInvalidCallbackURL(t *testing.T) {
	fs := &fakeStore{functions: map[string]*types.FunctionMetadata{
		"hello": {Name: "hello", Image: "alpine:latest", Replicas: 1},
	}}
	fp := &fakeProvider{containers: []*types.Container{{Name: "hello", Status: "running"}}}
	gw := newTestGateway(fs, fp, &fakeRouter{})

	for _, badURL := range []string{"ftp://example.com/hook", "not a url at all", "file:///etc/passwd", "http://"} {
		req := httptest.NewRequest(http.MethodPost, "/async-function/hello", strings.NewReader("ping"))
		req.Header.Set("X-Callback-Url", badURL)
		req = mux.SetURLVars(req, map[string]string{"name": "hello"})
		recorder := httptest.NewRecorder()
		gw.HandleInvokeFunctionAsync(recorder, req)

		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("expected status %d for callback URL %q, got %d", http.StatusBadRequest, badURL, recorder.Code)
		}
	}
}

func TestHandleInvokeFunction_RoutesRequest(t *testing.T) {
	response := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"X-Test": []string{"true"}},
		Body:       io.NopCloser(strings.NewReader("pong")),
	}

	fs := &fakeStore{functions: map[string]*types.FunctionMetadata{
		"hello": {Name: "hello", Image: "alpine:latest", Replicas: 1},
	}}
	fp := &fakeProvider{
		containers: []*types.Container{{Name: "hello", Status: "running"}},
	}
	fr := &fakeRouter{resp: response}
	gw := newTestGateway(fs, fp, fr)

	req := httptest.NewRequest(http.MethodPost, "/function/hello", strings.NewReader("ping"))
	req = mux.SetURLVars(req, map[string]string{"name": "hello"})
	recorder := httptest.NewRecorder()

	gw.HandleInvokeFunction(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if recorder.Body.String() != "pong" {
		t.Fatalf("expected response body %q, got %q", "pong", recorder.Body.String())
	}
	if recorder.Header().Get("X-Test") != "true" {
		t.Fatalf("expected response header to be forwarded")
	}
	if fr.lastFunction != "hello" {
		t.Fatalf("expected router to be called with function %q, got %q", "hello", fr.lastFunction)
	}
	if fr.lastRequest == nil || fr.lastRequest.Method != http.MethodPost {
		t.Fatalf("expected router to receive original request method")
	}
}
