package service

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
)

var callbackAuthDomainPattern = regexp.MustCompile(
	`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+$`,
)

// NormalizeCallbackAuthAllowedDomains normalizes the callback-domain allowlist
// stored by admins. Items may be plain hosts, URLs, IPs, localhost, or
// wildcard domains in "*.example.com" form.
func NormalizeCallbackAuthAllowedDomains(raw []string) ([]string, error) {
	return normalizeCallbackAuthAllowedDomains(raw, true)
}

// ParseCallbackAuthAllowedDomains parses the persisted JSON allowlist. Invalid
// historical entries are ignored so one bad old value does not break reads.
func ParseCallbackAuthAllowedDomains(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{}
	}
	var items []string
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return []string{}
	}
	normalized, _ := normalizeCallbackAuthAllowedDomains(items, false)
	if len(normalized) == 0 {
		return []string{}
	}
	return normalized
}

// IsCallbackAuthDomainAllowed checks a normalized callback hostname against the
// configured allowlist. Empty allowlist means deny all callback handoff flows.
func IsCallbackAuthDomainAllowed(hostname string, allowlist []string) bool {
	host, err := normalizeCallbackAuthHost(hostname)
	if err != nil || len(allowlist) == 0 {
		return false
	}
	for _, item := range allowlist {
		allowed := strings.ToLower(strings.TrimSpace(item))
		if strings.HasPrefix(allowed, "*.") {
			base := strings.TrimPrefix(allowed, "*.")
			// Wildcard entries intentionally match only subdomains. Add the root
			// domain separately when both example.com and *.example.com are valid.
			if host != base && strings.HasSuffix(host, "."+base) {
				return true
			}
			continue
		}
		if host == allowed {
			return true
		}
	}
	return false
}

// NormalizeCallbackAuthHostname converts a callback URL hostname into the same
// canonical form used by the admin allowlist.
func NormalizeCallbackAuthHostname(raw string) (string, error) {
	return normalizeCallbackAuthHost(raw)
}

func normalizeCallbackAuthAllowedDomains(raw []string, strict bool) ([]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	seen := make(map[string]struct{}, len(raw))
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		normalized, err := normalizeCallbackAuthAllowlistItem(item)
		if err != nil {
			if strict {
				return nil, err
			}
			continue
		}
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

func normalizeCallbackAuthAllowlistItem(raw string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return "", nil
	}

	if strings.HasPrefix(value, "*.") {
		base := strings.TrimPrefix(value, "*.")
		if !isValidCallbackAuthDNSDomain(base) {
			return "", fmt.Errorf("invalid callback domain: %q", raw)
		}
		return "*." + base, nil
	}

	hostSource := value
	if strings.Contains(value, "://") {
		parsed, err := url.Parse(value)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return "", fmt.Errorf("invalid callback domain: %q", raw)
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return "", fmt.Errorf("invalid callback domain scheme: %q", raw)
		}
		hostSource = parsed.Host
	} else if strings.ContainsAny(value, "/?#") {
		return "", fmt.Errorf("invalid callback domain: %q", raw)
	}

	host, err := normalizeCallbackAuthHost(hostSource)
	if err != nil {
		return "", fmt.Errorf("invalid callback domain: %q", raw)
	}
	return host, nil
}

func normalizeCallbackAuthHost(raw string) (string, error) {
	host := strings.TrimSpace(strings.ToLower(raw))
	if host == "" {
		return "", fmt.Errorf("host is required")
	}
	if strings.Contains(host, "@") {
		return "", fmt.Errorf("userinfo is not allowed")
	}

	if h, port, err := net.SplitHostPort(host); err == nil {
		if !isValidCallbackAuthPort(port) {
			return "", fmt.Errorf("invalid port")
		}
		host = h
	} else if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = strings.TrimPrefix(strings.TrimSuffix(host, "]"), "[")
	} else if strings.Count(host, ":") == 1 {
		candidate, port, found := strings.Cut(host, ":")
		if found && candidate != "" && port != "" {
			if !isValidCallbackAuthPort(port) {
				return "", fmt.Errorf("invalid port")
			}
			host = candidate
		}
	}

	host = strings.Trim(host, "[]")
	host = strings.TrimSuffix(host, ".")
	if host == "" {
		return "", fmt.Errorf("host is required")
	}
	if host == "localhost" {
		return host, nil
	}
	if ip := net.ParseIP(host); ip != nil {
		return strings.ToLower(ip.String()), nil
	}
	if isValidCallbackAuthDNSDomain(host) {
		return host, nil
	}
	return "", fmt.Errorf("invalid host")
}

func isValidCallbackAuthPort(raw string) bool {
	if raw == "" {
		return false
	}
	n := 0
	for _, r := range raw {
		if r < '0' || r > '9' {
			return false
		}
		n = n*10 + int(r-'0')
		if n > 65535 {
			return false
		}
	}
	return n > 0
}

func isValidCallbackAuthDNSDomain(domain string) bool {
	return domain != "" &&
		!strings.Contains(domain, "@") &&
		!strings.Contains(domain, "*") &&
		callbackAuthDomainPattern.MatchString(domain)
}
