package helpers

import (
	"strings"

	config "github.com/reyimanuel/letter-administration/internal/infrastructures/config"
)

// ToAbsoluteURL converts a stored file/path value into an absolute URL suitable for browsers.
//
// Behavior:
// - Empty input returns empty.
// - Already-absolute http(s) URLs are returned as-is.
// - Windows backslashes are normalized to '/'.
// - A leading '/' is ensured.
// - The configured BaseURL (config.Get().BaseURL) is prefixed when available.
func ToAbsoluteURL(path string) string {
	p := strings.TrimSpace(path)
	if p == "" {
		return ""
	}

	if strings.HasPrefix(p, "http://") || strings.HasPrefix(p, "https://") {
		return p
	}

	p = strings.ReplaceAll(p, "\\", "/")
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}

	cfg := config.Get()
	if cfg == nil {
		return p
	}

	base := strings.TrimSpace(cfg.BaseURL)
	if base == "" {
		return p
	}
	base = strings.TrimRight(base, "/")

	return base + p
}
