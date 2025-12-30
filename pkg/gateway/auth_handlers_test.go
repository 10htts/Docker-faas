package gateway

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/docker-faas/docker-faas/pkg/auth"
)

func TestHandleLogin(t *testing.T) {
	manager := auth.NewManager(time.Minute)
	gw := newTestGateway(&fakeStore{}, &fakeProvider{}, &fakeRouter{})
	gw.SetAuth(manager, "admin", "secret")

	body := map[string]string{"username": "admin", "password": "secret"}
	payload, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/auth/login", bytes.NewReader(payload))
	rr := httptest.NewRecorder()

	gw.HandleLogin(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["token"] == "" {
		t.Fatal("expected token in response")
	}
}

func TestHandleLoginInvalid(t *testing.T) {
	manager := auth.NewManager(time.Minute)
	gw := newTestGateway(&fakeStore{}, &fakeProvider{}, &fakeRouter{})
	gw.SetAuth(manager, "admin", "secret")

	body := map[string]string{"username": "admin", "password": "wrong"}
	payload, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/auth/login", bytes.NewReader(payload))
	rr := httptest.NewRecorder()

	gw.HandleLogin(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestHandleLogout(t *testing.T) {
	manager := auth.NewManager(time.Minute)
	gw := newTestGateway(&fakeStore{}, &fakeProvider{}, &fakeRouter{})
	gw.SetAuth(manager, "admin", "secret")

	token, _, err := manager.Issue("admin")
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	req := httptest.NewRequest("POST", "/auth/logout", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()

	gw.HandleLogout(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rr.Code)
	}
	if _, ok := manager.Validate(token); ok {
		t.Fatal("expected token to be revoked")
	}
}

func TestHandleConfig(t *testing.T) {
	gw := newTestGateway(&fakeStore{}, &fakeProvider{}, &fakeRouter{})
	gw.SetConfigView(&ConfigView{
		AuthEnabled:             true,
		RequireAuthForFunctions: true,
		CORSAllowedOrigins:      []string{"*"},
		FunctionsNetwork:        "docker-faas-net",
		DefaultReplicas:         1,
		MaxReplicas:             10,
		MetricsEnabled:          true,
		MetricsPort:             "9090",
		DebugBindAddress:        "127.0.0.1",
		AuthRateLimit:           10,
		AuthRateWindowSeconds:   60,
		AuthTokenTTLSeconds:     1800,
	})

	req := httptest.NewRequest("GET", "/system/config", nil)
	rr := httptest.NewRecorder()
	gw.HandleConfig(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}
