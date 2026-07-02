package identity

import (
	"context"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestIdentityAuthRepositoryAPITokenActiveLifecycle(t *testing.T) {
	fixture := newAuthRepoAPITokenFixture(t)

	t.Run("stores hash only and returns raw token once", func(t *testing.T) {
		assertAPITokenCreateStoresHashOnly(t, fixture)
	})
	t.Run("filters verifies and touches active tokens", func(t *testing.T) {
		assertAPITokenActiveFilteringAndVerification(t, fixture)
	})
	t.Run("revokes by owner and excludes disabled users", func(t *testing.T) {
		assertAPITokenRevocationAndActiveUser(t, fixture)
	})
	t.Run("bulk revokes only the requested user", func(t *testing.T) {
		assertAPITokenBulkRevokeIsUserScoped(t, fixture)
	})
}

type authRepoAPITokenFixture struct {
	ctx     context.Context
	now     time.Time
	store   platform.RecordStore
	repo    *recordStoreIdentityAuthRepository
	created identityCreatedAPIToken
}

func newAuthRepoAPITokenFixture(t *testing.T) authRepoAPITokenFixture {
	t.Helper()
	ctx := context.Background()
	now := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
	store := platform.NewStore()
	repo := newRecordStoreIdentityAuthRepository(store)
	repo.apiTokenGenerator = func() string { return "active_token" }

	seedAuthRepoUser(t, store, "US1", "alice", "online")
	seedAuthRepoUser(t, store, "US2", "bob", "online")
	seedAuthRepoUser(t, store, "DISABLED", "disabled", "disabled")

	created, err := repo.CreateAPIToken(ctx, "US1", "notebook", now, time.Hour)
	if err != nil {
		t.Fatalf("create token: %v", err)
	}
	seedAuthRepoAPIToken(t, store, "ATEXPIRED", "US1", platform.FormatUserAPIToken("ATEXPIRED", "expired"), now.Add(-time.Hour), false)
	seedAuthRepoAPIToken(t, store, "ATREVOKED", "US1", platform.FormatUserAPIToken("ATREVOKED", "revoked"), now.Add(time.Hour), true)
	seedAuthRepoAPIToken(t, store, "ATWRONGUSER", "US2", platform.FormatUserAPIToken("ATWRONGUSER", "wrong"), now.Add(time.Hour), false)

	return authRepoAPITokenFixture{ctx: ctx, now: now, store: store, repo: repo, created: created}
}

func assertAPITokenCreateStoresHashOnly(t *testing.T, fixture authRepoAPITokenFixture) {
	t.Helper()
	if fixture.created.RawToken != "nexuspaas_AT2600001_active_token" {
		t.Fatalf("raw token = %q, want generated token", fixture.created.RawToken)
	}
	stored, ok := fixture.store.Get(fixture.ctx, apiTokensResource, fixture.created.ID)
	if !ok {
		t.Fatal("created token was not stored")
	}
	if stored.Data["token"] != nil || stored.Data["token_hash"] == "" {
		t.Fatalf("stored token fields = %#v, want hash only and no raw token", stored.Data)
	}
	if response := fixture.created.Response(); response["token"] == "" || response["token_hash"] != nil {
		t.Fatalf("created token response = %#v, want raw token once without hash", response)
	}
}

func assertAPITokenActiveFilteringAndVerification(t *testing.T, fixture authRepoAPITokenFixture) {
	t.Helper()
	if got := fixture.repo.CountActiveAPITokens(fixture.ctx, "US1", fixture.now); got != 1 {
		t.Fatalf("active token count = %d, want 1", got)
	}
	listed := fixture.repo.ListActiveAPITokens(fixture.ctx, "US1", fixture.now)
	if len(listed) != 1 || listed[0].ID != fixture.created.ID {
		t.Fatalf("active token list = %#v, want only created token", listed)
	}
	if metadata := listed[0].Metadata(); metadata["token_hash"] != nil || metadata["id"] != fixture.created.ID {
		t.Fatalf("token metadata = %#v, want sanitized created token", metadata)
	}

	token, user, ok := fixture.repo.FindActiveAPITokenByRaw(fixture.ctx, "nexuspaas_AT2600001_active_token", fixture.now)
	if !ok || token.ID != fixture.created.ID || user.ID != "US1" {
		t.Fatalf("verify active token = token:%#v user:%#v ok:%v, want created token/user", token, user, ok)
	}
	if !fixture.repo.TouchAPITokenLastUsed(fixture.ctx, token.ID, fixture.now.Add(time.Minute)) {
		t.Fatal("touch last_used_at failed")
	}
	touched, _ := fixture.store.Get(fixture.ctx, apiTokensResource, token.ID)
	if touched.Data["last_used_at"] == nil {
		t.Fatalf("touched token = %#v, want last_used_at", touched.Data)
	}
	assertAPITokenRawValuesFailClosed(t, fixture, []string{
		platform.FormatUserAPIToken("ATEXPIRED", "expired"),
		platform.FormatUserAPIToken("ATREVOKED", "revoked"),
		"nexuspaas_active_token",
		"missing",
	})
}

func assertAPITokenRawValuesFailClosed(t *testing.T, fixture authRepoAPITokenFixture, raws []string) {
	t.Helper()
	for _, raw := range raws {
		if token, user, ok := fixture.repo.FindActiveAPITokenByRaw(fixture.ctx, raw, fixture.now); ok {
			t.Fatalf("verify %q = token:%#v user:%#v ok:%v, want fail closed", raw, token, user, ok)
		}
	}
}

func assertAPITokenRevocationAndActiveUser(t *testing.T, fixture authRepoAPITokenFixture) {
	t.Helper()
	if _, ok := fixture.repo.RevokeAPIToken(fixture.ctx, "US2", fixture.created.ID, fixture.now); ok {
		t.Fatal("wrong owner revoked token, want fail closed")
	}
	stillActive, _ := fixture.store.Get(fixture.ctx, apiTokensResource, fixture.created.ID)
	if boolValue(stillActive.Data, "revoked") {
		t.Fatalf("wrong-owner revoke mutated token: %#v", stillActive.Data)
	}
	revoked, ok := fixture.repo.RevokeAPIToken(fixture.ctx, "US1", fixture.created.ID, fixture.now)
	if !ok || revoked.ID != fixture.created.ID || !revoked.Revoked {
		t.Fatalf("owner revoke = %#v ok=%v, want revoked token", revoked, ok)
	}
	if _, ok := fixture.repo.RevokeAPIToken(fixture.ctx, "US1", fixture.created.ID, fixture.now); ok {
		t.Fatal("second revoke succeeded, want idempotent fail-closed false")
	}
	if _, ok := fixture.repo.FindActiveUserByUsername(fixture.ctx, "disabled"); ok {
		t.Fatal("disabled user was returned as active")
	}
}

func assertAPITokenBulkRevokeIsUserScoped(t *testing.T, fixture authRepoAPITokenFixture) {
	t.Helper()
	seedAuthRepoAPIToken(t, fixture.store, "ATBULK1", "US1", platform.FormatUserAPIToken("ATBULK1", "bulk-1"), fixture.now.Add(time.Hour), false)
	seedAuthRepoAPIToken(t, fixture.store, "ATBULK2", "US1", platform.FormatUserAPIToken("ATBULK2", "bulk-2"), fixture.now.Add(time.Hour), false)
	seedAuthRepoAPIToken(t, fixture.store, "ATBULKOTHER", "US2", platform.FormatUserAPIToken("ATBULKOTHER", "bulk-other"), fixture.now.Add(time.Hour), false)
	revokedForUser := fixture.repo.RevokeAPITokensForUser(fixture.ctx, "US1", fixture.now)
	if len(revokedForUser) != 3 {
		t.Fatalf("bulk revoked tokens = %#v, want all three unrevoked US1 tokens including expired cleanup candidate", revokedForUser)
	}
	other, _ := fixture.store.Get(fixture.ctx, apiTokensResource, "ATBULKOTHER")
	if boolValue(other.Data, "revoked") {
		t.Fatalf("bulk revoke mutated other user token: %#v", other.Data)
	}
}

func TestIdentityAuthRepositoryAPITokenLookupDoesNotListTokens(t *testing.T) {
	fixture := newAuthRepoAPITokenFixture(t)
	spy := &authRepoListSpyStore{RecordStore: fixture.store}
	fixture.repo.store = spy

	token, _, ok := fixture.repo.FindActiveAPITokenByRaw(fixture.ctx, fixture.created.RawToken, fixture.now)
	if !ok || token.ID != fixture.created.ID {
		t.Fatalf("indexed token lookup failed: token=%#v ok=%v", token, ok)
	}
	if spy.listCalls != 0 {
		t.Fatalf("api token verification called List %d times, want indexed Get only", spy.listCalls)
	}
}

func TestIdentityAuthRepositorySessionRefreshLifecycle(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 15, 11, 0, 0, 0, time.UTC)
	store := platform.NewStore()
	repo := newRecordStoreIdentityAuthRepository(store)
	repo.accessTokenGenerator = func(userID string) string { return "access." + userID + ".fixed" }
	repo.refreshTokenGenerator = func() string { return "refresh.fixed" }
	seedAuthRepoUser(t, store, "US1", "alice", "offline")

	pair, err := repo.IssueSessionPair(ctx, "US1", now, time.Hour, 2*time.Hour)
	if err != nil {
		t.Fatalf("issue session pair: %v", err)
	}
	if pair.AccessToken != "access.US1.fixed" || pair.RefreshToken != "refresh.fixed" {
		t.Fatalf("session pair = %#v, want fixed generated tokens", pair)
	}
	if _, ok := repo.FindValidSession(ctx, pair.AccessToken, now); !ok {
		t.Fatal("issued session should be valid")
	}
	consumed, ok := repo.ConsumeRefreshToken(ctx, pair.RefreshToken, now)
	if !ok || consumed.UserID != "US1" {
		t.Fatalf("consume refresh = %#v ok=%v, want user refresh", consumed, ok)
	}
	if _, ok := repo.ConsumeRefreshToken(ctx, pair.RefreshToken, now); ok {
		t.Fatal("refresh replay succeeded, want fail closed")
	}
	if session, ok := repo.RevokeSession(ctx, pair.AccessToken, now); !ok || session.UserID != "US1" {
		t.Fatalf("revoke session = %#v ok=%v, want revoked session", session, ok)
	}
	if _, ok := repo.FindValidSession(ctx, pair.AccessToken, now); ok {
		t.Fatal("revoked/deleted session is still valid")
	}
	if !repo.SetUserStatus(ctx, "US1", "online") {
		t.Fatal("set user status failed")
	}
	user, _ := store.Get(ctx, usersResource, "US1")
	if user.Data["status"] != "online" {
		t.Fatalf("user status = %#v, want online", user.Data)
	}
}

func TestIdentityAuthRepositoryIssueSessionCompensatesRefreshConflict(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	store := platform.NewStore()
	repo := newRecordStoreIdentityAuthRepository(store)
	repo.accessTokenGenerator = func(userID string) string { return "access." + userID + ".conflict" }
	repo.refreshTokenGenerator = func() string { return "refresh.conflict" }
	if _, err := store.Create(ctx, refreshTokensResource, map[string]any{
		"id":         "refresh.conflict",
		"user_id":    "US1",
		"expires_at": now.Add(time.Hour).Format(time.RFC3339),
	}); err != nil {
		t.Fatal(err)
	}

	if pair, err := repo.IssueSessionPair(ctx, "US1", now, time.Hour, time.Hour); err == nil {
		t.Fatalf("issue session pair = %#v nil err, want refresh conflict", pair)
	}
	if _, ok := store.Get(ctx, sessionsResource, "access.US1.conflict"); ok {
		t.Fatal("session was left behind after refresh create conflict")
	}
}

func TestIdentityAuthRepositoryCleanupExpiredAuthRecords(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 15, 13, 0, 0, 0, time.UTC)
	store := platform.NewStore()
	repo := newRecordStoreIdentityAuthRepository(store)
	past := now.Add(-time.Hour).Format(time.RFC3339)
	future := now.Add(time.Hour).Format(time.RFC3339)

	seedAuthRepoRecord(t, store, sessionsResource, map[string]any{"id": "s-old", "expires_at": past})
	seedAuthRepoRecord(t, store, sessionsResource, map[string]any{"id": "s-new", "expires_at": future})
	seedAuthRepoRecord(t, store, refreshTokensResource, map[string]any{"id": "r-old", "expires_at": past})
	seedAuthRepoRecord(t, store, apiTokensResource, map[string]any{"id": "t-old", "expires_at": past})
	seedAuthRepoRecord(t, store, apiTokensResource, map[string]any{"id": "t-revoked", "expires_at": future, "revoked": true})
	seedAuthRepoRecord(t, store, apiTokensResource, map[string]any{"id": "t-live", "expires_at": future, "revoked": false})

	if removed := repo.CleanupExpiredAuthRecords(ctx, now); removed != 4 {
		t.Fatalf("removed = %d, want 4", removed)
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

func seedAuthRepoUser(t *testing.T, store platform.RecordStore, id, username, status string) {
	t.Helper()
	seedAuthRepoRecord(t, store, usersResource, map[string]any{
		"id":            id,
		"username":      username,
		"status":        status,
		"role":          "user",
		"system_role":   2,
		"password_hash": platform.HashSecret("correct-password"),
	})
}

func seedAuthRepoAPIToken(t *testing.T, store platform.RecordStore, id, userID, raw string, expiresAt time.Time, revoked bool) {
	t.Helper()
	seedAuthRepoRecord(t, store, apiTokensResource, map[string]any{
		"id":           id,
		"user_id":      userID,
		"name":         id,
		"token_hash":   platform.HashSecret(raw),
		"token_prefix": tokenPrefix(raw),
		"expires_at":   expiresAt.Format(time.RFC3339),
		"created_at":   expiresAt.Add(-time.Hour).Format(time.RFC3339),
		"revoked":      revoked,
	})
}

func seedAuthRepoRecord(t *testing.T, store platform.RecordStore, resource string, data map[string]any) {
	t.Helper()
	if _, err := store.Create(context.Background(), resource, data); err != nil {
		t.Fatal(err)
	}
}

type authRepoListSpyStore struct {
	platform.RecordStore
	listCalls int
}

func (s *authRepoListSpyStore) List(ctx context.Context, resource string) []contracts.Record[map[string]any] {
	s.listCalls++
	return s.RecordStore.List(ctx, resource)
}
