package identity

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

var (
	errLDAPInvalidCredentials = errors.New("ldap invalid credentials")
	errLDAPUnavailable        = errors.New("ldap unavailable")
	errLDAPNotConfigured      = errors.New("ldap not configured")

	msgLDAPSyncFailed = "user could not be synchronized"

	newLDAPDirectory = func(cfg platform.Config) ldapDirectory {
		return newGoLDAPDirectory(cfg)
	}
)

type ldapDirectory interface {
	Authenticate(ctx context.Context, username, password string) (string, error)
	UpsertUser(ctx context.Context, user map[string]any, password string, options ...ldapUpsertOption) (ldapUpsertResult, error)
	RestoreUpsert(ctx context.Context, result ldapUpsertResult) error
	DeleteUser(ctx context.Context, user map[string]any) error
	RestoreDeletedUser(ctx context.Context, user map[string]any) error
	ListUsernames(ctx context.Context) (map[string]bool, error)
}

type ldapUpsertResult struct {
	Username      string
	DN            string
	Created       bool
	Previous      map[string][]string
	ModifiedAttrs []string
}

type ldapUpsertOptions struct {
	preserveExistingPassword bool
}

type ldapUpsertOption func(*ldapUpsertOptions)

func preserveExistingLDAPPassword() ldapUpsertOption {
	return func(options *ldapUpsertOptions) {
		options.preserveExistingPassword = true
	}
}

func ldapDirectoryFor(app *platform.App) (ldapDirectory, bool) {
	if app == nil || !app.Config.LDAPEnabled {
		return nil, false
	}
	dir := newLDAPDirectory(app.Config)
	return dir, dir != nil
}

func authenticateUser(app *platform.App, r *http.Request, username, password string) (map[string]any, bool) {
	repo := authRepository(app)
	if dir, ok := ldapDirectoryFor(app); ok {
		ldapUsername, err := dir.Authenticate(r.Context(), username, password)
		if err == nil {
			if ldapUsername == "" {
				ldapUsername = username
			}
			userRecord, found := repo.FindActiveUserByUsername(r.Context(), ldapUsername)
			if found {
				slog.Info("ldap auth success", "username", safeLDAPUsername(ldapUsername))
				return userRecord.Data, true
			}
			slog.Warn("ldap auth rejected by local identity state", "username", safeLDAPUsername(ldapUsername))
			return nil, false
		}
		slog.Warn("ldap auth failed, falling back to local credentials", "username", safeLDAPUsername(username), "error", sanitizeLDAPError(err))
	}
	userRecord, ok := repo.FindActiveUserByUsername(r.Context(), username)
	if !ok || !passwordMatches(userRecord.Data, password) {
		return nil, false
	}
	return userRecord.Data, true
}

func createUserWithLDAP(app *platform.App, r *http.Request, user map[string]any, password string) (contracts.Record[map[string]any], int, map[string]any) {
	dir, ldapOn := ldapDirectoryFor(app)
	var ldapResult ldapUpsertResult
	if ldapOn {
		var err error
		ldapResult, err = dir.UpsertUser(r.Context(), user, password, preserveExistingLDAPPassword())
		if err != nil {
			slog.Warn("ldap user create sync failed", "username", safeLDAPUsername(textValue(user, "username")), "error", sanitizeLDAPError(err))
			return contracts.Record[map[string]any]{}, http.StatusServiceUnavailable, map[string]any{"message": msgLDAPSyncFailed}
		}
	}
	record, err := principalRepository(app).CreateUser(r.Context(), user)
	if err != nil {
		if ldapOn {
			compensateLDAPUpsert(r.Context(), dir, ldapResult)
		}
		if platform.IsCreateConflict(err) {
			return contracts.Record[map[string]any]{}, http.StatusConflict, map[string]any{"message": "user already exists"}
		}
		return contracts.Record[map[string]any]{}, http.StatusInternalServerError, map[string]any{"message": "user could not be created"}
	}
	return record, http.StatusOK, nil
}

func updateUserWithLDAP(app *platform.App, r *http.Request, id string, payload, update map[string]any) (contracts.Record[map[string]any], int, map[string]any) {
	repo := principalRepository(app)
	current, found := repo.GetUser(r.Context(), id)
	if !found {
		return contracts.Record[map[string]any]{}, http.StatusNotFound, map[string]any{"message": msgUserNotFound}
	}
	dir, ldapOn := ldapDirectoryFor(app)
	if !ldapOn {
		updated, ok := repo.UpdateUser(r.Context(), id, update)
		if !ok {
			return contracts.Record[map[string]any]{}, http.StatusNotFound, map[string]any{"message": msgUserNotFound}
		}
		return updated, http.StatusOK, nil
	}

	password := textValue(payload, "password")
	target := mergedUserData(current.Data, update)
	if password == "" {
		ldapResult, err := dir.UpsertUser(r.Context(), target, "")
		if err != nil {
			slog.Warn("ldap user update sync failed", "username", safeLDAPUsername(textValue(target, "username")), "error", sanitizeLDAPError(err))
			return contracts.Record[map[string]any]{}, http.StatusServiceUnavailable, map[string]any{"message": msgLDAPSyncFailed}
		}
		updated, ok := repo.UpdateUser(r.Context(), id, update)
		if !ok {
			compensateLDAPUpsert(r.Context(), dir, ldapResult)
			return contracts.Record[map[string]any]{}, http.StatusServiceUnavailable, map[string]any{"message": "user could not be updated"}
		}
		return updated, http.StatusOK, nil
	}

	updated, ok := repo.UpdateUser(r.Context(), id, update)
	if !ok {
		return contracts.Record[map[string]any]{}, http.StatusServiceUnavailable, map[string]any{"message": "user could not be updated"}
	}
	if _, err := dir.UpsertUser(r.Context(), target, password); err != nil {
		rollbackLocalUser(r.Context(), app, id, current.Data)
		slog.Warn("ldap user password sync failed", "username", safeLDAPUsername(textValue(target, "username")), "error", sanitizeLDAPError(err))
		return contracts.Record[map[string]any]{}, http.StatusServiceUnavailable, map[string]any{"message": msgLDAPSyncFailed}
	}
	return updated, http.StatusOK, nil
}

func resetUserPasswordWithLDAP(app *platform.App, r *http.Request, id, password string) bool {
	repo := principalRepository(app)
	current, found := repo.GetUser(r.Context(), id)
	if !found {
		return false
	}
	update := map[string]any{"password_hash": platform.HashSecret(password), "updated_at": time.Now().UTC().Format(time.RFC3339)}
	updated, ok := repo.UpdateUser(r.Context(), id, update)
	if !ok {
		return false
	}
	if dir, ldapOn := ldapDirectoryFor(app); ldapOn {
		if _, err := dir.UpsertUser(r.Context(), updated.Data, password); err != nil {
			rollbackLocalUser(r.Context(), app, id, current.Data)
			slog.Warn("ldap batch password sync failed", "username", safeLDAPUsername(textValue(updated.Data, "username")), "error", sanitizeLDAPError(err))
			return false
		}
	}
	return true
}

func updateUserRoleWithLDAP(app *platform.App, r *http.Request, id string, update map[string]any) (map[string]any, bool) {
	repo := principalRepository(app)
	current, found := repo.GetUser(r.Context(), id)
	if !found {
		return nil, false
	}
	dir, ldapOn := ldapDirectoryFor(app)
	target := mergedUserData(current.Data, update)
	var ldapResult ldapUpsertResult
	if ldapOn {
		var err error
		ldapResult, err = dir.UpsertUser(r.Context(), target, "")
		if err != nil {
			slog.Warn("ldap batch role sync failed", "username", safeLDAPUsername(textValue(target, "username")), "error", sanitizeLDAPError(err))
			return nil, false
		}
	}
	updated, ok := repo.UpdateUser(r.Context(), id, update)
	if !ok {
		if ldapOn {
			compensateLDAPUpsert(r.Context(), dir, ldapResult)
		}
		return nil, false
	}
	return updated.Data, true
}

func deleteUserWithLDAP(app *platform.App, r *http.Request, id string) (int, map[string]any) {
	repo := principalRepository(app)
	current, found := repo.GetUser(r.Context(), id)
	if !found {
		return http.StatusNotFound, map[string]any{"message": msgUserNotFound}
	}
	dir, ldapOn := ldapDirectoryFor(app)
	if ldapOn {
		if err := dir.DeleteUser(r.Context(), current.Data); err != nil {
			slog.Warn("ldap user delete sync failed", "username", safeLDAPUsername(textValue(current.Data, "username")), "error", sanitizeLDAPError(err))
			return http.StatusServiceUnavailable, map[string]any{"message": msgLDAPSyncFailed}
		}
	}
	if !repo.DeleteUser(r.Context(), id) {
		if ldapOn {
			if err := dir.RestoreDeletedUser(r.Context(), current.Data); err != nil {
				slog.Warn("ldap user delete compensation failed", "username", safeLDAPUsername(textValue(current.Data, "username")), "error", sanitizeLDAPError(err))
			}
		}
		return http.StatusServiceUnavailable, map[string]any{"message": "user could not be deleted"}
	}
	return http.StatusOK, nil
}

func syncLDAPMirror(ctx context.Context, app *platform.App, dir ldapDirectory) error {
	if app == nil || !app.Config.LDAPEnabled {
		slog.Info("ldap mirror sync skipped", "enabled", false)
		return nil
	}
	if dir == nil {
		var ok bool
		dir, ok = ldapDirectoryFor(app)
		if !ok {
			return errLDAPNotConfigured
		}
	}
	ldapUsers, err := dir.ListUsernames(ctx)
	if err != nil {
		return err
	}
	scanned := 0
	upserted := 0
	skipped := 0
	for _, record := range principalRepository(app).ListUsers(ctx) {
		scanned++
		username := strings.TrimSpace(textValue(record.Data, "username"))
		if username == "" || strings.EqualFold(textValue(record.Data, "status"), "deleted") {
			skipped++
			continue
		}
		if ldapUsers[strings.ToLower(username)] {
			continue
		}
		if _, err := dir.UpsertUser(ctx, record.Data, ""); err != nil {
			slog.Warn("ldap mirror upsert failed", "username", safeLDAPUsername(username), "error", sanitizeLDAPError(err))
			return err
		}
		upserted++
	}
	slog.Info("ldap mirror sync complete", "scanned", scanned, "ldap_users", len(ldapUsers), "upserted", upserted, "skipped", skipped)
	return nil
}

func registerLDAPMirror(app *platform.App) {
	app.RegisterMaintenanceTaskForService(serviceName, "ldap-mirror-sync", func(ctx context.Context) error {
		return syncLDAPMirror(ctx, app, nil)
	})
}

func mergedUserData(current, update map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range current {
		out[key] = value
	}
	for key, value := range update {
		out[key] = value
	}
	return out
}

func rollbackLocalUser(ctx context.Context, app *platform.App, id string, previous map[string]any) {
	restore := map[string]any{}
	for _, key := range []string{"username", "email", "name", "full_name", "password_hash", "status", "role", "role_id", "system_role", "updated_at"} {
		if value, ok := previous[key]; ok {
			restore[key] = value
		}
	}
	if _, ok := principalRepository(app).UpdateUser(ctx, id, restore); !ok {
		slog.Error("local user rollback failed", "id", id)
	}
}

func compensateLDAPUpsert(ctx context.Context, dir ldapDirectory, result ldapUpsertResult) {
	if dir == nil {
		return
	}
	if err := dir.RestoreUpsert(ctx, result); err != nil {
		slog.Warn("ldap user compensation failed", "username", safeLDAPUsername(result.Username), "error", sanitizeLDAPError(err))
	}
}

func safeLDAPUsername(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}

func sanitizeLDAPError(err error) string {
	if err == nil {
		return ""
	}
	switch {
	case errors.Is(err, errLDAPInvalidCredentials):
		return errLDAPInvalidCredentials.Error()
	case errors.Is(err, errLDAPNotConfigured):
		return errLDAPNotConfigured.Error()
	default:
		return errLDAPUnavailable.Error()
	}
}

func ldapUserFromPayload(user map[string]any) map[string]any {
	role := shared.FirstNonEmpty(textValue(user, "role"), roleName(intValue(user, "system_role", 2)))
	systemRole := systemRoleFor(role, intValue(user, "system_role", 2))
	return map[string]any{
		"id":          textValue(user, "id"),
		"username":    strings.TrimSpace(textValue(user, "username")),
		"email":       strings.TrimSpace(textValue(user, "email")),
		"full_name":   strings.TrimSpace(shared.FirstNonEmpty(textValue(user, "full_name"), textValue(user, "name"), textValue(user, "username"))),
		"name":        strings.TrimSpace(shared.FirstNonEmpty(textValue(user, "name"), textValue(user, "full_name"), textValue(user, "username"))),
		"role":        role,
		"role_id":     shared.FirstNonEmpty(textValue(user, "role_id"), defaultRoleID),
		"system_role": systemRole,
		"status":      textValue(user, "status"),
	}
}
