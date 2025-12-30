package gateway

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expiresAt"`
}

// HandleLogin handles POST /auth/login.
func (g *Gateway) HandleLogin(w http.ResponseWriter, r *http.Request) {
	if g.authMgr == nil {
		http.Error(w, "auth manager not configured", http.StatusServiceUnavailable)
		return
	}

	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	req.Username = strings.TrimSpace(req.Username)

	userMatch := subtle.ConstantTimeCompare([]byte(req.Username), []byte(g.authUser)) == 1
	passMatch := subtle.ConstantTimeCompare([]byte(req.Password), []byte(g.authPass)) == 1

	if !userMatch || !passMatch {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	token, expiresAt, err := g.authMgr.Issue(req.Username)
	if err != nil {
		http.Error(w, "failed to issue token", http.StatusInternalServerError)
		return
	}

	g.writeJSON(w, http.StatusOK, loginResponse{
		Token:     token,
		ExpiresAt: expiresAt.UTC().Format(time.RFC3339),
	})
}

// HandleLogout handles POST /auth/logout.
func (g *Gateway) HandleLogout(w http.ResponseWriter, r *http.Request) {
	if g.authMgr == nil {
		http.Error(w, "auth manager not configured", http.StatusServiceUnavailable)
		return
	}

	token := bearerToken(r.Header.Get("Authorization"))
	if token == "" {
		http.Error(w, "missing token", http.StatusUnauthorized)
		return
	}

	g.authMgr.Revoke(token)
	w.WriteHeader(http.StatusNoContent)
}

// HandleConfig handles GET /system/config.
func (g *Gateway) HandleConfig(w http.ResponseWriter, r *http.Request) {
	if g.config == nil {
		http.Error(w, "config not available", http.StatusNotFound)
		return
	}
	g.writeJSON(w, http.StatusOK, g.config)
}

func bearerToken(header string) string {
	if header == "" {
		return ""
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 {
		return ""
	}
	if !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}
