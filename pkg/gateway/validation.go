package gateway

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
)

var functionNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

func validateFunctionName(name string) error {
	if name == "" {
		return fmt.Errorf("function name is required")
	}
	if !functionNamePattern.MatchString(name) {
		return fmt.Errorf("invalid function name: %s", name)
	}
	return nil
}

func validateGitURL(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fmt.Errorf("git url is required")
	}

	scheme, host, err := parseGitHost(raw)
	if err != nil {
		return err
	}

	if scheme != "" {
		switch scheme {
		case "https", "http", "git", "ssh":
		default:
			return fmt.Errorf("unsupported git url scheme: %s", scheme)
		}
	}

	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if host == "" {
		return fmt.Errorf("git url host is required")
	}
	if isBlockedHostname(host) {
		return fmt.Errorf("git url host is not allowed: %s", host)
	}

	host = stripHostPort(host)
	if ip := net.ParseIP(host); ip != nil {
		if isBlockedIP(ip) {
			return fmt.Errorf("git url host is not allowed: %s", host)
		}
		return nil
	}

	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("failed to resolve git host: %s", host)
	}
	for _, ip := range ips {
		if isBlockedIP(ip) {
			return fmt.Errorf("git url host resolves to private address: %s", host)
		}
	}

	return nil
}

func parseGitHost(raw string) (string, string, error) {
	if strings.Contains(raw, "://") {
		parsed, err := url.Parse(raw)
		if err != nil {
			return "", "", fmt.Errorf("invalid git url: %w", err)
		}
		if parsed.Host == "" {
			return "", "", fmt.Errorf("git url host is required")
		}
		return strings.ToLower(parsed.Scheme), parsed.Host, nil
	}

	if at := strings.LastIndex(raw, "@"); at != -1 {
		rest := raw[at+1:]
		if colon := strings.Index(rest, ":"); colon != -1 {
			host := rest[:colon]
			if host == "" {
				return "", "", fmt.Errorf("git url host is required")
			}
			return "ssh", host, nil
		}
	}

	return "", "", fmt.Errorf("git url must include a scheme or user@host:path")
}

func isBlockedHostname(host string) bool {
	if host == "localhost" || host == "localhost.localdomain" {
		return true
	}
	for _, suffix := range []string{".local", ".internal", ".lan"} {
		if strings.HasSuffix(host, suffix) {
			return true
		}
	}
	return false
}

func stripHostPort(hostport string) string {
	if strings.HasPrefix(hostport, "[") {
		if host, _, err := net.SplitHostPort(hostport); err == nil {
			return strings.Trim(host, "[]")
		}
		return strings.Trim(hostport, "[]")
	}
	if strings.Count(hostport, ":") == 1 {
		if host, _, err := net.SplitHostPort(hostport); err == nil {
			return host
		}
	}
	return hostport
}

func isBlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsMulticast() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}
	if !ip.IsGlobalUnicast() {
		return true
	}
	if ip.IsPrivate() {
		return true
	}
	return false
}
