package shared

import (
	"encoding/json"
	"maps"
	"strings"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
)

// CloneMap returns a shallow copy of in, or an empty map when in is nil.
func CloneMap(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	return maps.Clone(in)
}

// MapValue returns the first key whose value is a map[string]any, or a JSON
// object string decoded into a map. It returns an empty map when none match.
func MapValue(data map[string]any, keys ...string) map[string]any {
	for _, key := range keys {
		if value, ok := data[key].(map[string]any); ok {
			return value
		}
		if raw, ok := data[key].(string); ok && strings.TrimSpace(raw) != "" {
			var parsed map[string]any
			if json.Unmarshal([]byte(raw), &parsed) == nil {
				return parsed
			}
		}
	}
	return map[string]any{}
}

// RecordID returns the record's ID, falling back to its "id"/"ID" data field.
func RecordID(record contracts.Record[map[string]any]) string {
	return FirstNonEmpty(record.ID, TextValue(record.Data, "id", "ID"))
}
