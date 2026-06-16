package integrationproxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func loginToMinIOConsole(r *http.Request, rawURL, accessKey, secretKey string, timeout time.Duration) ([]*http.Cookie, error) {
	base, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("minio console url: %w", err)
	}
	payload, _ := json.Marshal(map[string]string{"accessKey": accessKey, "secretKey": secretKey})
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, strings.TrimRight(base.String(), "/")+"/api/v1/login", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("minio console login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: timeout}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("minio console unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("minio console login rejected (%d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return prefixCookiePaths(resp.Cookies(), minioProxyPrefix), nil
}

func loginToPgAdmin(r *http.Request, rawURL, email, password string, timeout time.Duration) ([]*http.Cookie, error) {
	base, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("pgadmin url: %w", err)
	}
	client := &http.Client{
		Timeout: timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	baseURL := strings.TrimRight(base.String(), "/") + pgadminProxyPrefix
	collected, csrfToken, err := fetchPgAdminCSRF(r, client, baseURL)
	if err != nil {
		return nil, err
	}
	collected, location, err := submitPgAdminLogin(r, client, baseURL, email, password, csrfToken, collected)
	if err != nil {
		return nil, err
	}
	if location != "" {
		collected = warmPgAdminSession(r, client, base, location, collected)
	}
	return prefixCookiePaths(collected, pgadminProxyPrefix), nil
}

func fetchPgAdminCSRF(r *http.Request, client *http.Client, baseURL string) ([]*http.Cookie, string, error) {
	req1, err := http.NewRequestWithContext(r.Context(), http.MethodGet, baseURL+"/login", nil)
	if err != nil {
		return nil, "", fmt.Errorf("pgadmin login request: %w", err)
	}
	copyBrowserHeaders(req1, r)
	resp1, err := client.Do(req1)
	if err != nil {
		return nil, "", fmt.Errorf("pgadmin unreachable: %w", err)
	}
	body1, _ := io.ReadAll(io.LimitReader(resp1.Body, 65536))
	resp1.Body.Close()
	collected := mergeCookies(nil, resp1.Cookies())
	match := csrfPattern.FindSubmatch(body1)
	if match == nil {
		return nil, "", fmt.Errorf("pgadmin login page: csrf token not found")
	}
	return collected, string(match[1]), nil
}

func submitPgAdminLogin(r *http.Request, client *http.Client, baseURL, email, password, csrfToken string, cookies []*http.Cookie) ([]*http.Cookie, string, error) {
	form := url.Values{"email": {email}, "password": {password}, "csrf_token": {csrfToken}}
	req2, err := http.NewRequestWithContext(r.Context(), http.MethodPost, baseURL+"/authenticate/login", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, "", fmt.Errorf("pgadmin auth request: %w", err)
	}
	copyBrowserHeaders(req2, r)
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req2.Header.Set("X-pgA-CSRFToken", csrfToken)
	addCookies(req2, cookies)
	resp2, err := client.Do(req2)
	if err != nil {
		return nil, "", fmt.Errorf("pgadmin login post failed: %w", err)
	}
	body2, _ := io.ReadAll(io.LimitReader(resp2.Body, 4096))
	resp2.Body.Close()
	collected := mergeCookies(cookies, resp2.Cookies())
	if err := validatePgAdminLoginResponse(resp2.StatusCode, body2); err != nil {
		return nil, "", err
	}
	return collected, strings.TrimSpace(resp2.Header.Get("Location")), nil
}

func validatePgAdminLoginResponse(status int, body []byte) error {
	if status != http.StatusFound && status != http.StatusOK {
		return fmt.Errorf("pgadmin login rejected (%d): %s", status, strings.TrimSpace(string(body)))
	}
	if status != http.StatusOK {
		return nil
	}
	var result struct {
		Success int    `json:"success"`
		ErrMsg  string `json:"errormsg"`
	}
	if err := json.Unmarshal(body, &result); err == nil && result.Success != 1 {
		return fmt.Errorf("pgadmin login failed: %s", result.ErrMsg)
	}
	return nil
}

func warmPgAdminSession(r *http.Request, client *http.Client, base *url.URL, location string, cookies []*http.Cookie) []*http.Cookie {
	if !strings.HasPrefix(location, "http://") && !strings.HasPrefix(location, "https://") {
		location = strings.TrimRight(base.String(), "/") + location
	}
	warmReq, err := http.NewRequestWithContext(r.Context(), http.MethodGet, location, nil)
	if err != nil {
		return cookies
	}
	copyBrowserHeaders(warmReq, r)
	addCookies(warmReq, cookies)
	warmup, err := client.Do(warmReq)
	if err != nil {
		return cookies
	}
	defer warmup.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(warmup.Body, 4096))
	return mergeCookies(cookies, warmup.Cookies())
}

func redirectWithCookies(status int, location string, cookies []*http.Cookie) (int, platform.RawResponse) {
	headers := map[string]string{"Location": location}
	headerValues := map[string][]string{}
	if len(cookies) > 0 {
		for _, cookie := range cookies {
			headerValues["Set-Cookie"] = append(headerValues["Set-Cookie"], cookie.String())
		}
	}
	return status, platform.RawResponse{Headers: headers, HeaderValues: headerValues}
}

func prefixCookiePaths(cookies []*http.Cookie, prefix string) []*http.Cookie {
	for _, cookie := range cookies {
		if cookie.Path == "" || cookie.Path == "/" {
			cookie.Path = prefix + "/"
		}
	}
	return cookies
}

func addCookies(req *http.Request, cookies []*http.Cookie) {
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
}

func mergeCookies(existing, incoming []*http.Cookie) []*http.Cookie {
	out := append([]*http.Cookie{}, existing...)
	for _, cookie := range incoming {
		if cookie.MaxAge < 0 {
			out = removeCookie(out, cookie)
			continue
		}
		out = upsertCookie(out, cookie)
	}
	return out
}

func removeCookie(cookies []*http.Cookie, target *http.Cookie) []*http.Cookie {
	for i, current := range cookies {
		if sameCookieSlot(current, target) {
			return append(cookies[:i], cookies[i+1:]...)
		}
	}
	return cookies
}

func upsertCookie(cookies []*http.Cookie, target *http.Cookie) []*http.Cookie {
	for i, current := range cookies {
		if sameCookieSlot(current, target) {
			cookies[i] = target
			return cookies
		}
	}
	return append(cookies, target)
}

func sameCookieSlot(a, b *http.Cookie) bool {
	return a.Name == b.Name && a.Path == b.Path
}

func copyBrowserHeaders(dst, src *http.Request) {
	for _, header := range []string{"User-Agent", headerForwardedFor, "X-Real-Ip", headerForwardedHost, "Accept-Language"} {
		if values := src.Header.Values(header); len(values) > 0 {
			dst.Header[header] = append([]string{}, values...)
		}
	}
	if dst.Header.Get(headerForwardedFor) == "" && src.RemoteAddr != "" {
		dst.Header.Set(headerForwardedFor, strings.Split(src.RemoteAddr, ":")[0])
	}
	if dst.Header.Get(headerForwardedHost) == "" && src.Host != "" {
		dst.Header.Set(headerForwardedHost, src.Host)
	}
}

func requireAdmin(app *platform.App, r *http.Request) (int, any, bool) {
	userID := currentUserID(r)
	if userID == "" {
		return http.StatusUnauthorized, map[string]any{"message": "unauthorized"}, false
	}
	if !hasAdminPanel(app, r, userID) {
		return http.StatusForbidden, map[string]any{"message": "admin access required"}, false
	}
	return 0, nil, true
}

func hasAdminPanel(app *platform.App, r *http.Request, userID string) bool {
	if app == nil || app.Store == nil {
		return false
	}
	syncIdentityAdminReadModels(app, r)
	roles := identityAdminRecords(app, r, proxyAdminRolesResource, identityRolesResource)
	for _, user := range identityAdminRecords(app, r, proxyAdminUsersResource, identityUsersResource) {
		if !identityAdminUserMatches(user, userID) {
			continue
		}
		if recordGrantsAdminPanel(user) {
			return true
		}
		roleID := textValue(user, "role_id", "roleId", "RoleID", "role", "Role")
		return identityRoleGrantsAdminPanel(roles, roleID)
	}
	for _, role := range roles {
		if directRoleGrantsAdminPanel(role, userID) {
			return true
		}
	}
	return false
}

func identityAdminUserMatches(user map[string]any, userID string) bool {
	return identityAdminReadModelID(proxyAdminUsersResource, user) == userID ||
		textValue(user, keyUserID, keyUserIDCamel, keyUserIDTitle) == userID
}

func identityRoleGrantsAdminPanel(roles []map[string]any, roleID string) bool {
	for _, role := range roles {
		if identityAdminReadModelID(proxyAdminRolesResource, role) == roleID || textValue(role, "name", "Name") == roleID {
			return recordGrantsAdminPanel(role)
		}
	}
	return false
}

func directRoleGrantsAdminPanel(role map[string]any, userID string) bool {
	return textValue(role, keyUserID, keyUserIDCamel, keyUserIDTitle) == userID && recordGrantsAdminPanel(role)
}

func syncIdentityAdminReadModels(app *platform.App, r *http.Request) {
	if app == nil || app.Store == nil || app.Events == nil {
		return
	}
	app.RunProjection(r.Context(), identityProjectionConsumer, func(event contracts.Event) error {
		return projectIdentityAdminEvent(app, r, event)
	})
}

func projectIdentityAdminEvent(app *platform.App, r *http.Request, event contracts.Event) error {
	resource, data, deleted, ok := identityAdminProjection(event)
	if !ok {
		return nil
	}
	if deleted {
		deleteIdentityAdminReadModel(app, r, resource, data)
		return nil
	}
	return upsertIdentityAdminReadModel(app, r, resource, data)
}

func identityAdminProjection(event contracts.Event) (string, map[string]any, bool, bool) {
	switch strings.ToLower(event.Name) {
	case "usercreated", "userupdated", "userdisabled":
		return proxyAdminUsersResource, identityEventData(event), false, true
	case "userdeleted":
		return proxyAdminUsersResource, identityEventData(event), true, true
	case "rolecreated", "roleupdated":
		return proxyAdminRolesResource, identityEventData(event), false, true
	case "roledeleted":
		return proxyAdminRolesResource, identityEventData(event), true, true
	default:
		return "", nil, false, false
	}
}

func identityEventData(event contracts.Event) map[string]any {
	if next, ok := event.Data["new"].(map[string]any); ok {
		return maps.Clone(next)
	}
	return maps.Clone(event.Data)
}

func upsertIdentityAdminReadModel(app *platform.App, r *http.Request, resource string, data map[string]any) error {
	id := identityAdminReadModelID(resource, data)
	if id == "" {
		return nil
	}
	data["id"] = id
	if _, ok := app.Store.Update(r.Context(), resource, id, data); ok {
		return nil
	}
	if _, err := app.Store.Create(r.Context(), resource, data); err != nil {
		if platform.IsCreateConflict(err) {
			if _, ok := app.Store.Update(r.Context(), resource, id, data); !ok {
				return fmt.Errorf("integration proxy identity projection conflict update missed for %s/%s", resource, id)
			}
			return nil
		}
		return fmt.Errorf("integration proxy identity projection create failed for %s/%s: %w", resource, id, err)
	}
	return nil
}

func deleteIdentityAdminReadModel(app *platform.App, r *http.Request, resource string, data map[string]any) {
	if deleted, ok := data["deleted"].(bool); ok && !deleted {
		return
	}
	if id := identityAdminReadModelID(resource, data); id != "" {
		app.Store.Delete(r.Context(), resource, id)
	}
}

func identityAdminReadModelID(resource string, data map[string]any) string {
	id := textValue(data, keyID, keyIDTitle)
	userID := textValue(data, keyUserID, keyUserIDCamel, keyUserIDTitle)
	name := textValue(data, keyName, keyNameTitle)
	switch resource {
	case proxyAdminUsersResource:
		return firstNonEmpty(id, userID)
	case proxyAdminRolesResource:
		return firstNonEmpty(id, name, userID)
	default:
		return firstNonEmpty(id, userID, name)
	}
}

func identityAdminRecords(app *platform.App, r *http.Request, localResource, sourceResource string) []map[string]any {
	local := recordMaps(app, r, localResource)
	if !identitySourceCoHosted(app, sourceResource) {
		return local
	}
	source := recordMaps(app, r, sourceResource)
	if len(local) == 0 {
		return source
	}
	return mergeIdentityAdminRecords(localResource, source, local)
}

func mergeIdentityAdminRecords(resource string, source, local []map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(source)+len(local))
	seen := map[string]bool{}
	for _, record := range local {
		if id := identityAdminReadModelID(resource, record); id != "" {
			seen[id] = true
		}
		out = append(out, record)
	}
	for _, record := range source {
		id := identityAdminReadModelID(resource, record)
		if id != "" && seen[id] {
			continue
		}
		out = append(out, record)
	}
	return out
}

func recordMaps(app *platform.App, r *http.Request, resource string) []map[string]any {
	if app == nil || app.Store == nil {
		return nil
	}
	records := app.Store.List(r.Context(), resource)
	out := make([]map[string]any, 0, len(records))
	for _, record := range records {
		out = append(out, maps.Clone(record.Data))
	}
	return out
}

func identitySourceCoHosted(app *platform.App, sourceResource string) bool {
	if app == nil {
		return false
	}
	owner, _, ok := strings.Cut(sourceResource, ":")
	return ok && app.Config.AllowsService(owner)
}

func recordGrantsAdminPanel(data map[string]any) bool {
	if boolValue(data, "admin_panel", "adminPanel", "AdminPanel") {
		return true
	}
	capabilities := mapValue(data, "capabilities", "Capabilities")
	return boolValue(capabilities, "admin_panel", "adminPanel", "AdminPanel")
}

func currentUserID(r *http.Request) string {
	return strings.TrimSpace(r.Header.Get("X-User-ID"))
}

func isDisconnected(data map[string]any) bool {
	if timeValue(data, "disconnected_at", "disconnectedAt", "DisconnectedAt") != nil {
		return true
	}
	status := strings.ToLower(textValue(data, "status", "Status"))
	return status == "disconnected" || status == "deleted"
}

func summarizeUsage(rows []UserUsage) usageSummary {
	summary := usageSummary{UniqueUsers: len(rows)}
	for _, row := range rows {
		summary.SessionCount += row.SessionCount
		summary.TotalUploadBytes += row.UploadBytes
		summary.TotalDownloadBytes += row.DownloadBytes
		summary.TotalBytes += row.TotalBytes
	}
	return summary
}

func defaultMonthStart() time.Time {
	now := time.Now()
	return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
}

func resolveDate(raw string, fallback time.Time) time.Time {
	if parsed, err := time.Parse("2006-01-02", strings.TrimSpace(raw)); err == nil {
		return parsed
	}
	return fallback
}

func externalURL(app *platform.App, name string) string {
	if app == nil {
		return ""
	}
	return strings.TrimSpace(app.Config.ExternalURLs[name])
}

func parsePositiveInt(raw string) (int, error) {
	var value int
	for _, ch := range raw {
		if ch < '0' || ch > '9' {
			return 0, fmt.Errorf("not a positive integer")
		}
		value = value*10 + int(ch-'0')
	}
	if value <= 0 {
		return 0, fmt.Errorf("not a positive integer")
	}
	return value, nil
}

func firstTimeValue(data map[string]any, keys ...string) *time.Time {
	for _, key := range keys {
		if value := timeValue(data, key); value != nil {
			return value
		}
	}
	return nil
}

func timeString(data map[string]any, keys ...string) string {
	if value := timeValue(data, keys...); value != nil {
		return value.UTC().Format(time.RFC3339)
	}
	return textValue(data, keys...)
}

func timeValue(data map[string]any, keys ...string) *time.Time {
	for _, key := range keys {
		switch value := data[key].(type) {
		case time.Time:
			copy := value.UTC()
			return &copy
		case string:
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02"} {
				if parsed, err := time.Parse(layout, value); err == nil {
					parsed = parsed.UTC()
					return &parsed
				}
			}
		}
	}
	return nil
}

func textValue(data map[string]any, keys ...string) string {
	for _, key := range keys {
		switch value := data[key].(type) {
		case string:
			if strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
		case fmt.Stringer:
			if strings.TrimSpace(value.String()) != "" {
				return strings.TrimSpace(value.String())
			}
		}
	}
	return ""
}

func int64Value(data map[string]any, keys ...string) int64 {
	for _, key := range keys {
		switch value := data[key].(type) {
		case int:
			return int64(value)
		case int64:
			return value
		case float64:
			return int64(value)
		case json.Number:
			parsed, _ := value.Int64()
			return parsed
		}
	}
	return 0
}

func boolValue(data map[string]any, keys ...string) bool {
	for _, key := range keys {
		switch value := data[key].(type) {
		case bool:
			return value
		case string:
			parsed := strings.EqualFold(strings.TrimSpace(value), "true")
			if parsed {
				return true
			}
		}
	}
	return false
}

func mapValue(data map[string]any, keys ...string) map[string]any {
	for _, key := range keys {
		if value, ok := data[key].(map[string]any); ok {
			return value
		}
	}
	return map[string]any{}
}

func recordID(record contracts.Record[map[string]any]) string {
	return firstNonEmpty(record.ID, textValue(record.Data, "id", "ID"))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
