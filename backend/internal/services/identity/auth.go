package identity

import (
	"encoding/hex"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

func register(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	payload, err := decodePayload(r)
	if err != nil {
		return http.StatusBadRequest, map[string]any{"message": msgInvalidInputTitle}, nil
	}
	username := strings.TrimSpace(textValue(payload, "username"))
	password := textValue(payload, "password")
	email := strings.TrimSpace(textValue(payload, "email"))
	fullName := strings.TrimSpace(shared.FirstNonEmpty(textValue(payload, "full_name"), textValue(payload, "name")))
	userType := "origin"
	status := "offline"
	systemRole := 2
	roleID := defaultRoleID

	switch {
	case len(username) < 3 || len(username) > 50:
		return http.StatusBadRequest, map[string]any{"message": "username must be between 3 and 50 characters"}, nil
	case len(password) < 6:
		return http.StatusBadRequest, map[string]any{"message": "password must be at least 6 characters"}, nil
	case email != "" && !strings.Contains(email, "@"):
		return http.StatusBadRequest, map[string]any{"message": "email must be valid"}, nil
	}
	repo := principalRepository(app)
	if _, ok := repo.FindUserByUsername(r.Context(), username); ok {
		return http.StatusConflict, map[string]any{"message": "username already exists"}, nil
	}

	id := repo.NextUserID()
	user := map[string]any{
		"id":            id,
		"username":      username,
		"email":         email,
		"full_name":     fullName,
		"name":          shared.FirstNonEmpty(fullName, username),
		"password_hash": platform.HashSecret(password),
		"type":          userType,
		"status":        status,
		"role":          roleName(systemRole),
		"role_id":       roleID,
		"system_role":   systemRole,
	}
	record, statusCode, errData := createUserWithLDAP(app, r, user, password)
	if statusCode != http.StatusOK {
		return statusCode, errData, nil
	}
	publish(app, r, "UserCreated", publicUser(record.Data))
	return http.StatusOK, nil, nil
}

func login(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	payload, err := decodePayload(r)
	if err != nil {
		return http.StatusBadRequest, map[string]any{"message": msgInvalidInputTitle}, nil
	}
	username := strings.TrimSpace(textValue(payload, "username"))
	password := textValue(payload, "password")
	if username == "" || password == "" {
		return http.StatusBadRequest, map[string]any{"message": "username and password are required"}, nil
	}
	if loginLocked(app, r, username) {
		publishLoginFailure(app, r, username, pathLogin, reasonLocked)
		return http.StatusUnauthorized, map[string]any{"message": msgInvalidCredentials}, nil
	}
	if status, data, ok := validateLoginCaptcha(app, r, username, payload); !ok {
		return status, data, nil
	}
	user, ok := authenticateUser(app, r, username, password)
	if !ok {
		recordLoginFailure(app, r, username)
		publishLoginFailure(app, r, username, pathLogin, reasonInvalidCredentials)
		return http.StatusUnauthorized, map[string]any{"message": msgInvalidCredentials}, nil
	}
	clearLoginFailures(app, r, username)
	data, err := issueSession(app, r, user)
	if err != nil {
		return http.StatusServiceUnavailable, map[string]any{"message": "session could not be issued"}, nil
	}
	cookies := authCookies(r, textValue(data, "token"), textValue(data, "refresh_token"))
	return rawJSON(r, http.StatusOK, data, cookies)
}

func validateLoginCaptcha(app *platform.App, r *http.Request, username string, payload map[string]any) (int, any, bool) {
	captchaID := strings.TrimSpace(textValue(payload, "captcha_id"))
	captchaAnswer := textValue(payload, "captcha_answer")
	if captchaRequired(app, r, username) {
		if captchaID == "" || strings.TrimSpace(captchaAnswer) == "" {
			recordLoginFailure(app, r, username)
			publishLoginFailure(app, r, username, pathLogin, reasonCaptchaRequired)
			return http.StatusUnauthorized, map[string]any{"message": msgCaptchaRequired}, false
		}
		if !verifyCaptcha(app, r, captchaID, captchaAnswer) {
			return rejectInvalidCaptcha(app, r, username)
		}
		return 0, nil, true
	}
	if captchaID != "" && !verifyCaptcha(app, r, captchaID, captchaAnswer) {
		return rejectInvalidCaptcha(app, r, username)
	}
	return 0, nil, true
}

func rejectInvalidCaptcha(app *platform.App, r *http.Request, username string) (int, any, bool) {
	recordLoginFailure(app, r, username)
	publishLoginFailure(app, r, username, pathLogin, reasonInvalidCaptcha)
	return http.StatusUnauthorized, map[string]any{"message": msgInvalidCaptcha}, false
}

func logout(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	repo := authRepository(app)
	token := tokenFromCookie(r, "token")
	if token != "" {
		if session, ok := repo.RevokeSession(r.Context(), token, time.Now().UTC()); ok {
			userID := session.UserID
			if userID != "" {
				if ok := repo.SetUserStatus(r.Context(), userID, "offline"); !ok {
					slog.Warn("user status update skipped", "user_id", userID, "status", "offline")
				}
			}
			// Denylist the token so every replica rejects it immediately, even
			// before the store delete is observed elsewhere (finding 1).
			revokeCredential(app, r.Context(), "session", token, session.ExpiresAt)
		}
	}
	if refresh := tokenFromCookie(r, "refresh_token"); refresh != "" {
		repo.DeleteRefreshToken(r.Context(), refresh)
	}
	return rawJSON(r, http.StatusOK, nil, clearAuthCookies(r))
}

func refreshToken(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	payload, _ := decodePayload(r)
	refresh := shared.FirstNonEmpty(
		strings.TrimSpace(textValue(payload, "refresh_token")),
		strings.TrimSpace(r.FormValue("refresh_token")),
		tokenFromCookie(r, "refresh_token"),
	)
	if refresh == "" {
		return http.StatusBadRequest, map[string]any{"message": "refresh_token is required"}, nil
	}
	repo := authRepository(app)
	record, ok := repo.ConsumeRefreshToken(r.Context(), refresh, time.Now().UTC())
	if !ok {
		return http.StatusUnauthorized, map[string]any{"message": "Invalid refresh token"}, nil
	}
	userRecord, ok := repo.FindActiveUserByID(r.Context(), record.UserID)
	if !ok {
		return http.StatusUnauthorized, map[string]any{"message": "Invalid refresh token"}, nil
	}
	data, err := issueSession(app, r, userRecord.Data)
	if err != nil {
		return http.StatusServiceUnavailable, map[string]any{"message": "session could not be issued"}, nil
	}
	cookies := authCookies(r, textValue(data, "token"), textValue(data, "refresh_token"))
	return rawJSON(r, http.StatusOK, map[string]any{"token": data["token"], "refresh_token": data["refresh_token"]}, cookies)
}

func cliLogin(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	payload, err := decodePayload(r)
	if err != nil {
		return http.StatusBadRequest, map[string]any{"message": msgInvalidInputTitle}, nil
	}
	username := strings.TrimSpace(textValue(payload, "username"))
	password := textValue(payload, "password")
	name := strings.TrimSpace(shared.FirstNonEmpty(textValue(payload, "name"), "nexuspaas-cli"))
	if username == "" || password == "" || len(name) > 64 {
		return http.StatusBadRequest, map[string]any{"message": "username, password, and a valid token name are required"}, nil
	}
	if loginLocked(app, r, username) {
		publishLoginFailure(app, r, username, pathCLILogin, reasonLocked)
		return http.StatusUnauthorized, map[string]any{"message": msgInvalidCredentials}, nil
	}
	user, ok := authenticateUser(app, r, username, password)
	if !ok {
		recordLoginFailure(app, r, username)
		publishLoginFailure(app, r, username, pathCLILogin, reasonInvalidCredentials)
		return http.StatusUnauthorized, map[string]any{"message": msgInvalidCredentials}, nil
	}
	clearLoginFailures(app, r, username)
	created, status, errData := createAPITokenForUser(app, r, textValue(user, "id"), name)
	if status != http.StatusCreated {
		return status, errData, nil
	}
	return http.StatusOK, map[string]any{
		"token":      created["token"],
		"token_id":   created["id"],
		"expires_at": created["expires_at"],
		"user":       publicUser(user),
	}, nil
}

func issueSession(app *platform.App, r *http.Request, user map[string]any) (map[string]any, error) {
	userID := textValue(user, "id")
	pair, err := authRepository(app).IssueSessionPair(r.Context(), userID, time.Now().UTC(), 24*time.Hour, 7*24*time.Hour)
	if err != nil {
		return nil, err
	}
	if ok := authRepository(app).SetUserStatus(r.Context(), userID, "online"); !ok {
		slog.Warn("user status update skipped", "user_id", userID, "status", "online")
	}
	user["status"] = "online"
	return map[string]any{
		"token":         pair.AccessToken,
		"refresh_token": pair.RefreshToken,
		"user":          publicUser(user),
	}, nil
}

func authCookies(r *http.Request, token, refresh string) []string {
	return []string{
		cookieString(r, "token", token, 24*time.Hour, 0),
		cookieString(r, "refresh_token", refresh, 7*24*time.Hour, 0),
	}
}

func clearAuthCookies(r *http.Request) []string {
	return []string{
		cookieString(r, "token", "", -time.Second, -1),
		cookieString(r, "refresh_token", "", -time.Second, -1),
	}
}

func cookieString(r *http.Request, name, value string, ttl time.Duration, maxAge int) string {
	cookie := &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isSecure(r),
		MaxAge:   maxAge,
	}
	if ttl > 0 {
		cookie.Expires = time.Now().UTC().Add(ttl)
	}
	return cookie.String()
}

func isSecure(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func tokenFromCookie(r *http.Request, name string) string {
	cookie, err := r.Cookie(name)
	if err != nil {
		return ""
	}
	return cookie.Value
}

func publishLoginFailure(app *platform.App, r *http.Request, username, path, reason string) {
	publish(app, r, "AuditEvent", map[string]any{
		"user_id":       "anonymous",
		"username":      strings.ToLower(strings.TrimSpace(username)),
		"action":        "login_failed",
		"resource_type": "auth",
		"resource":      path,
		"resource_id":   path,
		"success":       false,
		"reason":        reason,
		"source_ip":     requestIP(r),
		"user_agent":    r.UserAgent(),
	})
}

func loginLocked(app *platform.App, r *http.Request, username string) bool {
	record, ok := app.Store.Get(r.Context(), loginFailuresResource, loginFailureID(username, requestIP(r)))
	if !ok {
		return false
	}
	lockedUntil := textValue(record.Data, "locked_until")
	if lockedUntil == "" {
		return false
	}
	parsed, err := time.Parse(time.RFC3339, lockedUntil)
	if err != nil {
		return true
	}
	if time.Now().UTC().Before(parsed) {
		return true
	}
	app.Store.Delete(r.Context(), loginFailuresResource, record.ID)
	return false
}

func recordLoginFailure(app *platform.App, r *http.Request, username string) {
	ip := requestIP(r)
	id := loginFailureID(username, ip)
	count := 1
	updates := map[string]any{
		"id":         id,
		"username":   strings.ToLower(strings.TrimSpace(username)),
		"ip":         ip,
		"failures":   count,
		"updated_at": time.Now().UTC().Format(time.RFC3339),
	}
	if current, ok := app.Store.Get(r.Context(), loginFailuresResource, id); ok {
		count = intValue(current.Data, "failures", 0) + 1
		updates["failures"] = count
		if count > defaultLoginMaxFailed {
			updates["locked_until"] = time.Now().UTC().Add(defaultLoginLockout).Format(time.RFC3339)
		}
		if _, ok := app.Store.Update(r.Context(), loginFailuresResource, id, updates); !ok {
			slog.Warn("login failure update skipped", "id", id)
		}
		return
	}
	if count > defaultLoginMaxFailed {
		updates["locked_until"] = time.Now().UTC().Add(defaultLoginLockout).Format(time.RFC3339)
	}
	beforeLoginFailureCreate(app, r, id)
	if _, err := app.Store.Create(r.Context(), loginFailuresResource, updates); err != nil && platform.IsCreateConflict(err) {
		updateConflictingLoginFailure(app, r, id, updates)
	}
}

func updateConflictingLoginFailure(app *platform.App, r *http.Request, id string, updates map[string]any) {
	current, ok := app.Store.Get(r.Context(), loginFailuresResource, id)
	if !ok {
		return
	}
	count := intValue(current.Data, "failures", 0) + 1
	updates["failures"] = count
	if count > defaultLoginMaxFailed {
		updates["locked_until"] = time.Now().UTC().Add(defaultLoginLockout).Format(time.RFC3339)
	}
	if _, ok := app.Store.Update(r.Context(), loginFailuresResource, id, updates); !ok {
		slog.Warn("login failure update skipped", "id", id)
	}
}

func clearLoginFailures(app *platform.App, r *http.Request, username string) {
	app.Store.Delete(r.Context(), loginFailuresResource, loginFailureID(username, requestIP(r)))
}

func loginFailureID(username, ip string) string {
	return "LF" + hex.EncodeToString([]byte(strings.ToLower(strings.TrimSpace(username))+"|"+strings.TrimSpace(ip)))
}

func requestIP(r *http.Request) string {
	if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); forwarded != "" {
		if first := strings.TrimSpace(strings.Split(forwarded, ",")[0]); first != "" {
			return first
		}
	}
	return strings.TrimSpace(r.RemoteAddr)
}

func passwordMatches(user map[string]any, password string) bool {
	if platform.VerifySecret(textValue(user, "password_hash"), password) {
		return true
	}
	return textValue(user, "password") != "" && platform.VerifySecret("plain:"+textValue(user, "password"), password)
}
