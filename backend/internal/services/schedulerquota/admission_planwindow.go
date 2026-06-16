package schedulerquota

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

func admissionPlanActive(data map[string]any, now time.Time) bool {
	if validFrom := admissionTimeValue(data, "valid_from", "validFrom", "ValidFrom"); validFrom != nil && now.Before(*validFrom) {
		return false
	}
	if validUntil := admissionTimeValue(data, "valid_until", "validUntil", "ValidUntil"); validUntil != nil && now.After(*validUntil) {
		return false
	}
	return admissionWeekWindowsContain(admissionWeekWindows(data), now)
}

func admissionWeekWindows(data map[string]any) []map[string]any {
	raw, ok := firstPresent(data, "week_windows", "weekWindows", "WeekWindows")
	if !ok || raw == nil {
		return nil
	}
	if windows, ok := raw.([]map[string]any); ok {
		return windows
	}
	items, ok := raw.([]any)
	if !ok {
		if text, ok := raw.(string); ok && strings.TrimSpace(text) != "" {
			var decoded []map[string]any
			if json.Unmarshal([]byte(text), &decoded) == nil {
				return decoded
			}
		}
		return nil
	}
	windows := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if window, ok := item.(map[string]any); ok {
			windows = append(windows, window)
		}
	}
	return windows
}

func admissionWeekWindowsContain(windows []map[string]any, now time.Time) bool {
	if len(windows) == 0 {
		return true
	}
	second := admissionWeekSecond(now)
	for _, window := range windows {
		start := int(getInt64(firstValue(window, "start", "Start"), -1))
		end := int(getInt64(firstValue(window, "end", "End"), -1))
		if start >= 0 && end > start && end <= 604800 && second >= start && second < end {
			return true
		}
	}
	return false
}

func admissionWeekSecond(t time.Time) int {
	utc := t.UTC()
	weekday := (int(utc.Weekday()) + 6) % 7
	return weekday*86400 + utc.Hour()*3600 + utc.Minute()*60 + utc.Second()
}

func admissionStringList(data map[string]any, keys ...string) []string {
	for _, key := range keys {
		if values := shared.StringSlice(data[key]); values != nil {
			return values
		}
		if text := shared.TextValue(data, key); text != "" {
			var decoded []string
			if json.Unmarshal([]byte(text), &decoded) == nil {
				return decoded
			}
		}
	}
	return nil
}

func admissionTimeValue(data map[string]any, keys ...string) *time.Time {
	for _, key := range keys {
		switch value := data[key].(type) {
		case time.Time:
			t := value.UTC()
			return &t
		case string:
			if strings.TrimSpace(value) == "" {
				continue
			}
			if t, err := time.Parse(time.RFC3339, value); err == nil {
				utc := t.UTC()
				return &utc
			}
		}
	}
	return nil
}
