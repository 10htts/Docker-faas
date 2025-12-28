//go:build integration
// +build integration

package integration

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/docker-faas/docker-faas/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	gatewayURL = "http://localhost:8080"
	username   = "admin"
	password   = "admin"
)

func TestIntegration(t *testing.T) {
	client := &http.Client{
		Timeout: 5 * time.Minute,
	}

	// Wait for gateway to be ready
	waitForGateway(t, client)

	t.Run("SystemInfo", func(t *testing.T) {
		resp, err := makeRequest(client, "GET", gatewayURL+"/system/info", nil)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var info types.SystemInfo
		err = json.NewDecoder(resp.Body).Decode(&info)
		require.NoError(t, err)

		assert.Equal(t, "docker-faas", info.Provider.Name)
		assert.Equal(t, "docker", info.Provider.Orchestration)
	})

	t.Run("Healthz", func(t *testing.T) {
		// Health check should work without auth
		req, _ := http.NewRequest("GET", gatewayURL+"/healthz", nil)
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("DeployFunction", func(t *testing.T) {
		deployment := types.FunctionDeployment{
			Service: "test-echo",
			Image:   "ghcr.io/openfaas/alpine:latest",
			EnvVars: map[string]string{
				"fprocess": "cat",
			},
			Labels: map[string]string{
				"test": "integration",
			},
		}

		body, _ := json.Marshal(deployment)
		resp, err := makeRequest(client, "POST", gatewayURL+"/system/functions", bytes.NewReader(body))
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusAccepted, resp.StatusCode)

		// Wait for function to be deployed
		time.Sleep(5 * time.Second)
	})

	t.Run("ListFunctions", func(t *testing.T) {
		resp, err := makeRequest(client, "GET", gatewayURL+"/system/functions", nil)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var functions []types.FunctionStatus
		err = json.NewDecoder(resp.Body).Decode(&functions)
		require.NoError(t, err)

		assert.NotEmpty(t, functions)
		found := false
		for _, fn := range functions {
			if fn.Name == "test-echo" {
				found = true
				assert.Equal(t, "ghcr.io/openfaas/alpine:latest", fn.Image)
				break
			}
		}
		assert.True(t, found, "test-echo function not found")
	})

	t.Run("InvokeFunction", func(t *testing.T) {
		// Give function time to start
		time.Sleep(2 * time.Second)

		testData := "Hello World"
		resp, err := makeRequest(client, "POST", gatewayURL+"/function/test-echo", bytes.NewReader([]byte(testData)))
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		assert.Contains(t, string(body), testData)
	})

	t.Run("ScaleFunction", func(t *testing.T) {
		scaleReq := types.ScaleServiceRequest{
			ServiceName: "test-echo",
			Replicas:    3,
		}

		body, _ := json.Marshal(scaleReq)
		resp, err := makeRequest(client, "POST", gatewayURL+"/system/scale-function/test-echo", bytes.NewReader(body))
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusAccepted, resp.StatusCode)

		// Wait for scaling
		time.Sleep(3 * time.Second)

		// Verify replicas
		resp, err = makeRequest(client, "GET", gatewayURL+"/system/functions", nil)
		require.NoError(t, err)
		defer resp.Body.Close()

		var functions []types.FunctionStatus
		json.NewDecoder(resp.Body).Decode(&functions)

		for _, fn := range functions {
			if fn.Name == "test-echo" {
				assert.Equal(t, 3, fn.Replicas)
				break
			}
		}
	})

	t.Run("GetLogs", func(t *testing.T) {
		resp, err := makeRequest(client, "GET", gatewayURL+"/system/logs?name=test-echo&tail=10", nil)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		body, _ := io.ReadAll(resp.Body)
		assert.NotEmpty(t, body)
	})

	t.Run("BuildFromZip", func(t *testing.T) {
		zipPayload := buildZipPayload(t)
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)

		require.NoError(t, writer.WriteField("name", "build-zip-echo"))
		require.NoError(t, writer.WriteField("runtime", "python"))
		require.NoError(t, writer.WriteField("sourceType", "zip"))
		require.NoError(t, writer.WriteField("deploy", "true"))

		part, err := writer.CreateFormFile("file", "build-zip-echo.zip")
		require.NoError(t, err)
		_, err = part.Write(zipPayload.Bytes())
		require.NoError(t, err)

		require.NoError(t, writer.Close())

		resp, err := makeMultipartRequest(client, "POST", gatewayURL+"/system/builds", body, writer.FormDataContentType())
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusAccepted, resp.StatusCode)

		waitForFunction(t, client, "build-zip-echo", 2*time.Minute)

		testData := "Zip Build"
		resp, err = makeRequest(client, "POST", gatewayURL+"/function/build-zip-echo", bytes.NewReader([]byte(testData)))
		require.NoError(t, err)
		defer resp.Body.Close()

		responseBody, _ := io.ReadAll(resp.Body)
		assert.Contains(t, string(responseBody), testData)

		resp, err = makeRequest(client, "DELETE", gatewayURL+"/system/functions?functionName=build-zip-echo", nil)
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusAccepted, resp.StatusCode)
	})

	t.Run("DeleteFunction", func(t *testing.T) {
		resp, err := makeRequest(client, "DELETE", gatewayURL+"/system/functions?functionName=test-echo", nil)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusAccepted, resp.StatusCode)

		// Verify deletion
		time.Sleep(2 * time.Second)
		resp, err = makeRequest(client, "GET", gatewayURL+"/system/functions", nil)
		require.NoError(t, err)
		defer resp.Body.Close()

		var functions []types.FunctionStatus
		json.NewDecoder(resp.Body).Decode(&functions)

		for _, fn := range functions {
			assert.NotEqual(t, "test-echo", fn.Name)
		}
	})
}

func makeRequest(client *http.Client, method, url string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}

	req.SetBasicAuth(username, password)
	req.Header.Set("Content-Type", "application/json")

	return client.Do(req)
}

func makeMultipartRequest(client *http.Client, method, url string, body io.Reader, contentType string) (*http.Response, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}

	req.SetBasicAuth(username, password)
	req.Header.Set("Content-Type", contentType)

	return client.Do(req)
}

func buildZipPayload(t *testing.T) *bytes.Buffer {
	t.Helper()

	var buffer bytes.Buffer
	zipWriter := zip.NewWriter(&buffer)

	writeZipFile(t, zipWriter, "docker-faas.yaml", strings.TrimSpace(`
name: build-zip-echo
runtime: python
command: "python handler.py"
`)+"\n")
	writeZipFile(t, zipWriter, "handler.py", strings.TrimSpace(`
import sys


def main():
    payload = sys.stdin.read().strip()
    if payload:
        print("zip-build: " + payload)
    else:
        print("zip-build: ready")


if __name__ == "__main__":
    main()
`)+"\n")

	require.NoError(t, zipWriter.Close())
	return &buffer
}

func writeZipFile(t *testing.T, zipWriter *zip.Writer, name, content string) {
	t.Helper()

	writer, err := zipWriter.Create(name)
	require.NoError(t, err)
	_, err = writer.Write([]byte(content))
	require.NoError(t, err)
}

func waitForFunction(t *testing.T, client *http.Client, name string, timeout time.Duration) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			t.Fatalf("Function %s did not become ready in time", name)
		case <-ticker.C:
			resp, err := makeRequest(client, "GET", gatewayURL+"/system/functions", nil)
			if err != nil {
				continue
			}
			funcs := []types.FunctionStatus{}
			if err := json.NewDecoder(resp.Body).Decode(&funcs); err == nil {
				for _, fn := range funcs {
					if fn.Name == name {
						resp.Body.Close()
						return
					}
				}
			}
			resp.Body.Close()
		}
	}
}

func waitForGateway(t *testing.T, client *http.Client) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			t.Fatal("Gateway did not become ready in time")
		case <-ticker.C:
			req, _ := http.NewRequest("GET", gatewayURL+"/healthz", nil)
			resp, err := client.Do(req)
			if err == nil && resp.StatusCode == http.StatusOK {
				resp.Body.Close()
				return
			}
			if resp != nil {
				resp.Body.Close()
			}
		}
	}
}
