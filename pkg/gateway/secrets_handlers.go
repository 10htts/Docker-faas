package gateway

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
)

// SecretRequest represents a secret create/update request
type SecretRequest struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// SecretResponse represents a secret in responses
type SecretResponse struct {
	Name string `json:"name"`
}

// HandleCreateSecret handles POST /system/secrets
func (g *Gateway) HandleCreateSecret(w http.ResponseWriter, r *http.Request) {
	var req SecretRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Name == "" || req.Value == "" {
		http.Error(w, "Name and value are required", http.StatusBadRequest)
		return
	}

	secretManager := g.provider.GetSecretManager()
	if err := secretManager.CreateSecret(req.Name, req.Value); err != nil {
		g.logger.Errorf("Failed to create secret: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	g.writeJSON(w, http.StatusCreated, SecretResponse{Name: req.Name})
}

// HandleUpdateSecret handles PUT /system/secrets
func (g *Gateway) HandleUpdateSecret(w http.ResponseWriter, r *http.Request) {
	var req SecretRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Name == "" || req.Value == "" {
		http.Error(w, "Name and value are required", http.StatusBadRequest)
		return
	}

	secretManager := g.provider.GetSecretManager()
	if err := secretManager.UpdateSecret(req.Name, req.Value); err != nil {
		g.logger.Errorf("Failed to update secret: %v", err)
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	g.writeJSON(w, http.StatusOK, SecretResponse{Name: req.Name})
}

// HandleDeleteSecret handles DELETE /system/secrets
func (g *Gateway) HandleDeleteSecret(w http.ResponseWriter, r *http.Request) {
	secretName := r.URL.Query().Get("name")
	if secretName == "" {
		http.Error(w, "name parameter is required", http.StatusBadRequest)
		return
	}

	secretManager := g.provider.GetSecretManager()
	if err := secretManager.DeleteSecret(secretName); err != nil {
		g.logger.Errorf("Failed to delete secret: %v", err)
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleListSecrets handles GET /system/secrets
func (g *Gateway) HandleListSecrets(w http.ResponseWriter, r *http.Request) {
	secretManager := g.provider.GetSecretManager()
	secretNames, err := secretManager.ListSecrets()
	if err != nil {
		g.logger.Errorf("Failed to list secrets: %v", err)
		http.Error(w, "Failed to list secrets", http.StatusInternalServerError)
		return
	}

	secrets := make([]SecretResponse, 0, len(secretNames))
	for _, name := range secretNames {
		secrets = append(secrets, SecretResponse{Name: name})
	}

	g.writeJSON(w, http.StatusOK, secrets)
}

// HandleGetSecret handles GET /system/secrets/{name}
func (g *Gateway) HandleGetSecret(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	secretName := vars["name"]

	if secretName == "" {
		http.Error(w, "Secret name is required", http.StatusBadRequest)
		return
	}

	secretManager := g.provider.GetSecretManager()

	// Check if secret exists (don't return the value for security)
	if !secretManager.SecretExists(secretName) {
		http.Error(w, "Secret not found", http.StatusNotFound)
		return
	}

	g.writeJSON(w, http.StatusOK, SecretResponse{Name: secretName})
}
