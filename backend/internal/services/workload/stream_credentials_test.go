package workload

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestStreamCredentialsRequiresOwnedActiveStreamingJob(t *testing.T) {
	app := newStreamCredentialTestApp(true)
	seedWorkloadProject(t, app, "P1")
	seedWorkloadProjectMember(t, app, "P1", "U1")
	createWorkloadRecord(t, app, jobsResource, map[string]any{
		"id":                      "J1",
		"job_id":                  "J1",
		"project_id":              "P1",
		"user_id":                 "U1",
		"status":                  "running",
		"streaming_session":       true,
		"stream_max_bitrate_kbps": 9000,
	})

	rec := serveStreamCredentials(t, app, `{"job_id":"J1","session_id":"browser:one","ttl_seconds":7200}`, "U1", http.StatusOK)
	data := responseEnvelopeData(t, rec)
	turn := data["turn"].(map[string]any)
	username := turn["username"].(string)

	if turn["ttl_seconds"] != float64(3600) {
		t.Fatalf("ttl_seconds = %#v, want capped 3600", turn["ttl_seconds"])
	}
	if !strings.HasSuffix(username, ":U1-browser-one") {
		t.Fatalf("username = %q, want sanitized user/session suffix", username)
	}
	if turn["password"] != streamTURNPassword("turn-secret", username) {
		t.Fatalf("TURN password did not match REST HMAC")
	}
	uris := turn["uris"].([]any)
	if len(uris) != 1 || uris[0] != "turn:turn.example.com:3478?transport=udp" {
		t.Fatalf("uris = %#v, want configured TURN URI", uris)
	}
}

func TestStreamCredentialsTTLsExpireAndDoNotDiscloseSharedSecret(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantTTL int
	}{
		{name: "caps requested ttl", body: `{"job_id":"J1","ttl_seconds":7200}`, wantTTL: 3600},
		{name: "honors lower requested ttl", body: `{"job_id":"J1","ttl_seconds":600}`, wantTTL: 600},
		{name: "defaults missing ttl", body: `{"job_id":"J1"}`, wantTTL: 3600},
		{name: "defaults zero ttl", body: `{"job_id":"J1","ttl_seconds":0}`, wantTTL: 3600},
		{name: "defaults negative ttl", body: `{"job_id":"J1","ttl_seconds":-1}`, wantTTL: 3600},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := newStreamCredentialTestApp(true)
			seedStreamCredentialJob(t, app)
			before := time.Now().UTC()
			rec := serveStreamCredentials(t, app, tt.body, "U1", http.StatusOK)
			after := time.Now().UTC()
			data := responseEnvelopeData(t, rec)
			turn := data["turn"].(map[string]any)
			assertStreamCredentialTTL(t, turn, tt.wantTTL, before, after)
			assertStreamCredentialSecretSafe(t, rec.Body.String(), turn)
		})
	}
}

func TestStreamCredentialsRejectsInvalidSessionState(t *testing.T) {
	tests := []struct {
		name      string
		app       *platform.App
		job       map[string]any
		userID    string
		want      int
		wantError string
	}{
		{
			name:      "not project member",
			app:       newStreamCredentialTestApp(true),
			job:       streamCredentialJobFixture("J1", "running", true),
			userID:    "U2",
			want:      http.StatusForbidden,
			wantError: "project access denied",
		},
		{
			name:      "not streaming job",
			app:       newStreamCredentialTestApp(true),
			job:       streamCredentialJobFixture("J1", "running", false),
			userID:    "U1",
			want:      http.StatusConflict,
			wantError: "job is not a streaming session",
		},
		{
			name:      "inactive job",
			app:       newStreamCredentialTestApp(true),
			job:       streamCredentialJobFixture("J1", "completed", true),
			userID:    "U1",
			want:      http.StatusConflict,
			wantError: "streaming session is not active",
		},
		{
			name:      "turn not configured",
			app:       newStreamCredentialTestApp(false),
			job:       streamCredentialJobFixture("J1", "running", true),
			userID:    "U1",
			want:      http.StatusServiceUnavailable,
			wantError: "stream TURN credentials are not configured",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seedWorkloadProject(t, tt.app, "P1")
			seedWorkloadProjectMember(t, tt.app, "P1", "U1")
			createWorkloadRecord(t, tt.app, jobsResource, tt.job)

			rec := serveStreamCredentials(t, tt.app, `{"job_id":"J1"}`, tt.userID, tt.want)
			data := responseEnvelopeData(t, rec)
			if data["message"] != tt.wantError {
				t.Fatalf("error = %#v, want %q", data, tt.wantError)
			}
		})
	}
}

func TestStreamCredentialsRejectsMissingJob(t *testing.T) {
	app := newStreamCredentialTestApp(true)
	seedWorkloadProject(t, app, "P1")
	seedWorkloadProjectMember(t, app, "P1", "U1")

	rec := serveStreamCredentials(t, app, `{"job_id":"missing"}`, "U1", http.StatusNotFound)
	data := responseEnvelopeData(t, rec)
	if data["message"] != "job not found" {
		t.Fatalf("error = %#v, want job not found", data)
	}
}

func newStreamCredentialTestApp(turnConfigured bool) *platform.App {
	cfg := platform.Config{
		ServiceName:                 "all",
		HTTPAddr:                    ":0",
		RequireAuth:                 true,
		APIKeys:                     map[string]bool{"key-U1": true, "key-U2": true},
		APIKeyPrincipals:            streamCredentialAPIPrincipals(),
		StreamTURNCredentialTTL:     time.Hour,
		StreamMaxBitrateKbps:        12000,
		StreamEgressBudgetKbps:      800000,
		StreamMaxConcurrentSessions: 64,
	}
	if turnConfigured {
		cfg.StreamTURNURIs = []string{"turn:turn.example.com:3478?transport=udp"}
		cfg.StreamTURNSharedSecret = "turn-secret"
	}
	app := platform.NewApp(cfg)
	app.RegisterService(Spec())
	Register(app)
	return app
}

func streamCredentialAPIPrincipals() map[string]platform.APIKeyPrincipal {
	return map[string]platform.APIKeyPrincipal{
		"key-U1": {ID: "U1", Username: "U1", Role: "user"},
		"key-U2": {ID: "U2", Username: "U2", Role: "user"},
	}
}

func streamCredentialJobFixture(id, status string, streaming bool) map[string]any {
	return map[string]any{
		"id":                id,
		"job_id":            id,
		"project_id":        "P1",
		"user_id":           "U1",
		"status":            status,
		"streaming_session": streaming,
	}
}

func seedStreamCredentialJob(t *testing.T, app *platform.App) {
	t.Helper()
	seedWorkloadProject(t, app, "P1")
	seedWorkloadProjectMember(t, app, "P1", "U1")
	createWorkloadRecord(t, app, jobsResource, streamCredentialJobFixture("J1", "running", true))
}

func serveStreamCredentials(t *testing.T, app http.Handler, body, userID string, want int) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/stream/credentials", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", "key-"+userID)
	app.ServeHTTP(rec, req)
	if rec.Code != want {
		t.Fatalf("POST /api/v1/stream/credentials returned %d, want %d: %s", rec.Code, want, rec.Body.String())
	}
	return rec
}

func assertStreamCredentialTTL(t *testing.T, turn map[string]any, wantTTL int, before, after time.Time) {
	t.Helper()
	if turn["ttl_seconds"] != float64(wantTTL) {
		t.Fatalf("ttl_seconds = %#v, want %d", turn["ttl_seconds"], wantTTL)
	}
	expires, err := time.Parse(time.RFC3339, turn["expires_at"].(string))
	if err != nil {
		t.Fatalf("expires_at is not RFC3339: %v", err)
	}
	minExpires := before.Add(time.Duration(wantTTL) * time.Second).Add(-time.Second)
	maxExpires := after.Add(time.Duration(wantTTL) * time.Second).Add(time.Second)
	if expires.Before(minExpires) || expires.After(maxExpires) {
		t.Fatalf("expires_at = %s, want between %s and %s", expires, minExpires, maxExpires)
	}
	username := turn["username"].(string)
	prefix, _, ok := strings.Cut(username, ":")
	if !ok {
		t.Fatalf("username = %q, want expiry prefix", username)
	}
	expiryUnix, err := strconv.ParseInt(prefix, 10, 64)
	if err != nil {
		t.Fatalf("username expiry prefix = %q, want unix timestamp", prefix)
	}
	if expiryUnix != expires.Unix() {
		t.Fatalf("username expiry = %d, want expires_at unix %d", expiryUnix, expires.Unix())
	}
}

func assertStreamCredentialSecretSafe(t *testing.T, responseBody string, turn map[string]any) {
	t.Helper()
	const secret = "turn-secret"
	username := turn["username"].(string)
	password := turn["password"].(string)
	if password != streamTURNPassword(secret, username) {
		t.Fatalf("TURN password did not match REST HMAC")
	}
	if password == secret {
		t.Fatalf("TURN password must not equal shared secret")
	}
	if strings.Contains(responseBody, secret) {
		t.Fatalf("response disclosed TURN shared secret")
	}
}
