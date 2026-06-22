package identity

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

const oidcStateCookieName = "nexuspaas_oidc_state"

func oidcStart(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	if strings.TrimSpace(app.Config.DexURL) == "" {
		return oidcProviderUnavailable(app, r, platform.RouteSpec{})
	}
	state := oidcRandomState()
	values := url.Values{
		"response_type": {"code"},
		"client_id":     {"platform"},
		"redirect_uri":  {oidcRedirectURI(r)},
		"scope":         {"openid email profile"},
		"state":         {state},
	}
	return http.StatusFound, platform.RawResponse{
		Headers: map[string]string{"Location": "/api/v1/oidc/authorize?" + values.Encode()},
		HeaderValues: map[string][]string{
			"Set-Cookie": {oidcStateCookie(r, state, 300)},
		},
	}, nil
}

func oidcLoginForm(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	authRequestID := strings.TrimSpace(r.URL.Query().Get("auth_request_id"))
	if authRequestID == "" {
		return http.StatusBadRequest, map[string]any{"message": "missing auth_request_id"}, nil
	}
	return oidcProviderUnavailable(app, r, platform.RouteSpec{})
}

func oidcLogin(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	fields := oidcFields(r)
	if strings.TrimSpace(fields["auth_request_id"]) == "" || strings.TrimSpace(fields["username"]) == "" || fields["password"] == "" {
		return http.StatusBadRequest, map[string]any{"message": msgInvalidInput}, nil
	}
	return oidcProviderUnavailable(app, r, platform.RouteSpec{})
}

func oidcToken(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	fields := oidcFields(r)
	grantType := strings.TrimSpace(fields["grant_type"])
	if grantType == "" || strings.TrimSpace(fields["client_id"]) == "" {
		return http.StatusBadRequest, map[string]any{"message": "grant_type and client_id are required"}, nil
	}
	switch grantType {
	case "authorization_code":
		if strings.TrimSpace(fields["code"]) == "" {
			return http.StatusBadRequest, map[string]any{"message": "code is required"}, nil
		}
	case "refresh_token":
		if strings.TrimSpace(fields["refresh_token"]) == "" {
			return http.StatusBadRequest, map[string]any{"message": "refresh_token is required"}, nil
		}
	case "client_credentials", "urn:ietf:params:oauth:grant-type:device_code":
		return oidcProviderUnavailable(app, r, platform.RouteSpec{})
	default:
		return http.StatusBadRequest, map[string]any{"message": "unsupported grant_type"}, nil
	}
	return oidcProviderUnavailable(app, r, platform.RouteSpec{})
}

func oidcRevoke(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	if strings.TrimSpace(oidcFields(r)["token"]) == "" {
		return http.StatusBadRequest, map[string]any{"message": "token is required"}, nil
	}
	return oidcProviderUnavailable(app, r, platform.RouteSpec{})
}

func oidcAuthorize(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	fields := oidcFields(r)
	if strings.TrimSpace(fields["client_id"]) == "" || strings.TrimSpace(fields["response_type"]) == "" || strings.TrimSpace(fields["redirect_uri"]) == "" {
		return http.StatusBadRequest, map[string]any{"message": "client_id, response_type, and redirect_uri are required"}, nil
	}
	return oidcProviderUnavailable(app, r, platform.RouteSpec{})
}

func oidcUserInfo(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(header, "Bearer ") || strings.TrimSpace(strings.TrimPrefix(header, "Bearer ")) == "" {
		return http.StatusUnauthorized, map[string]any{"message": "bearer token is required"}, nil
	}
	return oidcProviderUnavailable(app, r, platform.RouteSpec{})
}

func oidcCallback(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	fields := oidcFields(r)
	code := strings.TrimSpace(fields["code"])
	state := strings.TrimSpace(fields["state"])
	if code == "" || state == "" {
		return http.StatusBadRequest, map[string]any{"message": "code and state are required"}, nil
	}
	if strings.TrimSpace(app.Config.DexURL) == "" {
		return oidcProviderUnavailable(app, r, platform.RouteSpec{})
	}
	if !oidcStateValid(r, state) {
		return http.StatusBadRequest, map[string]any{"message": "invalid OIDC state"}, nil
	}
	token, ok := exchangeOIDCCode(app, r, code)
	if !ok {
		return http.StatusBadGateway, map[string]any{"message": "OIDC token exchange failed", "reason": "oidc_provider_unreachable"}, nil
	}
	user, ok := oidcCallbackUser(app, r, token)
	if !ok {
		return http.StatusUnauthorized, map[string]any{"message": "OIDC user is not registered"}, nil
	}
	data, err := issueSession(app, r, user.Data)
	if err != nil {
		return http.StatusServiceUnavailable, map[string]any{"message": "session could not be issued"}, nil
	}
	cookies := append(authCookies(r, textValue(data, "token"), textValue(data, "refresh_token")), clearOIDCStateCookie(r))
	return http.StatusFound, platform.RawResponse{
		Headers:      map[string]string{"Location": "/ui/?auth=oidc"},
		HeaderValues: map[string][]string{"Set-Cookie": cookies},
	}, nil
}

func oidcProviderUnavailable(_ *platform.App, _ *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	return http.StatusServiceUnavailable, map[string]any{
		"message": "OIDC provider unavailable",
		"reason":  "oidc_provider_not_configured",
	}, nil
}

func oidcRandomState() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return platform.NewUUID()
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}

func oidcStateCookie(r *http.Request, value string, maxAge int) string {
	cookie := &http.Cookie{
		Name:     oidcStateCookieName,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isSecure(r),
		MaxAge:   maxAge,
	}
	if maxAge > 0 {
		cookie.Expires = time.Now().UTC().Add(time.Duration(maxAge) * time.Second)
	} else {
		cookie.Expires = time.Now().UTC().Add(-time.Second)
	}
	return cookie.String()
}

func oidcStateValid(r *http.Request, state string) bool {
	cookie, err := r.Cookie(oidcStateCookieName)
	return err == nil && cookie.Value != "" && cookie.Value == state
}

func clearOIDCStateCookie(r *http.Request) string {
	return oidcStateCookie(r, "", -1)
}

func exchangeOIDCCode(app *platform.App, r *http.Request, code string) (map[string]any, bool) {
	form := url.Values{
		"grant_type":   {"authorization_code"},
		"client_id":    {"platform"},
		"code":         {code},
		"redirect_uri": {oidcRedirectURI(r)},
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, strings.TrimRight(app.Config.DexURL, "/")+"/token", strings.NewReader(form.Encode()))
	if err != nil {
		slog.Warn("oidc token request build failed", "error", err)
		return nil, false
	}
	req.Header.Set(headerContentType, "application/x-www-form-urlencoded")
	resp, err := dexClient.Do(req)
	if err != nil {
		slog.Warn("oidc token exchange failed", "error", err)
		return nil, false
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		slog.Warn("oidc token response read failed", "error", err)
		return nil, false
	}
	if resp.StatusCode != http.StatusOK {
		slog.Warn("oidc token exchange returned non-OK", "status", resp.StatusCode)
		return nil, false
	}
	var token map[string]any
	if err := json.Unmarshal(raw, &token); err != nil {
		slog.Warn("oidc token response decode failed", "error", err)
		return nil, false
	}
	return token, true
}

func oidcRedirectURI(r *http.Request) string {
	scheme := "http"
	if isSecure(r) {
		scheme = "https"
	}
	if forwarded, ok := oidcForwardedProto(r.Header); ok {
		scheme = forwarded
	}
	host := r.Host
	if forwarded, ok := oidcForwardedHost(r.Header); ok {
		host = forwarded
	}
	return scheme + "://" + host + "/api/v1/oidc/callback"
}

func oidcForwardedProto(header http.Header) (string, bool) {
	return oidcValidForwardedProto(header.Values("X-Forwarded-Proto"))
}

func oidcForwardedHost(header http.Header) (string, bool) {
	return oidcValidForwardedHost(header.Values("X-Forwarded-Host"))
}

func oidcValidForwardedProto(values []string) (string, bool) {
	if len(values) != 1 {
		return "", false
	}
	value := strings.ToLower(strings.TrimSpace(values[0]))
	if strings.Contains(value, ",") || (value != "http" && value != "https") {
		return "", false
	}
	return value, true
}

func oidcValidForwardedHost(values []string) (string, bool) {
	if len(values) != 1 {
		return "", false
	}
	return oidcValidHostAuthority(values[0])
}

func oidcValidHostAuthority(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" || strings.Contains(value, ",") || strings.ContainsAny(value, " \t\r\n/\\@") {
		return "", false
	}
	host := value
	if splitHost, port, err := net.SplitHostPort(value); err == nil {
		if !oidcValidPort(port) {
			return "", false
		}
		host = splitHost
	} else if strings.Contains(value, ":") {
		if !strings.HasPrefix(value, "[") || !strings.HasSuffix(value, "]") {
			return "", false
		}
		host = strings.TrimSuffix(strings.TrimPrefix(value, "["), "]")
	}
	host = strings.Trim(host, "[]")
	if host == "" || strings.Contains(host, ",") || strings.ContainsAny(host, " \t\r\n/\\@") {
		return "", false
	}
	return value, true
}

func oidcValidPort(port string) bool {
	if port == "" {
		return false
	}
	value, err := strconv.Atoi(port)
	return err == nil && value > 0 && value <= 65535
}

func oidcCallbackUser(app *platform.App, r *http.Request, token map[string]any) (identityUser, bool) {
	claims := oidcIDTokenClaims(textValue(token, "id_token"))
	for _, identifier := range []string{
		textValue(claims, "preferred_username"),
		textValue(claims, "email"),
		textValue(claims, "sub"),
	} {
		record, ok := principalRepository(app).FindUserByIdentifier(r.Context(), identifier)
		if !ok {
			continue
		}
		if user, ok := userFromRecord(record); ok {
			return user, true
		}
	}
	return identityUser{}, false
}

func oidcIDTokenClaims(idToken string) map[string]any {
	parts := strings.Split(idToken, ".")
	if len(parts) < 2 {
		return map[string]any{}
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return map[string]any{}
	}
	var claims map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&claims); err != nil {
		return map[string]any{}
	}
	return claims
}

func oidcFields(r *http.Request) map[string]string {
	fields := map[string]string{}
	for key, values := range r.URL.Query() {
		setFirstField(fields, key, values)
	}
	if r.Body == nil {
		return fields
	}
	body, _ := io.ReadAll(r.Body)
	_ = r.Body.Close()
	if len(body) == 0 {
		return fields
	}
	contentType := strings.ToLower(r.Header.Get(headerContentType))
	if strings.Contains(contentType, "application/x-www-form-urlencoded") {
		mergeFormFields(fields, body)
		return fields
	}
	mergeJSONFields(fields, body)
	return fields
}

func setFirstField(fields map[string]string, key string, values []string) {
	if len(values) > 0 {
		fields[key] = values[0]
	}
}

func mergeFormFields(fields map[string]string, body []byte) {
	values, err := url.ParseQuery(string(body))
	if err != nil {
		return
	}
	for key, value := range values {
		setFirstField(fields, key, value)
	}
}

func mergeJSONFields(fields map[string]string, body []byte) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return
	}
	for key, value := range payload {
		fields[key] = textValue(payload, key)
		if fields[key] == "" && value != nil {
			fields[key] = fmt.Sprint(value)
		}
	}
}
