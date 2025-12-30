package gateway

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/docker-faas/docker-faas/pkg/builder"
	"github.com/docker-faas/docker-faas/pkg/metrics"
	"github.com/docker-faas/docker-faas/pkg/provider"
	"github.com/docker-faas/docker-faas/pkg/store"
	"github.com/docker-faas/docker-faas/pkg/types"
)

type buildRequest struct {
	Name   string          `json:"name"`
	Deploy *bool           `json:"deploy,omitempty"`
	Source buildSourceSpec `json:"source"`
}

type buildSourceSpec struct {
	Type     string            `json:"type"`
	Runtime  string            `json:"runtime,omitempty"`
	Git      *buildGitSource   `json:"git,omitempty"`
	Zip      *buildZipSource   `json:"zip,omitempty"`
	Files    []buildInlineFile `json:"files,omitempty"`
	Manifest string            `json:"manifest,omitempty"`
}

type buildGitSource struct {
	URL  string `json:"url"`
	Ref  string `json:"ref,omitempty"`
	Path string `json:"path,omitempty"`
}

type buildZipSource struct {
	Filename string `json:"filename,omitempty"`
	Data     string `json:"data,omitempty"`
}

type buildInlineFile struct {
	Path    string `json:"path"`
	Content string `json:"content,omitempty"`
	Remove  bool   `json:"remove,omitempty"`
}

type buildInspectFile struct {
	Path     string `json:"path"`
	Content  string `json:"content,omitempty"`
	Editable bool   `json:"editable"`
}

type buildResponse struct {
	Name     string `json:"name"`
	Image    string `json:"image"`
	Deployed bool   `json:"deployed"`
	Updated  bool   `json:"updated"`
}

// HandleBuildFunction handles POST /system/builds (source-to-image).
func (g *Gateway) HandleBuildFunction(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	req, tempDir, cleanup, err := g.parseBuildRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer cleanup()

	buildEntry := BuildEntry{
		Name:       req.Name,
		SourceType: req.Source.Type,
		Runtime:    req.Source.Runtime,
		Status:     "running",
		StartedAt:  start.UTC(),
	}
	if req.Source.Git != nil {
		buildEntry.GitURL = req.Source.Git.URL
		buildEntry.GitRef = req.Source.Git.Ref
		buildEntry.SourcePath = req.Source.Git.Path
	}
	if req.Source.Zip != nil {
		buildEntry.ZipName = req.Source.Zip.Filename
	}
	if g.builds != nil {
		buildEntry = g.builds.Add(buildEntry)
	}

	manifest, dockerfile, contextDir, err := g.prepareBuildContext(r.Context(), tempDir, req, true)
	if err != nil {
		g.logger.Errorf("Build preparation failed: %v", err)
		if g.builds != nil {
			durationMs := int64(time.Since(start).Milliseconds())
			finished := time.Now().UTC()
			msg := err.Error()
			status := "failed"
			g.builds.Update(buildEntry.ID, BuildUpdate{
				Status:     &status,
				FinishedAt: &finished,
				DurationMs: &durationMs,
				Error:      &msg,
			})
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	name := req.Name
	if name == "" && manifest != nil {
		name = manifest.Name
	}
	if name == "" {
		if g.builds != nil {
			durationMs := int64(time.Since(start).Milliseconds())
			finished := time.Now().UTC()
			msg := "name is required (request or docker-faas.yaml)"
			status := "failed"
			g.builds.Update(buildEntry.ID, BuildUpdate{
				Status:     &status,
				FinishedAt: &finished,
				DurationMs: &durationMs,
				Error:      &msg,
			})
		}
		http.Error(w, "name is required (request or docker-faas.yaml)", http.StatusBadRequest)
		return
	}
	if err := validateFunctionName(name); err != nil {
		if g.builds != nil {
			durationMs := int64(time.Since(start).Milliseconds())
			finished := time.Now().UTC()
			msg := err.Error()
			status := "failed"
			g.builds.Update(buildEntry.ID, BuildUpdate{
				Status:     &status,
				FinishedAt: &finished,
				DurationMs: &durationMs,
				Error:      &msg,
			})
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if g.builds != nil {
		update := BuildUpdate{}
		update.Name = &name
		if manifest != nil && manifest.Runtime != "" {
			runtime := manifest.Runtime
			update.Runtime = &runtime
		}
		g.builds.Update(buildEntry.ID, update)
	}

	imageName := buildImageTag(name)
	output := newLimitedBuffer(g.buildOutputLimit)
	if err := builder.BuildImage(r.Context(), g.provider.DockerClient(), contextDir, dockerfile, imageName, g.logger, output); err != nil {
		g.logger.Errorf("Build failed: %v", err)
		if g.builds != nil {
			durationMs := int64(time.Since(start).Milliseconds())
			finished := time.Now().UTC()
			msg := err.Error()
			status := "failed"
			out := output.String()
			truncated := output.Truncated()
			g.builds.Update(buildEntry.ID, BuildUpdate{
				Status:     &status,
				FinishedAt: &finished,
				DurationMs: &durationMs,
				Error:      &msg,
				Output:     &out,
				Truncated:  &truncated,
			})
		}
		http.Error(w, fmt.Sprintf("build failed: %v", err), http.StatusInternalServerError)
		return
	}

	deploy := true
	if req.Deploy != nil {
		deploy = *req.Deploy
	}

	updated := false
	if deploy {
		updated, err = g.deployBuiltImage(r.Context(), name, imageName, manifest)
		if err != nil {
			g.logger.Errorf("Deploy failed: %v", err)
			if g.builds != nil {
				durationMs := int64(time.Since(start).Milliseconds())
				finished := time.Now().UTC()
				msg := err.Error()
				status := "failed"
				out := output.String()
				truncated := output.Truncated()
				g.builds.Update(buildEntry.ID, BuildUpdate{
					Status:     &status,
					FinishedAt: &finished,
					DurationMs: &durationMs,
					Error:      &msg,
					Output:     &out,
					Image:      &imageName,
					Truncated:  &truncated,
				})
			}
			http.Error(w, fmt.Sprintf("deploy failed: %v", err), http.StatusInternalServerError)
			return
		}
	}

	duration := time.Since(start).Seconds()
	g.logger.Infof("Build completed for %s in %.2fs (image: %s)", name, duration, imageName)
	if g.builds != nil {
		durationMs := int64(time.Since(start).Milliseconds())
		finished := time.Now().UTC()
		status := "success"
		out := output.String()
		truncated := output.Truncated()
		deployed := deploy
		g.builds.Update(buildEntry.ID, BuildUpdate{
			Status:     &status,
			FinishedAt: &finished,
			DurationMs: &durationMs,
			Image:      &imageName,
			Deployed:   &deployed,
			Updated:    &updated,
			Output:     &out,
			Truncated:  &truncated,
		})
	}

	g.writeJSON(w, http.StatusAccepted, buildResponse{
		Name:     name,
		Image:    imageName,
		Deployed: deploy,
		Updated:  updated,
	})
}

func (g *Gateway) parseBuildRequest(r *http.Request) (*buildRequest, string, func(), error) {
	tempDir, err := os.MkdirTemp("", "docker-faas-build-")
	if err != nil {
		return nil, "", func() {}, fmt.Errorf("failed to create temp dir: %w", err)
	}

	cleanup := func() {
		_ = os.RemoveAll(tempDir)
	}

	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "multipart/") {
		req, err := g.parseMultipartBuild(r, tempDir)
		if err != nil {
			cleanup()
			return nil, "", func() {}, err
		}
		return req, tempDir, cleanup, nil
	}

	var req buildRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		cleanup()
		return nil, "", func() {}, fmt.Errorf("invalid JSON body: %w", err)
	}

	if req.Source.Type == "" {
		req.Source.Type = "git"
	}

	return &req, tempDir, cleanup, nil
}

// HandleInspectBuild handles POST /system/builds/inspect to preview manifests.
func (g *Gateway) HandleInspectBuild(w http.ResponseWriter, r *http.Request) {
	req, tempDir, cleanup, err := g.parseBuildRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer cleanup()

	manifest, _, contextDir, err := g.prepareBuildContext(r.Context(), tempDir, req, false)
	if err != nil {
		g.logger.Errorf("Inspect preparation failed: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	name := req.Name
	runtime := req.Source.Runtime
	command := ""
	manifestRaw := ""

	if manifest != nil {
		if manifest.Name != "" {
			name = manifest.Name
		}
		if manifest.Runtime != "" {
			runtime = manifest.Runtime
		}
		command = manifest.Command

		manifestPath := filepath.Join(contextDir, "docker-faas.yaml")
		if raw, err := os.ReadFile(manifestPath); err == nil {
			manifestRaw = string(raw)
		}
	}

	files, err := collectSourceFiles(contextDir)
	if err != nil && !errors.Is(err, errInspectFileLimit) {
		g.logger.Warnf("Inspect file list failed: %v", err)
	}

	resp := struct {
		Name     string             `json:"name,omitempty"`
		Runtime  string             `json:"runtime,omitempty"`
		Command  string             `json:"command,omitempty"`
		Manifest string             `json:"manifest,omitempty"`
		Files    []buildInspectFile `json:"files,omitempty"`
	}{
		Name:     name,
		Runtime:  runtime,
		Command:  command,
		Manifest: manifestRaw,
		Files:    files,
	}

	g.writeJSON(w, http.StatusOK, resp)
}

func (g *Gateway) parseMultipartBuild(r *http.Request, tempDir string) (*buildRequest, error) {
	if err := r.ParseMultipartForm(250 << 20); err != nil {
		return nil, fmt.Errorf("invalid multipart form: %w", err)
	}

	name := strings.TrimSpace(r.FormValue("name"))
	runtime := strings.TrimSpace(r.FormValue("runtime"))
	manifest := r.FormValue("manifest")
	deploy := parseBoolPtr(r.FormValue("deploy"))
	sourceType := strings.TrimSpace(r.FormValue("sourceType"))
	if sourceType == "" {
		sourceType = "zip"
	}

	var files []buildInlineFile
	if filesRaw := r.FormValue("files"); filesRaw != "" {
		if err := json.Unmarshal([]byte(filesRaw), &files); err != nil {
			return nil, fmt.Errorf("invalid files payload: %w", err)
		}
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		return nil, fmt.Errorf("zip file is required")
	}
	defer file.Close()

	zipPath := filepath.Join(tempDir, header.Filename)
	out, err := os.Create(zipPath)
	if err != nil {
		return nil, fmt.Errorf("failed to save zip: %w", err)
	}
	if _, err := io.Copy(out, file); err != nil {
		out.Close()
		return nil, fmt.Errorf("failed to write zip: %w", err)
	}
	out.Close()

	req := &buildRequest{
		Name:   name,
		Deploy: deploy,
		Source: buildSourceSpec{
			Type:     sourceType,
			Runtime:  runtime,
			Zip:      &buildZipSource{Filename: header.Filename},
			Files:    files,
			Manifest: manifest,
		},
	}

	return req, nil
}

func (g *Gateway) prepareBuildContext(ctx context.Context, tempDir string, req *buildRequest, generateDockerfile bool) (*builder.Manifest, string, string, error) {
	source := req.Source

	if source.Type == "zip" {
		if source.Zip == nil || source.Zip.Filename == "" {
			return nil, "", "", fmt.Errorf("zip source requires filename")
		}
		zipPath := filepath.Join(tempDir, source.Zip.Filename)
		if err := extractZip(zipPath, tempDir); err != nil {
			return nil, "", "", err
		}
		_ = os.Remove(zipPath)
	} else if source.Type == "git" {
		if source.Git == nil || source.Git.URL == "" {
			return nil, "", "", fmt.Errorf("git source requires url")
		}
		if err := validateGitURL(source.Git.URL); err != nil {
			return nil, "", "", err
		}
		if err := cloneRepo(ctx, source.Git, tempDir); err != nil {
			return nil, "", "", err
		}
	} else if source.Type == "inline" {
		// Inline only: create empty context.
	} else {
		return nil, "", "", fmt.Errorf("unsupported source type: %s", source.Type)
	}

	contextDir := tempDir
	if source.Git != nil && source.Git.Path != "" {
		joined, err := safeJoin(tempDir, source.Git.Path)
		if err != nil {
			return nil, "", "", err
		}
		contextDir = joined
	}

	if err := ensureDir(contextDir); err != nil {
		return nil, "", "", fmt.Errorf("invalid source path: %w", err)
	}

	if source.Type == "zip" && strings.TrimSpace(source.Manifest) != "" {
		if !hasFile(contextDir, "Dockerfile") && !hasFile(contextDir, "docker-faas.yaml") {
			if resolved, err := resolveSingleSubdir(contextDir); err == nil && resolved != contextDir {
				contextDir = resolved
			}
		}
	}

	if len(source.Files) > 0 {
		if err := writeInlineFiles(contextDir, source.Files); err != nil {
			return nil, "", "", err
		}
	}

	manifest, err := g.loadManifest(contextDir, source)
	if err != nil {
		return nil, "", "", err
	}

	if manifest == nil && strings.TrimSpace(source.Manifest) == "" {
		if resolved, err := resolveSingleSubdir(contextDir); err == nil && resolved != contextDir {
			contextDir = resolved
			manifest, err = g.loadManifest(contextDir, source)
			if err != nil {
				return nil, "", "", err
			}
		}
	}

	if manifest == nil && strings.TrimSpace(source.Manifest) == "" {
		if manifestDir, err := findManifestDir(contextDir); err == nil && manifestDir != "" && manifestDir != contextDir {
			contextDir = manifestDir
			manifest, err = g.loadManifest(contextDir, source)
			if err != nil {
				return nil, "", "", err
			}
		}
	}

	if manifest != nil {
		if manifest.Name == "" && req.Name != "" {
			manifest.Name = req.Name
		}
		if manifest.Runtime == "" && source.Runtime != "" {
			manifest.Runtime = source.Runtime
		}
		if manifest.Command == "" {
			return nil, "", "", fmt.Errorf("docker-faas.yaml must define command")
		}
	}

	dockerfile := filepath.Join(contextDir, "Dockerfile")
	if _, err := os.Stat(dockerfile); err != nil && generateDockerfile {
		if os.IsNotExist(err) {
			if manifest == nil {
				if resolved, err := resolveSingleSubdir(contextDir); err == nil && resolved != contextDir {
					contextDir = resolved
					manifest, err = g.loadManifest(contextDir, source)
					if err != nil {
						return nil, "", "", err
					}
				}
			}
			if manifest == nil {
				return nil, "", "", fmt.Errorf("Dockerfile or docker-faas.yaml is required")
			}
			dockerfileBody, err := builder.GenerateDockerfile(manifest, contextDir)
			if err != nil {
				return nil, "", "", err
			}
			if err := os.WriteFile(dockerfile, []byte(dockerfileBody), 0o644); err != nil {
				return nil, "", "", fmt.Errorf("failed to write Dockerfile: %w", err)
			}
		} else {
			return nil, "", "", fmt.Errorf("failed to stat Dockerfile: %w", err)
		}
	}

	return manifest, "Dockerfile", contextDir, nil
}

func (g *Gateway) loadManifest(contextDir string, source buildSourceSpec) (*builder.Manifest, error) {
	manifestPath := filepath.Join(contextDir, "docker-faas.yaml")

	if strings.TrimSpace(source.Manifest) != "" {
		if err := os.WriteFile(manifestPath, []byte(source.Manifest), 0o644); err != nil {
			return nil, fmt.Errorf("failed to write manifest: %w", err)
		}
	}

	if _, err := os.Stat(manifestPath); err == nil {
		return builder.LoadManifest(manifestPath)
	}

	return nil, nil
}

func (g *Gateway) deployBuiltImage(ctx context.Context, name, image string, manifest *builder.Manifest) (bool, error) {
	deployment := types.FunctionDeployment{
		Service: name,
		Image:   image,
	}

	if manifest != nil {
		deployment.Network = manifest.Network
		deployment.EnvProcess = manifest.Command
		deployment.EnvVars = manifest.Env
		deployment.Labels = manifest.Labels
		deployment.Secrets = manifest.Secrets
		deployment.Limits = manifest.Limits
		deployment.Requests = manifest.Requests
		deployment.ReadOnlyRootFilesystem = manifest.ReadOnlyRootFilesystem
		deployment.Debug = manifest.Debug
	}

	if deployment.Network == "" {
		deployment.Network = provider.FunctionNetworkName(g.network, deployment.Service)
	}

	existing, _ := g.store.GetFunction(name)
	if existing != nil {
		if err := g.provider.UpdateFunction(ctx, &deployment, existing.Replicas); err != nil {
			return true, err
		}

		existing.Image = deployment.Image
		existing.EnvProcess = deployment.EnvProcess
		envVars, err := store.EncodeMap(deployment.EnvVars)
		if err != nil {
			return true, fmt.Errorf("failed to encode envVars: %w", err)
		}
		labels, err := store.EncodeMap(deployment.Labels)
		if err != nil {
			return true, fmt.Errorf("failed to encode labels: %w", err)
		}
		secretsJSON, err := store.EncodeSlice(deployment.Secrets)
		if err != nil {
			return true, fmt.Errorf("failed to encode secrets: %w", err)
		}

		existing.EnvVars = envVars
		existing.Labels = labels
		existing.Secrets = secretsJSON
		existing.Network = deployment.Network
		existing.ReadOnly = deployment.ReadOnlyRootFilesystem
		existing.Debug = deployment.Debug

		if deployment.Limits != nil {
			limitsJSON, err := json.Marshal(deployment.Limits)
			if err != nil {
				return true, fmt.Errorf("failed to encode limits: %w", err)
			}
			existing.Limits = string(limitsJSON)
		}
		if deployment.Requests != nil {
			requestsJSON, err := json.Marshal(deployment.Requests)
			if err != nil {
				return true, fmt.Errorf("failed to encode requests: %w", err)
			}
			existing.Requests = string(requestsJSON)
		}

		if err := g.store.UpdateFunction(existing); err != nil {
			return true, err
		}

		metrics.UpdateFunctionReplicas(name, existing.Replicas)
		return true, nil
	}

	replicas := 1
	if err := g.provider.DeployFunction(ctx, &deployment, replicas); err != nil {
		return false, err
	}

	envVars, err := store.EncodeMap(deployment.EnvVars)
	if err != nil {
		return false, fmt.Errorf("failed to encode envVars: %w", err)
	}
	labels, err := store.EncodeMap(deployment.Labels)
	if err != nil {
		return false, fmt.Errorf("failed to encode labels: %w", err)
	}
	secretsJSON, err := store.EncodeSlice(deployment.Secrets)
	if err != nil {
		return false, fmt.Errorf("failed to encode secrets: %w", err)
	}

	metadata := &types.FunctionMetadata{
		Name:       deployment.Service,
		Image:      deployment.Image,
		EnvProcess: deployment.EnvProcess,
		EnvVars:    envVars,
		Labels:     labels,
		Secrets:    secretsJSON,
		Network:    deployment.Network,
		Replicas:   replicas,
		ReadOnly:   deployment.ReadOnlyRootFilesystem,
		Debug:      deployment.Debug,
	}

	if deployment.Limits != nil {
		limitsJSON, err := json.Marshal(deployment.Limits)
		if err != nil {
			return false, fmt.Errorf("failed to encode limits: %w", err)
		}
		metadata.Limits = string(limitsJSON)
	}
	if deployment.Requests != nil {
		requestsJSON, err := json.Marshal(deployment.Requests)
		if err != nil {
			return false, fmt.Errorf("failed to encode requests: %w", err)
		}
		metadata.Requests = string(requestsJSON)
	}

	if err := g.store.CreateFunction(metadata); err != nil {
		g.provider.RemoveFunction(ctx, deployment.Service)
		return false, err
	}

	functions, _ := g.store.ListFunctions()
	metrics.UpdateFunctionsDeployed(len(functions))
	metrics.UpdateFunctionReplicas(deployment.Service, replicas)

	return false, nil
}

func buildImageTag(name string) string {
	safe := strings.ToLower(name)
	safe = strings.ReplaceAll(safe, "_", "-")
	safe = strings.ReplaceAll(safe, " ", "-")
	return fmt.Sprintf("docker-faas/%s:%d", safe, time.Now().Unix())
}

type limitedBuffer struct {
	buf       bytes.Buffer
	limit     int
	truncated bool
}

func newLimitedBuffer(limit int) *limitedBuffer {
	if limit <= 0 {
		limit = 1024
	}
	return &limitedBuffer{limit: limit}
}

func (l *limitedBuffer) Write(p []byte) (int, error) {
	if l.truncated {
		return len(p), nil
	}
	remaining := l.limit - l.buf.Len()
	if remaining <= 0 {
		l.truncated = true
		return len(p), nil
	}
	if len(p) > remaining {
		_, _ = l.buf.Write(p[:remaining])
		l.truncated = true
		return len(p), nil
	}
	return l.buf.Write(p)
}

func (l *limitedBuffer) String() string {
	return l.buf.String()
}

func (l *limitedBuffer) Truncated() bool {
	return l.truncated
}

const (
	maxZipEntries          = 2000
	maxZipFileBytes        = 100 << 20
	maxZipTotalBytes       = 500 << 20
	maxZipCompressionRatio = 100
)

func extractZip(zipPath, dest string) error {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("failed to open zip: %w", err)
	}
	defer reader.Close()

	if err := validateZipArchive(reader.File); err != nil {
		return err
	}

	var extractedTotal uint64
	for _, file := range reader.File {
		if err := extractZipFile(file, dest, &extractedTotal); err != nil {
			return err
		}
	}

	return nil
}

func validateZipArchive(files []*zip.File) error {
	if len(files) > maxZipEntries {
		return fmt.Errorf("zip contains too many entries (max %d)", maxZipEntries)
	}

	var total uint64
	for _, file := range files {
		if file.Name == "" {
			return fmt.Errorf("zip entry has empty name")
		}
		if strings.Contains(file.Name, "\x00") || strings.Contains(file.Name, ":") {
			return fmt.Errorf("zip entry has invalid name: %s", file.Name)
		}
		if file.FileInfo().Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("zip entry uses symlink: %s", file.Name)
		}
		if !file.FileInfo().IsDir() && file.UncompressedSize64 > maxZipFileBytes {
			return fmt.Errorf("zip entry too large: %s", file.Name)
		}
		if file.CompressedSize64 > 0 && file.UncompressedSize64 > 1<<20 {
			ratio := file.UncompressedSize64 / file.CompressedSize64
			if ratio > maxZipCompressionRatio {
				return fmt.Errorf("zip entry compression ratio too high: %s", file.Name)
			}
		}
		total += file.UncompressedSize64
		if total > maxZipTotalBytes {
			return fmt.Errorf("zip uncompressed size exceeds %d bytes", maxZipTotalBytes)
		}
	}
	return nil
}

func extractZipFile(file *zip.File, dest string, extractedTotal *uint64) error {
	path, err := safeJoin(dest, file.Name)
	if err != nil {
		return err
	}

	if file.FileInfo().IsDir() {
		return os.MkdirAll(path, 0o755)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	rc, err := file.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()

	limit := int64(maxZipFileBytes + 1)
	written, err := io.Copy(out, io.LimitReader(rc, limit))
	if err != nil {
		_ = os.Remove(path)
		return err
	}
	if written > int64(maxZipFileBytes) {
		_ = os.Remove(path)
		return fmt.Errorf("zip entry too large: %s", file.Name)
	}
	if extractedTotal != nil {
		*extractedTotal += uint64(written)
		if *extractedTotal > maxZipTotalBytes {
			_ = os.Remove(path)
			return fmt.Errorf("zip uncompressed size exceeds %d bytes", maxZipTotalBytes)
		}
	}

	return nil
}

func writeInlineFiles(base string, files []buildInlineFile) error {
	for _, file := range files {
		if file.Path == "" {
			return fmt.Errorf("inline file path is required")
		}
		path, err := safeJoin(base, file.Path)
		if err != nil {
			return err
		}
		if file.Remove {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("failed to remove %s: %w", file.Path, err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(file.Content), 0o644); err != nil {
			return fmt.Errorf("failed to write %s: %w", file.Path, err)
		}
	}
	return nil
}

func safeJoin(base, target string) (string, error) {
	target = strings.ReplaceAll(target, "\\", "/")
	if strings.Contains(target, ":") {
		return "", fmt.Errorf("invalid path: %s", target)
	}
	clean := filepath.Clean(target)
	if filepath.IsAbs(clean) || strings.HasPrefix(clean, "..") {
		return "", fmt.Errorf("invalid path: %s", target)
	}
	path := filepath.Join(base, clean)
	baseClean := filepath.Clean(base) + string(os.PathSeparator)
	if !strings.HasPrefix(path, baseClean) {
		return "", fmt.Errorf("invalid path: %s", target)
	}
	return path, nil
}

func ensureDir(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", path)
	}
	return nil
}

func resolveSingleSubdir(path string) (string, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return path, err
	}
	if len(entries) != 1 {
		return path, nil
	}
	if entries[0].IsDir() {
		return filepath.Join(path, entries[0].Name()), nil
	}
	return path, nil
}

func findManifestDir(root string) (string, error) {
	var found string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() != "." && shouldSkipInspectDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.EqualFold(d.Name(), "docker-faas.yaml") {
			found = filepath.Dir(path)
			return errManifestFound
		}
		return nil
	})
	if err != nil && !errors.Is(err, errManifestFound) {
		return "", err
	}
	return found, nil
}

func cloneRepo(ctx context.Context, git *buildGitSource, dest string) error {
	args := []string{"clone", "--depth", "1", git.URL, dest}
	if git.Ref != "" {
		args = []string{"clone", "--depth", "1", "--branch", git.Ref, git.URL, dest}
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if git.Ref != "" {
			cmd = exec.CommandContext(ctx, "git", "clone", "--depth", "1", git.URL, dest)
			output, err = cmd.CombinedOutput()
			if err == nil {
				checkout := exec.CommandContext(ctx, "git", "-C", dest, "checkout", git.Ref)
				if out, chkErr := checkout.CombinedOutput(); chkErr != nil {
					return fmt.Errorf("git checkout failed: %s", strings.TrimSpace(string(out)))
				}
				return nil
			}
		}
		return fmt.Errorf("git clone failed: %s", strings.TrimSpace(string(output)))
	}

	return nil
}

func parseBoolPtr(value string) *bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return nil
	}
	return &parsed
}

const (
	maxInspectFiles     = 400
	maxInspectFileBytes = 200 * 1024
)

var errInspectFileLimit = errors.New("inspect file limit reached")
var errManifestFound = errors.New("manifest found")

func collectSourceFiles(root string) ([]buildInspectFile, error) {
	files := make([]buildInspectFile, 0)
	count := 0

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if d.IsDir() {
			if shouldSkipInspectDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		if count >= maxInspectFiles {
			return errInspectFileLimit
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)

		entry := buildInspectFile{
			Path:     rel,
			Content:  "",
			Editable: false,
		}

		if info.Size() <= maxInspectFileBytes {
			data, err := os.ReadFile(path)
			if err == nil && isTextContent(data) {
				entry.Content = string(data)
				entry.Editable = true
			}
		}

		files = append(files, entry)
		count++
		return nil
	})

	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})

	if err != nil && !errors.Is(err, errInspectFileLimit) {
		return files, err
	}
	return files, err
}

func shouldSkipInspectDir(name string) bool {
	switch name {
	case ".git", ".hg", ".svn", "node_modules", "vendor", "dist", "build", "bin", "obj", "target", "__pycache__", ".venv", "venv":
		return true
	default:
		return false
	}
}

func isTextContent(data []byte) bool {
	if len(data) == 0 {
		return true
	}
	if bytes.IndexByte(data, 0) != -1 {
		return false
	}
	return utf8.Valid(data)
}

func hasFile(path, name string) bool {
	_, err := os.Stat(filepath.Join(path, name))
	return err == nil
}
