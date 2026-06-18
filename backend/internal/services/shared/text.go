// Package shared holds small, behavior-stable helpers that were previously
// duplicated across the service handler packages (finding 24). Each function
// preserves the semantics of the copies it replaces; helpers with genuinely
// package-specific behavior intentionally stay in their own package.
package shared

import (
	"fmt"
	"strings"
)

// TextValue returns the first non-empty, trimmed string among the given keys.
// It understands plain strings and fmt.Stringer values, covering both the
// string-only and Stringer-aware variants previously duplicated per package.
// JSON-decoded payloads never carry fmt.Stringer values, so adding Stringer
// support is inert for store-backed data.
func TextValue(data map[string]any, keys ...string) string {
	for _, key := range keys {
		switch value := data[key].(type) {
		case string:
			if trimmed := strings.TrimSpace(value); trimmed != "" {
				return trimmed
			}
		case fmt.Stringer:
			if trimmed := strings.TrimSpace(value.String()); trimmed != "" {
				return trimmed
			}
		}
	}
	return ""
}

// FirstNonEmpty returns the first value that is not the empty string.
func FirstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// FirstNonBlank returns the first value whose trimmed form is not empty.
func FirstNonBlank(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
