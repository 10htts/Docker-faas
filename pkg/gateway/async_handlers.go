package gateway

import (
	"bytes"
	"context"
	"io"
	"net/http"

	"github.com/gorilla/mux"
)

// HandleInvokeFunctionAsync handles POST /async-function/{name} and fire-and-forget invocations.
func (g *Gateway) HandleInvokeFunctionAsync(w http.ResponseWriter, r *http.Request) {
	functionName := normalizeFunctionName(mux.Vars(r)["name"])
	if err := validateFunctionName(functionName); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
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
