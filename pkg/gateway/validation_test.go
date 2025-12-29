package gateway

import "testing"

func TestValidateFunctionName(t *testing.T) {
	valid := []string{"hello", "hello-1", "hello.world", "a_b", "A1", "a-1_b.c"}
	for _, name := range valid {
		if err := validateFunctionName(name); err != nil {
			t.Fatalf("expected valid name %q, got error: %v", name, err)
		}
	}

	invalid := []string{"", "-bad", "..", "bad/one", "bad\\one", "bad:one", " bad"}
	for _, name := range invalid {
		if err := validateFunctionName(name); err == nil {
			t.Fatalf("expected invalid name %q, got no error", name)
		}
	}
}

func TestValidateGitURL(t *testing.T) {
	valid := []string{
		"https://8.8.8.8/repo.git",
		"git://8.8.4.4/repo.git",
		"ssh://8.8.8.8/repo.git",
		"git@8.8.8.8:org/repo.git",
	}
	for _, raw := range valid {
		if err := validateGitURL(raw); err != nil {
			t.Fatalf("expected valid url %q, got error: %v", raw, err)
		}
	}

	invalid := []string{
		"",
		"file:///etc/passwd",
		"http://127.0.0.1/repo.git",
		"http://10.0.0.1/repo.git",
		"git://localhost/repo.git",
		"ssh://[::1]/repo.git",
		"git@localhost:repo.git",
	}
	for _, raw := range invalid {
		if err := validateGitURL(raw); err == nil {
			t.Fatalf("expected invalid url %q, got no error", raw)
		}
	}
}
