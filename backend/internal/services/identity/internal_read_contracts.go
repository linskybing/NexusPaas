package identity

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

const msgCredentialInvalid = "credential is invalid"

func registerInternalReadContracts(app *platform.App) {
	if app == nil {
		return
	}
	// Raw Mux internal handlers are outside the catalog guard, so each one must
	// call AuthorizeServiceRequest before reading credentials or owner data.
	app.Mux.HandleFunc("GET /internal/identity/users", internalReadUsersList(app))
	app.Mux.HandleFunc("GET /internal/identity/users/{id}", internalReadUserGet(app))
	app.Mux.HandleFunc("GET /internal/identity/roles", internalReadRolesList(app))
	app.Mux.HandleFunc("GET /internal/identity/roles/{id}", internalReadRoleGet(app))
	app.Mux.HandleFunc("POST /internal/identity/auth/session", internalAuthorizeSession(app))
	app.Mux.HandleFunc("POST /internal/identity/auth/api-token", internalAuthorizeAPIToken(app))
}

func internalReadUsersList(app *platform.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !app.AuthorizeServiceRequestForAudience(w, r, serviceName) {
			return
		}
		platform.WriteJSON(w, r, http.StatusOK, sanitizeInternalReadRecords(principalRepository(app).ListUsers(r.Context())))
	}
}

func internalReadUserGet(app *platform.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !app.AuthorizeServiceRequestForAudience(w, r, serviceName) {
			return
		}
		id := r.PathValue("id")
		repo := principalRepository(app)
		record, ok := repo.GetUser(r.Context(), id)
		if !ok {
			platform.WriteJSON(w, r, http.StatusNotFound, map[string]any{"resource": repo.UserResourceName(), "id": id})
			return
		}
		platform.WriteJSON(w, r, http.StatusOK, sanitizeInternalReadRecord(record))
	}
}

func internalReadRolesList(app *platform.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !app.AuthorizeServiceRequestForAudience(w, r, serviceName) {
			return
		}
		platform.WriteJSON(w, r, http.StatusOK, principalRepository(app).ListRoles(r.Context()))
	}
}

func internalReadRoleGet(app *platform.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !app.AuthorizeServiceRequestForAudience(w, r, serviceName) {
			return
		}
		id := r.PathValue("id")
		repo := principalRepository(app)
		record, ok := repo.GetRole(r.Context(), id)
		if !ok {
			platform.WriteJSON(w, r, http.StatusNotFound, map[string]any{"resource": repo.RoleResourceName(), "id": id})
			return
		}
		platform.WriteJSON(w, r, http.StatusOK, record)
	}
}

func sanitizeInternalReadRecords(records []contracts.Record[map[string]any]) []contracts.Record[map[string]any] {
	out := make([]contracts.Record[map[string]any], 0, len(records))
	for _, record := range records {
		out = append(out, sanitizeInternalReadRecord(record))
	}
	return out
}

func sanitizeInternalReadRecord(record contracts.Record[map[string]any]) contracts.Record[map[string]any] {
	record.Data = internalAuthUser(record.Data)
	return record
}

func internalAuthorizeSession(app *platform.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !app.AuthorizeServiceRequestForAudience(w, r, serviceName) {
			return
		}
		token := internalAuthToken(r)
		if token == "" {
			platform.WriteError(w, r, http.StatusBadRequest, "invalid_request", "token is required")
			return
		}
		repo := authRepository(app)
		session, ok := repo.FindValidSession(r.Context(), token, time.Now().UTC())
		if !ok || internalCredentialRevoked(app, r, "session", token) {
			platform.WriteError(w, r, http.StatusUnauthorized, "unauthorized", msgCredentialInvalid)
			return
		}
		user, ok := repo.FindActiveUserByID(r.Context(), session.UserID)
		if !ok {
			platform.WriteError(w, r, http.StatusUnauthorized, "unauthorized", msgCredentialInvalid)
			return
		}
		platform.WriteJSON(w, r, http.StatusOK, map[string]any{"user": internalAuthUser(user.Data)})
	}
}

func internalAuthorizeAPIToken(app *platform.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !app.AuthorizeServiceRequestForAudience(w, r, serviceName) {
			return
		}
		token := internalAuthToken(r)
		if token == "" {
			platform.WriteError(w, r, http.StatusBadRequest, "invalid_request", "token is required")
			return
		}
		if user, tokenID, ok := verifyInternalAPIToken(app, r, token); ok {
			platform.WriteJSON(w, r, http.StatusOK, map[string]any{"user": internalAuthUser(user), "api_token_id": tokenID})
			return
		}
		platform.WriteError(w, r, http.StatusUnauthorized, "unauthorized", msgCredentialInvalid)
	}
}

func verifyInternalAPIToken(app *platform.App, r *http.Request, token string) (map[string]any, string, bool) {
	repo := authRepository(app)
	apiToken, user, ok := repo.FindActiveAPITokenByRaw(r.Context(), token, time.Now().UTC())
	if !ok {
		return nil, "", false
	}
	if internalCredentialRevoked(app, r, "api_token", apiToken.ID) {
		return nil, "", false
	}
	if !repo.TouchAPITokenLastUsed(r.Context(), apiToken.ID, time.Now().UTC()) {
		slog.Warn("internal api token last_used_at update skipped", "token_id", apiToken.ID)
	}
	return user.Data, apiToken.ID, true
}

func internalAuthToken(r *http.Request) string {
	payload, err := decodePayload(r)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(textValue(payload, "token"))
}

func internalCredentialRevoked(app *platform.App, r *http.Request, kind, id string) bool {
	if app.Revocations == nil || id == "" {
		return false
	}
	revoked, err := app.Revocations.IsRevoked(r.Context(), kind, id)
	if err != nil {
		slog.Warn("internal credential revocation check failed", "kind", kind, "error", err)
		return false
	}
	return revoked
}

func internalAuthUser(user map[string]any) map[string]any {
	out := publicUser(user)
	for _, key := range []string{"status", "capabilities", "admin_panel", "adminPanel", "AdminPanel", "role_id", "roleId"} {
		if value, ok := user[key]; ok {
			out[key] = value
		}
	}
	return out
}
