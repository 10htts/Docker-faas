package gateway

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
)

// HandleInvokeFunctionAsync handles POST /async-function/{name} and fire-and-forget invocations.
func (g *Gateway) HandleInvokeFunctionAsync(w http.ResponseWriter, r *http.Request) {
	functionName := normalizeFunctionName(mux.Vars(r)["name"])
	if err := validateFunctionName(functionName); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Get function metadata
	fn, err := g.store.GetFunction(functionName)
	if err != nil {
		http.Error(w, "Function not found", http.StatusNotFound)
		return
	}

	// Check if function needs to scale up from zero
	containers, err := g.provider.GetFunctionContainers(r.Context(), functionName)
	if err != nil {
		g.logger.Errorf("Failed to get containers for function %s: %v", functionName, err)
		http.Error(w, "Failed to get function containers", http.StatusInternalServerError)
		return
	}

	availableReplicas := 0
	for _, c := range containers {
		if strings.Contains(c.Status, "running") || strings.Contains(c.Status, "Up") {
			availableReplicas++
		}
	}

	if availableReplicas == 0 {
		g.logger.Infof("Scaling function %s from zero for async invocation...", functionName)

		// Start the container
		if err := g.scaleFromZero(r.Context(), fn); err != nil {
			g.logger.Errorf("Failed to scale function %s from zero: %v", functionName, err)
			http.Error(w, "Failed to scale function", http.StatusInternalServerError)
			return
		}

		// Wait for container to be ready (with timeout)
		if err := g.waitForFunctionReady(r.Context(), functionName, 30*time.Second); err != nil {
			g.logger.Errorf("Function %s failed to start: %v", functionName, err)
			http.Error(w, "Function failed to start", http.StatusGatewayTimeout)
			return
		}

		g.logger.Infof("Function %s scaled from zero and ready for async invocation", functionName)
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	callID := generateCallID()

	headers := make(http.Header)
	for key, values := range r.Header {
		for _, value := range values {
			headers.Add(key, value)
		}
	}
	headers.Set("X-Call-Id", callID)

	go func(method string, payload []byte, hdr http.Header) {
		req, err := http.NewRequestWithContext(context.Background(), method, "/", bytes.NewReader(payload))
		if err != nil {
			g.logger.Errorf("Async invoke failed to create request for %s: %v", functionName, err)
			return
		}
		for key, values := range hdr {
			for _, value := range values {
				req.Header.Add(key, value)
			}
		}

		resp, err := g.router.RouteRequest(context.Background(), functionName, req)
		if err != nil {
			g.logger.Errorf("Async invoke failed for %s: %v", functionName, err)
			return
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}(r.Method, body, headers)

	w.Header().Set("X-Call-Id", callID)
	g.writeJSON(w, http.StatusAccepted, map[string]string{
		"status": "accepted",
		"callId": callID,
	})
}
