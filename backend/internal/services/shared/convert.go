package shared

import (
	"encoding/json"
	"fmt"
	"strings"
)

// IntValue returns the first integer-coercible value among the given keys,
// or 0 when none match. It accepts int, int64, float64, and json.Number.
func IntValue(data map[string]any, keys ...string) int {
	for _, key := range keys {
		switch value := data[key].(type) {
		case int:
			return value
		case int64:
			return int(value)
		case float64:
			return int(value)
		case json.Number:
			if n, err := value.Int64(); err == nil {
				return int(n)
			}
		}
	}
	return 0
}

// NumberValue returns the first numeric value among the given keys as a
// float64, or 0 when none match.
func NumberValue(data map[string]any, keys ...string) float64 {
	for _, key := range keys {
		switch value := data[key].(type) {
		case float64:
			return value
		case int:
			return float64(value)
		}
	}
	return 0
}

// BoolValue returns the first boolean-coercible value among the given keys.
// Strings are matched case-insensitively against "true" after trimming.
func BoolValue(data map[string]any, keys ...string) bool {
	for _, key := range keys {
		switch value := data[key].(type) {
		case bool:
			return value
		case string:
			return strings.EqualFold(strings.TrimSpace(value), "true")
		}
	}
	return false
}

// StringSlice converts a []string or []any value into a trimmed []string,
// dropping empty entries. It returns nil for any other type.
func StringSlice(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string{}, typed...)
	case []any:
		out := []string{}
		for _, item := range typed {
			text := strings.TrimSpace(fmt.Sprint(item))
			if text != "" {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}
