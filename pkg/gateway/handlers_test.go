package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/docker/docker/client"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"

	"github.com/docker-faas/docker-faas/pkg/provider"
	"github.com/docker-faas/docker-faas/pkg/secrets"
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
	return []*types.Container{}, nil
}

func (p *fakeProvider) GetContainerLogs(ctx context.Context, functionName string, tail int) (string, error) {
	if p.getLogsErr != nil {
		return "", p.getLogsErr
	}
	return "", nil
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

func TestHandleInvokeFunction_RoutesRequest(t *testing.T) {
	response := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"X-Test": []string{"true"}},
		Body:       io.NopCloser(strings.NewReader("pong")),
	}

	fs := &fakeStore{functions: make(map[string]*types.FunctionMetadata)}
	fp := &fakeProvider{}
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
