package identity

import (
	"context"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestCleanupExpiredAuthRecords(t *testing.T) {
	ctx := context.Background()
	store := platform.NewStore()
	past := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	future := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)

	store.Create(ctx, sessionsResource, map[string]any{"id": "s-old", "expires_at": past})
	store.Create(ctx, sessionsResource, map[string]any{"id": "s-new", "expires_at": future})
	store.Create(ctx, refreshTokensResource, map[string]any{"id": "r-old", "expires_at": past})
	store.Create(ctx, apiTokensResource, map[string]any{"id": "t-old", "expires_at": past})
	store.Create(ctx, apiTokensResource, map[string]any{"id": "t-revoked", "expires_at": future, "revoked": true})
	store.Create(ctx, apiTokensResource, map[string]any{"id": "t-live", "expires_at": future, "revoked": false})

	removed := CleanupExpiredAuthRecords(ctx, store)
	if removed != 4 {
		t.Fatalf("removed = %d, want 4 (s-old, r-old, t-old, t-revoked)", removed)
	}
	if _, ok := store.Get(ctx, sessionsResource, "s-new"); !ok {
		t.Fatal("live session must be retained")
	}
	if _, ok := store.Get(ctx, apiTokensResource, "t-live"); !ok {
		t.Fatal("live api token must be retained")
	}
	if _, ok := store.Get(ctx, apiTokensResource, "t-revoked"); ok {
		t.Fatal("revoked api token should be cleaned up")
	}
}
