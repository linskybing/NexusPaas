package identity

import (
	"context"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

type identityAuthRepository interface {
	FindActiveUserByID(ctx context.Context, id string) (identityUser, bool)
	FindActiveUserByUsername(ctx context.Context, username string) (identityUser, bool)
	SetUserStatus(ctx context.Context, id, status string) bool
	IssueSessionPair(ctx context.Context, userID string, now time.Time, accessTTL, refreshTTL time.Duration) (identitySessionPair, error)
	FindValidSession(ctx context.Context, token string, now time.Time) (identitySession, bool)
	ConsumeRefreshToken(ctx context.Context, token string, now time.Time) (identityRefreshToken, bool)
	RevokeSession(ctx context.Context, token string, now time.Time) (identitySession, bool)
	DeleteRefreshToken(ctx context.Context, token string) bool
	ListActiveAPITokens(ctx context.Context, userID string, now time.Time) []identityAPIToken
	CountActiveAPITokens(ctx context.Context, userID string, now time.Time) int
	CreateAPIToken(ctx context.Context, userID, name string, now time.Time, ttl time.Duration) (identityCreatedAPIToken, error)
	FindActiveAPITokenByRaw(ctx context.Context, rawToken string, now time.Time) (identityAPIToken, identityUser, bool)
	TouchAPITokenLastUsed(ctx context.Context, tokenID string, at time.Time) bool
	RevokeAPIToken(ctx context.Context, userID, tokenID string, now time.Time) (identityAPIToken, bool)
	RevokeAPITokensForUser(ctx context.Context, userID string, now time.Time) []identityAPIToken
	CleanupExpiredAuthRecords(ctx context.Context, now time.Time) int
}

type identityUser struct {
	ID       string
	Username string
	Status   string
	Data     map[string]any
	record   contracts.Record[map[string]any]
}

func (u identityUser) Record() contracts.Record[map[string]any] {
	record := u.record
	record.Data = shared.CloneMap(u.Data)
	return record
}

type identitySession struct {
	Token     string
	UserID    string
	ExpiresAt string
	Data      map[string]any
}

type identityRefreshToken struct {
	Token     string
	UserID    string
	ExpiresAt string
	Data      map[string]any
}

type identitySessionPair struct {
	AccessToken  string
	RefreshToken string
	UserID       string
}

type identityAPIToken struct {
	ID          string
	UserID      string
	Name        string
	TokenHash   string
	TokenPrefix string
	ExpiresAt   string
	CreatedAt   string
	LastUsedAt  string
	RevokedAt   string
	Revoked     bool
	Data        map[string]any
}

func (t identityAPIToken) Metadata() map[string]any {
	out := map[string]any{
		"id":           t.ID,
		"name":         t.Name,
		"token_prefix": t.TokenPrefix,
		"expires_at":   t.ExpiresAt,
		"created_at":   t.CreatedAt,
	}
	if t.LastUsedAt != "" {
		out["last_used_at"] = t.LastUsedAt
	}
	return out
}

type identityCreatedAPIToken struct {
	identityAPIToken
	RawToken string
}

func (t identityCreatedAPIToken) Response() map[string]any {
	out := t.Metadata()
	out["token"] = t.RawToken
	return out
}

type recordStoreIdentityAuthRepository struct {
	store                 platform.RecordStore
	principals            identityPrincipalRepository
	accessTokenGenerator  func(userID string) string
	refreshTokenGenerator func() string
	apiTokenGenerator     func() string
	now                   func() time.Time
}

func authRepository(app *platform.App) identityAuthRepository {
	if app == nil {
		return newRecordStoreIdentityAuthRepository(nil)
	}
	return newRecordStoreIdentityAuthRepository(app.Store, principalRepository(app))
}

func newRecordStoreIdentityAuthRepository(store platform.RecordStore, principals ...identityPrincipalRepository) *recordStoreIdentityAuthRepository {
	principalRepo := principalRepositoryFromStore(store)
	if len(principals) > 0 && principals[0] != nil {
		principalRepo = principals[0]
	}
	return &recordStoreIdentityAuthRepository{
		store:      store,
		principals: principalRepo,
		accessTokenGenerator: func(userID string) string {
			return "access." + userID + "." + randomHex(24)
		},
		refreshTokenGenerator: func() string {
			return "refresh." + randomHex(24)
		},
		apiTokenGenerator: func() string {
			return "nexuspaas_" + randomHex(24)
		},
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
}

func (r *recordStoreIdentityAuthRepository) FindActiveUserByID(ctx context.Context, id string) (identityUser, bool) {
	record, ok := r.principals.GetUser(ctx, id)
	if !ok {
		return identityUser{}, false
	}
	return userFromRecord(record)
}

func (r *recordStoreIdentityAuthRepository) FindActiveUserByUsername(ctx context.Context, username string) (identityUser, bool) {
	record, ok := r.principals.FindUserByUsername(ctx, username)
	if !ok {
		return identityUser{}, false
	}
	return userFromRecord(record)
}

func (r *recordStoreIdentityAuthRepository) SetUserStatus(ctx context.Context, id, status string) bool {
	return r.principals.SetUserStatus(ctx, id, status)
}

func (r *recordStoreIdentityAuthRepository) IssueSessionPair(ctx context.Context, userID string, now time.Time, accessTTL, refreshTTL time.Duration) (identitySessionPair, error) {
	if now.IsZero() {
		now = r.now()
	}
	for attempt := 0; attempt < 3; attempt++ {
		access := r.accessTokenGenerator(userID)
		refresh := r.refreshTokenGenerator()
		if err := r.createSession(ctx, access, userID, now, accessTTL); err != nil {
			if platform.IsCreateConflict(err) {
				continue
			}
			return identitySessionPair{}, err
		}
		if err := r.createRefreshToken(ctx, refresh, userID, now, refreshTTL); err != nil {
			r.store.Delete(ctx, sessionsResource, access)
			if platform.IsCreateConflict(err) {
				continue
			}
			return identitySessionPair{}, err
		}
		return identitySessionPair{AccessToken: access, RefreshToken: refresh, UserID: userID}, nil
	}
	return identitySessionPair{}, platform.CreateConflictError{Resource: sessionsResource, ID: "session"}
}

func (r *recordStoreIdentityAuthRepository) ConsumeRefreshToken(ctx context.Context, token string, now time.Time) (identityRefreshToken, bool) {
	record, ok := r.store.Get(ctx, refreshTokensResource, token)
	if !ok || tokenExpiredAt(record.Data, now) {
		return identityRefreshToken{}, false
	}
	if !r.store.Delete(ctx, refreshTokensResource, token) {
		return identityRefreshToken{}, false
	}
	return refreshTokenFromRecord(record), true
}

func (r *recordStoreIdentityAuthRepository) FindValidSession(ctx context.Context, token string, now time.Time) (identitySession, bool) {
	record, ok := r.store.Get(ctx, sessionsResource, token)
	if !ok || tokenExpiredAt(record.Data, now) || boolValue(record.Data, "revoked") {
		return identitySession{}, false
	}
	return sessionFromRecord(record), true
}

func (r *recordStoreIdentityAuthRepository) RevokeSession(ctx context.Context, token string, _ time.Time) (identitySession, bool) {
	record, ok := r.store.Get(ctx, sessionsResource, token)
	if !ok {
		return identitySession{}, false
	}
	if !r.store.Delete(ctx, sessionsResource, token) {
		return identitySession{}, false
	}
	return sessionFromRecord(record), true
}

func (r *recordStoreIdentityAuthRepository) DeleteRefreshToken(ctx context.Context, token string) bool {
	return r.store.Delete(ctx, refreshTokensResource, token)
}

func (r *recordStoreIdentityAuthRepository) ListActiveAPITokens(ctx context.Context, userID string, now time.Time) []identityAPIToken {
	var tokens []identityAPIToken
	for _, record := range r.store.List(ctx, apiTokensResource) {
		token := apiTokenFromRecord(record)
		if token.UserID == userID && tokenActiveAt(token, now) {
			tokens = append(tokens, token)
		}
	}
	return tokens
}

func (r *recordStoreIdentityAuthRepository) CountActiveAPITokens(ctx context.Context, userID string, now time.Time) int {
	return len(r.ListActiveAPITokens(ctx, userID, now))
}

func (r *recordStoreIdentityAuthRepository) CreateAPIToken(ctx context.Context, userID, name string, now time.Time, ttl time.Duration) (identityCreatedAPIToken, error) {
	if now.IsZero() {
		now = r.now()
	}
	if ttl <= 0 {
		ttl = defaultAPITokenTTL
	}
	rawToken := r.apiTokenGenerator()
	for attempt := 0; attempt < 3; attempt++ {
		id := r.store.NextID(apiTokensResource, "AT", 2600001, 0)
		if attempt > 0 {
			id = "AT" + strings.ToUpper(randomHex(8))
		}
		record, err := r.store.Create(ctx, apiTokensResource, map[string]any{
			"id":           id,
			"user_id":      userID,
			"name":         name,
			"token_hash":   platform.HashSecret(rawToken),
			"token_prefix": tokenPrefix(rawToken),
			"expires_at":   now.Add(ttl).Format(time.RFC3339),
			"created_at":   now.Format(time.RFC3339),
			"revoked":      false,
		})
		if err == nil {
			return identityCreatedAPIToken{identityAPIToken: apiTokenFromRecord(record), RawToken: rawToken}, nil
		}
		if !platform.IsCreateConflict(err) {
			return identityCreatedAPIToken{}, err
		}
	}
	return identityCreatedAPIToken{}, platform.CreateConflictError{Resource: apiTokensResource, ID: "api_token"}
}

func (r *recordStoreIdentityAuthRepository) FindActiveAPITokenByRaw(ctx context.Context, rawToken string, now time.Time) (identityAPIToken, identityUser, bool) {
	for _, record := range r.store.List(ctx, apiTokensResource) {
		token := apiTokenFromRecord(record)
		if !tokenActiveAt(token, now) || !platform.VerifySecret(token.TokenHash, rawToken) {
			continue
		}
		user, ok := r.FindActiveUserByID(ctx, token.UserID)
		if !ok {
			return identityAPIToken{}, identityUser{}, false
		}
		return token, user, true
	}
	return identityAPIToken{}, identityUser{}, false
}

func (r *recordStoreIdentityAuthRepository) TouchAPITokenLastUsed(ctx context.Context, tokenID string, at time.Time) bool {
	if at.IsZero() {
		at = r.now()
	}
	_, ok := r.store.Update(ctx, apiTokensResource, tokenID, map[string]any{"last_used_at": at.Format(time.RFC3339)})
	return ok
}

func (r *recordStoreIdentityAuthRepository) RevokeAPIToken(ctx context.Context, userID, tokenID string, now time.Time) (identityAPIToken, bool) {
	if tokenID == "" {
		return identityAPIToken{}, false
	}
	if now.IsZero() {
		now = r.now()
	}
	record, ok := r.store.Get(ctx, apiTokensResource, tokenID)
	if !ok {
		return identityAPIToken{}, false
	}
	token := apiTokenFromRecord(record)
	if token.UserID != userID || token.Revoked {
		return identityAPIToken{}, false
	}
	updated, ok := r.store.Update(ctx, apiTokensResource, tokenID, map[string]any{
		"revoked":    true,
		"revoked_at": now.Format(time.RFC3339),
	})
	if !ok {
		return identityAPIToken{}, false
	}
	return apiTokenFromRecord(updated), true
}

func (r *recordStoreIdentityAuthRepository) RevokeAPITokensForUser(ctx context.Context, userID string, now time.Time) []identityAPIToken {
	var revoked []identityAPIToken
	for _, record := range r.store.List(ctx, apiTokensResource) {
		token := apiTokenFromRecord(record)
		if token.UserID != userID || token.Revoked {
			continue
		}
		if updated, ok := r.RevokeAPIToken(ctx, userID, token.ID, now); ok {
			revoked = append(revoked, updated)
		}
	}
	return revoked
}

func (r *recordStoreIdentityAuthRepository) CleanupExpiredAuthRecords(ctx context.Context, now time.Time) int {
	removed := r.cleanupResource(ctx, sessionsResource, func(data map[string]any) bool { return tokenExpiredAt(data, now) })
	removed += r.cleanupResource(ctx, refreshTokensResource, func(data map[string]any) bool { return tokenExpiredAt(data, now) })
	removed += r.cleanupResource(ctx, apiTokensResource, func(data map[string]any) bool {
		return tokenExpiredAt(data, now) || boolValue(data, "revoked") || textValue(data, "revoked_at") != ""
	})
	return removed
}

func (r *recordStoreIdentityAuthRepository) createSession(ctx context.Context, token, userID string, now time.Time, ttl time.Duration) error {
	_, err := r.store.Create(ctx, sessionsResource, map[string]any{
		"id":         token,
		"token":      token,
		"user_id":    userID,
		"created_at": now.Format(time.RFC3339),
		"expires_at": now.Add(ttl).Format(time.RFC3339),
		"revoked":    false,
	})
	return err
}

func (r *recordStoreIdentityAuthRepository) createRefreshToken(ctx context.Context, token, userID string, now time.Time, ttl time.Duration) error {
	_, err := r.store.Create(ctx, refreshTokensResource, map[string]any{
		"id":         token,
		"token":      token,
		"user_id":    userID,
		"created_at": now.Format(time.RFC3339),
		"expires_at": now.Add(ttl).Format(time.RFC3339),
	})
	return err
}

func (r *recordStoreIdentityAuthRepository) cleanupResource(ctx context.Context, resource string, shouldRemove func(map[string]any) bool) int {
	removed := 0
	for _, record := range r.store.List(ctx, resource) {
		if shouldRemove(record.Data) && r.store.Delete(ctx, resource, record.ID) {
			removed++
		}
	}
	return removed
}

func userFromRecord(record contracts.Record[map[string]any]) (identityUser, bool) {
	data := shared.CloneMap(record.Data)
	if !activeUser(data) {
		return identityUser{}, false
	}
	return identityUser{
		ID:       shared.FirstNonEmpty(record.ID, textValue(data, "id")),
		Username: textValue(data, "username"),
		Status:   textValue(data, "status"),
		Data:     data,
		record:   record,
	}, true
}

func sessionFromRecord(record contracts.Record[map[string]any]) identitySession {
	data := shared.CloneMap(record.Data)
	return identitySession{
		Token:     shared.FirstNonEmpty(record.ID, textValue(data, "id"), textValue(data, "token")),
		UserID:    textValue(data, "user_id"),
		ExpiresAt: textValue(data, "expires_at"),
		Data:      data,
	}
}

func refreshTokenFromRecord(record contracts.Record[map[string]any]) identityRefreshToken {
	data := shared.CloneMap(record.Data)
	return identityRefreshToken{
		Token:     shared.FirstNonEmpty(record.ID, textValue(data, "id"), textValue(data, "token")),
		UserID:    textValue(data, "user_id"),
		ExpiresAt: textValue(data, "expires_at"),
		Data:      data,
	}
}

func apiTokenFromRecord(record contracts.Record[map[string]any]) identityAPIToken {
	data := shared.CloneMap(record.Data)
	return identityAPIToken{
		ID:          shared.FirstNonEmpty(record.ID, textValue(data, "id")),
		UserID:      textValue(data, "user_id"),
		Name:        textValue(data, "name"),
		TokenHash:   textValue(data, "token_hash"),
		TokenPrefix: textValue(data, "token_prefix"),
		ExpiresAt:   textValue(data, "expires_at"),
		CreatedAt:   textValue(data, "created_at"),
		LastUsedAt:  textValue(data, "last_used_at"),
		RevokedAt:   textValue(data, "revoked_at"),
		Revoked:     boolValue(data, "revoked") || textValue(data, "revoked_at") != "",
		Data:        data,
	}
}

func tokenActiveAt(token identityAPIToken, now time.Time) bool {
	return !token.Revoked && !tokenExpiredAt(token.Data, now)
}

func tokenExpiredAt(data map[string]any, now time.Time) bool {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return expiredAt(data, now)
}
