package platform

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestInternalJSONClientLocalDispatchPreservesHeaders(t *testing.T) {
	app := NewApp(Config{ServiceName: "all", ServiceAPIKey: "svc-key"})
	app.RegisterService(ServiceSpec{Name: "owner-service", Routes: []RouteSpec{{
		Method:       http.MethodPost,
		Pattern:      "/internal/owner/{id}",
		Resource:     "owners",
		Action:       "create",
		PolicyBypass: true,
	}}})
	app.RegisterCustomHandler(http.MethodPost, "/internal/owner/{id}", func(_ *App, r *http.Request, _ RouteSpec) (int, any, *Degraded) {
		if r.Header.Get("X-Service-Key") != "svc-key" {
			return http.StatusUnauthorized, map[string]any{"message": "missing service key"}, nil
		}
		return http.StatusOK, map[string]any{
			"id":           r.PathValue("id"),
			"request_id":   r.Header.Get("X-Request-ID"),
			"idempotency":  r.Header.Get("Idempotency-Key"),
			"content_type": r.Header.Get("Content-Type"),
		}, nil
	})

	var data map[string]any
	resp, err := NewInternalJSONClient(app, "owner-service").Do(context.Background(), InternalJSONRequest{
		Method: http.MethodPost,
		Path:   "/internal/owner/o-1",
		Headers: http.Header{
			"X-Request-ID":    []string{"req-1"},
			"Idempotency-Key": []string{"idem-1"},
		},
		Body:     map[string]any{"ok": true},
		Response: &data,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK || data["id"] != "o-1" || data["request_id"] != "req-1" || data["idempotency"] != "idem-1" || data["content_type"] != "application/json" {
		t.Fatalf("resp=%+v data=%#v, want local envelope data and forwarded headers", resp, data)
	}
}

func TestInternalJSONClientRemoteJoinQueryAndEnvelopeError(t *testing.T) {
	var gotPath, gotQuery, gotKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		gotKey = r.Header.Get("X-Service-Key")
		WriteError(w, r, http.StatusConflict, "conflict", "already exists")
	}))
	defer server.Close()

	app := NewApp(Config{
		ServiceName:   "consumer-service",
		ServiceAPIKey: "svc-key",
		ServiceURLs:   map[string]string{"owner-service": server.URL + "/base"},
	})
	resp, err := NewInternalJSONClient(app, "owner-service").Do(context.Background(), InternalJSONRequest{
		Method: http.MethodGet,
		Path:   "/internal/owner/o-1",
		Query:  url.Values{"filter": []string{"a b"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusConflict || resp.EnvelopeError == nil || resp.EnvelopeError.Message != "already exists" {
		t.Fatalf("resp=%+v, want exposed envelope error", resp)
	}
	if gotPath != "/base/internal/owner/o-1" || gotQuery != "filter=a+b" || gotKey != "svc-key" {
		t.Fatalf("path=%q query=%q key=%q, want joined remote request", gotPath, gotQuery, gotKey)
	}
}
