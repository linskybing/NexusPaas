package platform

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestMemoryRevocationsRevokeAndExpire(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryRevocations()

	if revoked, _ := store.IsRevoked(ctx, "session", "t1"); revoked {
		t.Fatal("fresh token should not be revoked")
	}
	if err := store.Revoke(ctx, "session", "t1", time.Hour); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if revoked, _ := store.IsRevoked(ctx, "session", "t1"); !revoked {
		t.Fatal("token should be revoked after Revoke")
	}
	// A non-positive TTL must still record the revocation (clamped to a minimum).
	if err := store.Revoke(ctx, "api_token", "a1", -time.Second); err != nil {
		t.Fatalf("Revoke negative ttl: %v", err)
	}
	if revoked, _ := store.IsRevoked(ctx, "api_token", "a1"); !revoked {
		t.Fatal("token revoked with clamped ttl should report revoked")
	}
}

func seedSession(t *testing.T, app *App, token, userID string) {
	t.Helper()
	ctx := context.Background()
	if _, err := app.Store.Create(ctx, "identity-service:users", map[string]any{"id": userID, "username": userID, "status": "online", "role": "user"}); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := app.Store.Create(ctx, "identity-service:sessions", map[string]any{
		"id": token, "token": token, "user_id": userID,
		"expires_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339), "revoked": false,
	}); err != nil {
		t.Fatalf("seed session: %v", err)
	}
}

func TestAuthorizeSessionTokenDeniedAfterRevocation(t *testing.T) {
	app := NewApp(Config{RequireAuth: true})
	seedSession(t, app, "access.u1.token", "u1")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/x", nil)

	if !app.authorizeSessionToken(req, "access.u1.token") {
		t.Fatal("valid session should authorize")
	}
	if err := app.Revocations.Revoke(req.Context(), "session", "access.u1.token", time.Hour); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/x", nil)
	if app.authorizeSessionToken(req2, "access.u1.token") {
		t.Fatal("revoked session must be denied")
	}
}

func TestAuthorizeSessionTokenDeniedWhenRevokedFlagSet(t *testing.T) {
	app := NewApp(Config{RequireAuth: true})
	seedSession(t, app, "access.u2.token", "u2")
	app.Store.Update(context.Background(), "identity-service:sessions", "access.u2.token", map[string]any{"revoked": true})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/x", nil)
	if app.authorizeSessionToken(req, "access.u2.token") {
		t.Fatal("session with revoked=true must be denied")
	}
}
