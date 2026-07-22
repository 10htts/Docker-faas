package gateway

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/docker-faas/docker-faas/pkg/secrets"
	"github.com/sirupsen/logrus"
)

type fakeSecretProvider struct {
	fakeProvider
	sm *secrets.SecretManager
}

func (p *fakeSecretProvider) GetSecretManager() *secrets.SecretManager {
	return p.sm
}

func newSecretTestGateway(t *testing.T) (*Gateway, *secrets.SecretManager) {
	t.Helper()
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	sm, err := secrets.NewSecretManager(t.TempDir(), logger)
	if err != nil {
		t.Fatalf("new secret manager: %v", err)
	}
	return newTestGateway(&fakeStore{}, &fakeSecretProvider{sm: sm}, &fakeRouter{}), sm
}

func TestHandleDeleteSecret_AcceptsFaasCLIJSONBody(t *testing.T) {
	gw, sm := newSecretTestGateway(t)
	if err := sm.CreateSecret("smoke-secret", "plain"); err != nil {
		t.Fatalf("create secret: %v", err)
	}

	body := []byte(`{"name":"smoke-secret","namespace":"openfaas-fn"}`)
	req := httptest.NewRequest(http.MethodDelete, "/system/secrets", bytes.NewReader(body))
	recorder := httptest.NewRecorder()
	gw.HandleDeleteSecret(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected faas-cli-compatible status %d, got %d: %s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if sm.SecretExists("smoke-secret") {
		t.Fatalf("expected secret to be deleted")
	}
}

func TestHandleDeleteSecret_KeepsLegacyQueryName(t *testing.T) {
	gw, sm := newSecretTestGateway(t)
	if err := sm.CreateSecret("legacy-secret", "plain"); err != nil {
		t.Fatalf("create secret: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/system/secrets?name=legacy-secret", nil)
	recorder := httptest.NewRecorder()
	gw.HandleDeleteSecret(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if sm.SecretExists("legacy-secret") {
		t.Fatalf("expected legacy-query secret to be deleted")
	}
}

func TestHandleDeleteSecret_RejectsUnknownNamespace(t *testing.T) {
	gw, sm := newSecretTestGateway(t)
	if err := sm.CreateSecret("namespaced-secret", "plain"); err != nil {
		t.Fatalf("create secret: %v", err)
	}

	body := []byte(`{"name":"namespaced-secret","namespace":"prod"}`)
	req := httptest.NewRequest(http.MethodDelete, "/system/secrets", bytes.NewReader(body))
	recorder := httptest.NewRecorder()
	gw.HandleDeleteSecret(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, recorder.Code)
	}
	if !sm.SecretExists("namespaced-secret") {
		t.Fatalf("rejected namespace must not delete the secret")
	}
}

func TestHandleDeleteSecret_RequiresName(t *testing.T) {
	gw, _ := newSecretTestGateway(t)

	req := httptest.NewRequest(http.MethodDelete, "/system/secrets", bytes.NewReader([]byte(`{}`)))
	recorder := httptest.NewRecorder()
	gw.HandleDeleteSecret(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, recorder.Code)
	}
}
