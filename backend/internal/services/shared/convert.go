package shared

import (
	"encoding/json"
	"fmt"
	"strconv"
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

// NumberValueOK returns the first numeric value among the given keys as a
// float64 plus true, or (0, false) when none match. It accepts every Go
// integer/float width, json.Number, and trimmed numeric strings.
func NumberValueOK(data map[string]any, keys ...string) (float64, bool) {
	if data == nil {
		return 0, false
	}
	for _, key := range keys {
		switch value := data[key].(type) {
		case int:
			return float64(value), true
		case int8:
			return float64(value), true
		case int16:
			return float64(value), true
		case int32:
			return float64(value), true
		case int64:
			return float64(value), true
		case uint:
			return float64(value), true
		case uint8:
			return float64(value), true
		case uint16:
			return float64(value), true
		case uint32:
			return float64(value), true
		case uint64:
			return float64(value), true
		case float32:
			return float64(value), true
		case float64:
			return value, true
		case json.Number:
			if parsed, err := value.Float64(); err == nil {
				return parsed, true
			}
		case string:
			if parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64); err == nil {
				return parsed, true
			}
		}
	}
	return 0, false
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
