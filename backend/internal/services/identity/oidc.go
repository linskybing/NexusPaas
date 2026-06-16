package identity

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

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

func oidcDeviceAuthorization(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	if strings.TrimSpace(oidcFields(r)["client_id"]) == "" {
		return http.StatusBadRequest, map[string]any{"message": "client_id is required"}, nil
	}
	return oidcProviderUnavailable(app, r, platform.RouteSpec{})
}

func oidcWellKnown(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	path := strings.Trim(strings.TrimSpace(r.PathValue("path")), "/")
	if path != "openid-configuration" {
		return http.StatusNotFound, map[string]any{"message": "OIDC discovery path not found"}, nil
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
	if strings.TrimSpace(fields["code"]) == "" || strings.TrimSpace(fields["state"]) == "" {
		return http.StatusBadRequest, map[string]any{"message": "code and state are required"}, nil
	}
	return oidcProviderUnavailable(app, r, platform.RouteSpec{})
}

func oidcProviderUnavailable(_ *platform.App, _ *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	return http.StatusServiceUnavailable, map[string]any{
		"message": "OIDC provider unavailable",
		"reason":  "oidc_provider_not_configured",
	}, nil
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
