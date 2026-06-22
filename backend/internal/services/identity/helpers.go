package identity

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

func firstStringSlice(payload map[string]any, keys ...string) []string {
	for _, key := range keys {
		if values := shared.StringSlice(payload[key]); values != nil {
			return values
		}
	}
	return nil
}

func firstMapSlice(payload map[string]any, keys ...string) []map[string]any {
	for _, key := range keys {
		if values := mapSlice(payload[key]); values != nil {
			return values
		}
	}
	return nil
}

func mapSlice(value any) []map[string]any {
	switch items := value.(type) {
	case []map[string]any:
		return items
	case []any:
		out := []map[string]any{}
		for _, item := range items {
			if row, ok := item.(map[string]any); ok {
				out = append(out, row)
			}
		}
		return out
	default:
		return nil
	}
}

func cloneJSONRequest(r *http.Request, payload map[string]any) *http.Request {
	raw, _ := json.Marshal(payload)
	req := r.Clone(r.Context())
	req.Body = io.NopCloser(bytes.NewReader(raw))
	req.ContentLength = int64(len(raw))
	req.Header.Set(headerContentType, contentTypeJSON)
	return req
}

func batchResult() map[string]any {
	return map[string]any{"succeeded": 0, "failed": 0, "items": []any{}, "errors": []any{}}
}

func addBatchResult(result map[string]any, ok bool, data any) {
	if ok {
		result["succeeded"] = result["succeeded"].(int) + 1
		result["items"] = append(result["items"].([]any), data)
		return
	}
	result["failed"] = result["failed"].(int) + 1
	result["errors"] = append(result["errors"].([]any), data)
}

func pathValue(r *http.Request, name string) string {
	return strings.TrimSpace(r.PathValue(name))
}

func positiveQueryInt(r *http.Request, key string, fallback int) int {
	var value int
	if _, err := fmt.Sscanf(strings.TrimSpace(r.URL.Query().Get(key)), "%d", &value); err != nil || value <= 0 {
		return fallback
	}
	return value
}

func tokenExpired(data map[string]any) bool {
	return expiredAt(data, time.Now().UTC())
}

func expiredAt(data map[string]any, now time.Time) bool {
	expiresAt := textValue(data, "expires_at")
	if expiresAt == "" {
		return false
	}
	parsed, err := time.Parse(time.RFC3339, expiresAt)
	return err != nil || now.After(parsed)
}

func decodePayload(r *http.Request) (map[string]any, error) {
	if r.Body == nil {
		return map[string]any{}, nil
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	r.Body.Close()
	r.Body = io.NopCloser(bytes.NewReader(body))
	if len(strings.TrimSpace(string(body))) == 0 {
		return map[string]any{}, nil
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func rawJSON(r *http.Request, status int, data any, cookies []string) (int, platform.RawResponse, *platform.Degraded) {
	body, _ := json.Marshal(platform.Envelope{
		Success:   status < 400,
		Data:      data,
		RequestID: platform.RequestID(r),
		TraceID:   platform.TraceID(r),
	})
	headers := map[string]string{headerContentType: contentTypeJSON}
	values := map[string][]string{}
	if len(cookies) > 0 {
		values["Set-Cookie"] = cookies
	}
	return status, platform.RawResponse{ContentType: contentTypeJSON, Headers: headers, HeaderValues: values, Body: body}, nil
}

func identityEvent(r *http.Request, name string, data map[string]any) contracts.Event {
	traceID := platform.TraceID(r)
	if traceID == "" {
		traceID = platform.NewUUID()
	}
	return contracts.Event{
		EventID:        platform.NewUUID(),
		Name:           name,
		Source:         serviceName,
		OccurredAt:     time.Now().UTC(),
		TraceID:        traceID,
		SchemaVersion:  1,
		IdempotencyKey: r.Header.Get("Idempotency-Key"),
		Data:           data,
	}
}

func publish(app *platform.App, r *http.Request, name string, data map[string]any) {
	if err := app.Events.Publish(r.Context(), identityEvent(r, name, data)); err != nil {
		slog.Error("identity event publish failed", "event", name, "error", err)
	}
}

func nextID(app *platform.App, _ *http.Request, resource, prefix string, base int) string {
	return app.Store.NextID(resource, prefix, base, 0)
}

func randomHex(size int) string {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return hex.EncodeToString([]byte(time.Now().UTC().Format("150405.000000000")))
	}
	return hex.EncodeToString(buf)
}

func randomInt(max int64) int64 {
	if max <= 0 {
		return 0
	}
	n, err := rand.Int(rand.Reader, big.NewInt(max))
	if err != nil {
		return time.Now().UTC().UnixNano() % max
	}
	return n.Int64()
}

func textValue(data map[string]any, key string) string {
	switch value := data[key].(type) {
	case string:
		return value
	case fmt.Stringer:
		return value.String()
	case float64:
		return fmt.Sprintf("%.0f", value)
	case int:
		return fmt.Sprintf("%d", value)
	default:
		return ""
	}
}

func intValue(data map[string]any, key string, fallback int) int {
	switch value := data[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	case json.Number:
		n, err := value.Int64()
		if err == nil {
			return int(n)
		}
	case string:
		switch strings.TrimSpace(value) {
		case "0":
			return 0
		case "1":
			return 1
		case "2":
			return 2
		}
	}
	return fallback
}

func boolValue(data map[string]any, key string) bool {
	switch value := data[key].(type) {
	case bool:
		return value
	case string:
		return strings.EqualFold(value, "true")
	default:
		return false
	}
}
