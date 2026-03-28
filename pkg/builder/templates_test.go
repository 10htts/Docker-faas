package builder

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGeneratePythonDockerfileUsesUVForRequirements(t *testing.T) {
	t.Parallel()

	contextDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(contextDir, "requirements.txt"), []byte("requests==2.32.0\n"), 0o644); err != nil {
		t.Fatalf("write requirements.txt: %v", err)
	}

	dockerfile, err := GenerateDockerfile(&Manifest{
		Runtime: "python",
		Command: "python handler.py",
	}, contextDir)
	if err != nil {
		t.Fatalf("GenerateDockerfile returned error: %v", err)
	}

	for _, want := range []string{
		"FROM python:3.11-slim AS deps",
		"uv pip install --system --compile -r requirements.txt",
		"COPY --from=deps /usr/local /usr/local",
		"RUN python -m compileall /home/app",
	} {
		if !strings.Contains(dockerfile, want) {
			t.Fatalf("dockerfile missing %q:\n%s", want, dockerfile)
		}
	}

	if strings.Contains(dockerfile, "RUN pip install --no-cache-dir -r requirements.txt") {
		t.Fatalf("dockerfile still uses pip requirements install:\n%s", dockerfile)
	}
}

func TestGeneratePythonDockerfileWithoutRequirementsStaysSingleStage(t *testing.T) {
	t.Parallel()

	dockerfile, err := GenerateDockerfile(&Manifest{
		Runtime: "python",
		Command: "python handler.py",
	}, t.TempDir())
	if err != nil {
		t.Fatalf("GenerateDockerfile returned error: %v", err)
	}

	if strings.Contains(dockerfile, "uv pip install --system --compile -r requirements.txt") {
		t.Fatalf("dockerfile unexpectedly uses uv without requirements:\n%s", dockerfile)
	}

	if strings.Contains(dockerfile, "COPY --from=deps /usr/local /usr/local") {
		t.Fatalf("dockerfile unexpectedly copies dependency stage without requirements:\n%s", dockerfile)
	}

	if !strings.Contains(dockerfile, "RUN python -m compileall /home/app") {
		t.Fatalf("dockerfile missing compileall step:\n%s", dockerfile)
	}
}
