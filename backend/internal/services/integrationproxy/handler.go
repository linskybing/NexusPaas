package integrationproxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

const (
	serviceName                = "integration-proxy-service"
	identityProjectionConsumer = serviceName + ":identity_admin_projection"
	proxyAdminRolesResource    = serviceName + ":admin_roles"
	proxyAdminUsersResource    = serviceName + ":admin_users"
	identityRolesResource      = "identity-service:roles"
	identityUsersResource      = "identity-service:users"
	vpnClientsResource         = serviceName + ":vpn_clients"
	vpnUsageResource           = serviceName + ":vpn_usage_sessions"
	minioProxyPrefix           = "/api/v1/minio-console"
	pgadminProxyPrefix         = "/api/v1/pgadmin"

	keyID          = "id"
	keyIDTitle     = "ID"
	keyName        = "name"
	keyNameTitle   = "Name"
	keyUserID      = "user_id"
	keyUserIDCamel = "userId"
	keyUserIDTitle = "UserID"

	headerForwardedFor  = "X-Forwarded-For"
	headerForwardedHost = "X-Forwarded-Host"
)

var csrfPattern = regexp.MustCompile(`"csrfToken":\s*"([^"]+)"`)

type Client struct {
	CommonName     string `json:"commonName"`
	RealAddress    string `json:"realAddress"`
	VirtualAddress string `json:"virtualAddress"`
	BytesReceived  int64  `json:"bytesReceived"`
	BytesSent      int64  `json:"bytesSent"`
	ConnectedSince string `json:"connectedSince"`
	Node           string `json:"node"`
}

type ClientsResponse struct {
	Clients []Client `json:"clients"`
	Total   int      `json:"total"`
}

type UserUsage struct {
	Username      string     `json:"username"`
	SessionCount  int64      `json:"sessionCount"`
	UploadBytes   int64      `json:"uploadBytes"`
	DownloadBytes int64      `json:"downloadBytes"`
	TotalBytes    int64      `json:"totalBytes"`
	FirstSeen     *time.Time `json:"firstSeen"`
	LastSeen      *time.Time `json:"lastSeen"`
}

type usageSummary struct {
	UniqueUsers        int   `json:"uniqueUsers"`
	SessionCount       int64 `json:"sessionCount"`
	TotalUploadBytes   int64 `json:"totalUploadBytes"`
	TotalDownloadBytes int64 `json:"totalDownloadBytes"`
	TotalBytes         int64 `json:"totalBytes"`
}

type usageResponse struct {
	Rows     []UserUsage    `json:"rows"`
	Summary  usageSummary   `json:"summary"`
	Since    string         `json:"since"`
	Until    string         `json:"until,omitempty"`
	Filters  map[string]any `json:"filters"`
	RowCount int            `json:"rowCount"`
}

func Register(app *platform.App) {
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/admin/vpn/clients", listClients)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/admin/vpn/usage", getUsage)
	app.RegisterCustomHandler(http.MethodDelete, "/api/v1/admin/vpn/clients/{cn}", disconnectClient)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/minio-console-sso", minioSSOLogin)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/pgadmin-sso", pgadminSSOLogin)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/pgadmin-auth-check", pgadminAuthCheck)
	registerVPNUsageCollector(app)
}

func listClients(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	if status, data, ok := requireAdmin(app, r); !ok {
		return status, data, nil
	}
	clients, err := liveVPNClients(app, r)
	if err != nil {
		return http.StatusBadGateway, map[string]any{"error": err.Error()}, nil
	}
	if clients == nil {
		clients = activeVPNClients(app, r)
	}
	return http.StatusOK, ClientsResponse{Clients: clients, Total: len(clients)}, nil
}

func disconnectClient(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	if status, data, ok := requireAdmin(app, r); !ok {
		return status, data, nil
	}
	cn := strings.TrimSpace(r.PathValue("cn"))
	if cn == "" {
		return http.StatusBadRequest, map[string]any{"message": "common name required"}, nil
	}
	if handled, err := disconnectLiveVPNClient(app, r, cn); handled {
		if err != nil {
			return http.StatusBadGateway, map[string]any{"error": err.Error()}, nil
		}
		return http.StatusOK, nil, nil
	}
	found := false
	now := time.Now().UTC()
	for _, record := range app.Store.List(r.Context(), vpnClientsResource) {
		if textValue(record.Data, "commonName", "common_name", "CommonName") != cn || isDisconnected(record.Data) {
			continue
		}
		found = true
		if _, ok := app.Store.Update(r.Context(), vpnClientsResource, record.ID, map[string]any{
			"status":          "disconnected",
			"disconnected_at": now,
		}); !ok {
			slog.Warn("vpn client status update skipped", "client_id", record.ID)
		}
	}
	if !found {
		return http.StatusBadGateway, map[string]any{"error": fmt.Sprintf("client %q not connected to any vpn pod", cn)}, nil
	}
	return http.StatusOK, nil, nil
}

func liveVPNClients(app *platform.App, r *http.Request) ([]Client, error) {
	return configuredVPNFetcher(r.Context(), app.Config)
}

func disconnectLiveVPNClient(app *platform.App, r *http.Request, commonName string) (bool, error) {
	baseURLs := vpnAPIURLs(app.Config)
	if len(baseURLs) == 0 {
		return false, nil
	}
	notFound := 0
	failures := []string{}
	for _, baseURL := range baseURLs {
		found, err := disconnectFromVPNAPI(app.Config, r, baseURL, commonName)
		if err != nil {
			failures = append(failures, err.Error())
			continue
		}
		if found {
			return true, nil
		}
		notFound++
	}
	if notFound == len(baseURLs) {
		return true, fmt.Errorf("client %q not connected to any vpn pod", commonName)
	}
	return true, fmt.Errorf("disconnect failed, unreachable vpn APIs: %s", strings.Join(failures, "; "))
}

// fetchVPNClients performs the request-independent GET /api/v1/vpn/clients call
// against a single VPN gateway. The admin handler and the lease-gated usage
// collector both depend on it so gateway fetching has a single source of truth.
func fetchVPNClients(ctx context.Context, cfg platform.Config, baseURL string) ([]Client, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("vpn api url: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(parsed.String(), "/")+"/api/v1/vpn/clients", nil)
	if err != nil {
		return nil, fmt.Errorf("vpn clients request: %w", err)
	}
	if apiKey := strings.TrimSpace(cfg.VPNAPIKey); apiKey != "" {
		req.Header.Set("X-API-Key", apiKey)
	}
	resp, err := (&http.Client{Timeout: cfg.VPNAPITimeout}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s unreachable: %w", parsed.Host, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("%s returned %d: %s", parsed.Host, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var result ClientsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("%s decode: %w", parsed.Host, err)
	}
	for i := range result.Clients {
		if result.Clients[i].Node == "" {
			result.Clients[i].Node = parsed.Host
		}
	}
	return result.Clients, nil
}

func disconnectFromVPNAPI(cfg platform.Config, r *http.Request, baseURL, commonName string) (bool, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return false, fmt.Errorf("vpn api url: %w", err)
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodDelete, strings.TrimRight(parsed.String(), "/")+"/api/v1/vpn/clients/"+url.PathEscape(commonName), nil)
	if err != nil {
		return false, fmt.Errorf("vpn disconnect request: %w", err)
	}
	if apiKey := strings.TrimSpace(cfg.VPNAPIKey); apiKey != "" {
		req.Header.Set("X-API-Key", apiKey)
	}
	resp, err := (&http.Client{Timeout: cfg.VPNAPITimeout}).Do(req)
	if err != nil {
		return false, fmt.Errorf("%s unreachable: %w", parsed.Host, err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return false, fmt.Errorf("%s returned %d: %s", parsed.Host, resp.StatusCode, strings.TrimSpace(string(body)))
	}
}

func getUsage(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	if status, data, ok := requireAdmin(app, r); !ok {
		return status, data, nil
	}
	since := defaultMonthStart()
	if raw := strings.TrimSpace(r.URL.Query().Get("since")); raw != "" {
		since = resolveDate(raw, since)
	}
	username := strings.TrimSpace(r.URL.Query().Get("username"))
	var until *time.Time
	if raw := strings.TrimSpace(r.URL.Query().Get("until")); raw != "" {
		parsed := resolveDate(raw, time.Now()).Add(24*time.Hour - time.Nanosecond)
		until = &parsed
	}

	rows := aggregateUsage(app, r, since, until, username)
	resp := usageResponse{
		Rows:    rows,
		Summary: summarizeUsage(rows),
		Since:   since.Format("2006-01-02"),
		Filters: map[string]any{
			"username": username,
		},
		RowCount: len(rows),
	}
	if until != nil {
		resp.Until = until.Format("2006-01-02")
	}
	return http.StatusOK, resp, nil
}

func minioSSOLogin(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	if status, data, ok := requireAdmin(app, r); !ok {
		return status, data, nil
	}
	rawURL := firstNonEmpty(externalURL(app, "minio-console"), externalURL(app, "minio"))
	accessKey := strings.TrimSpace(app.Config.MinIOConsoleAccessKey)
	secretKey := strings.TrimSpace(app.Config.MinIOConsoleSecretKey)
	if rawURL == "" || accessKey == "" || secretKey == "" {
		return http.StatusBadGateway, map[string]any{"error": "minio console credentials not configured"}, nil
	}
	cookies, err := loginToMinIOConsole(r, rawURL, accessKey, secretKey, app.Config.MinIOOperationTimeout)
	if err != nil {
		return http.StatusBadGateway, map[string]any{"error": err.Error()}, nil
	}
	status, response := redirectWithCookies(http.StatusFound, minioProxyPrefix+"/", cookies)
	return status, response, nil
}

func pgadminSSOLogin(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	if status, data, ok := requireAdmin(app, r); !ok {
		return status, data, nil
	}
	rawURL := externalURL(app, "pgadmin")
	email := strings.TrimSpace(app.Config.PGAdminDefaultEmail)
	password := strings.TrimSpace(app.Config.PGAdminDefaultPassword)
	if rawURL == "" || email == "" || password == "" {
		return http.StatusBadGateway, map[string]any{"error": "pgadmin service account credentials not configured"}, nil
	}
	cookies, err := loginToPgAdmin(r, rawURL, email, password, app.Config.PGAdminSSOHTTPTimeout)
	if err != nil {
		return http.StatusBadGateway, map[string]any{"error": err.Error()}, nil
	}
	status, response := redirectWithCookies(http.StatusFound, pgadminProxyPrefix+"/browser/", cookies)
	return status, response, nil
}

func pgadminAuthCheck(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	if status, data, ok := requireAdmin(app, r); !ok {
		return status, data, nil
	}
	return http.StatusOK, platform.RawResponse{Body: nil}, nil
}

func activeVPNClients(app *platform.App, r *http.Request) []Client {
	seen := map[string]bool{}
	out := []Client{}
	for _, record := range app.Store.List(r.Context(), vpnClientsResource) {
		data := record.Data
		if isDisconnected(data) {
			continue
		}
		client := Client{
			CommonName:     textValue(data, "commonName", "common_name", "CommonName"),
			RealAddress:    textValue(data, "realAddress", "real_address", "RealAddress"),
			VirtualAddress: textValue(data, "virtualAddress", "virtual_address", "VirtualAddress"),
			BytesReceived:  int64Value(data, "bytesReceived", "bytes_received", "BytesReceived"),
			BytesSent:      int64Value(data, "bytesSent", "bytes_sent", "BytesSent"),
			ConnectedSince: timeString(data, "connectedSince", "connected_since", "ConnectedSince"),
			Node:           textValue(data, "node", "Node"),
		}
		if client.CommonName == "" {
			continue
		}
		key := client.CommonName + "|" + client.RealAddress
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, client)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CommonName == out[j].CommonName {
			return out[i].RealAddress < out[j].RealAddress
		}
		return out[i].CommonName < out[j].CommonName
	})
	return out
}

func mergeClients(perPod [][]Client) []Client {
	seen := map[string]bool{}
	out := []Client{}
	for _, clients := range perPod {
		for _, client := range clients {
			if client.CommonName == "" {
				continue
			}
			key := client.CommonName + "|" + client.RealAddress
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, client)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CommonName == out[j].CommonName {
			return out[i].RealAddress < out[j].RealAddress
		}
		return out[i].CommonName < out[j].CommonName
	})
	return out
}

func vpnAPIURLs(cfg platform.Config) []string {
	out := make([]string, 0, len(cfg.VPNAPIURLs))
	for _, item := range cfg.VPNAPIURLs {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

func aggregateUsage(app *platform.App, r *http.Request, since time.Time, until *time.Time, username string) []UserUsage {
	byUser := map[string]*UserUsage{}
	for _, record := range app.Store.List(r.Context(), vpnUsageResource) {
		accumulateUsageRecord(byUser, record.Data, since, until, username)
	}
	out := make([]UserUsage, 0, len(byUser))
	for _, usage := range byUser {
		out = append(out, *usage)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].TotalBytes == out[j].TotalBytes {
			return out[i].Username < out[j].Username
		}
		return out[i].TotalBytes > out[j].TotalBytes
	})
	return out
}

func accumulateUsageRecord(byUser map[string]*UserUsage, data map[string]any, since time.Time, until *time.Time, username string) {
	rowUsername, connectedSince, lastSeen, ok := usageRecordWindow(data, since, until, username)
	if !ok {
		return
	}
	usage := byUser[rowUsername]
	if usage == nil {
		usage = &UserUsage{Username: rowUsername}
		byUser[rowUsername] = usage
	}
	usage.SessionCount++
	usage.UploadBytes += int64Value(data, "uploadBytes", "upload_bytes", "bytesReceived", "bytes_received", "BytesReceived")
	usage.DownloadBytes += int64Value(data, "downloadBytes", "download_bytes", "bytesSent", "bytes_sent", "BytesSent")
	usage.TotalBytes = usage.UploadBytes + usage.DownloadBytes
	updateUsageBounds(usage, connectedSince, lastSeen)
}

func usageRecordWindow(data map[string]any, since time.Time, until *time.Time, username string) (string, *time.Time, *time.Time, bool) {
	rowUsername := textValue(data, "username", "Username")
	if rowUsername == "" || username != "" && rowUsername != username {
		return "", nil, nil, false
	}
	connectedSince := timeValue(data, "connectedSince", "connected_since", "ConnectedSince")
	lastSeen := firstTimeValue(data, "disconnectedAt", "disconnected_at", "DisconnectedAt", "lastSeenAt", "last_seen_at", "LastSeenAt")
	if connectedSince == nil && lastSeen == nil {
		return "", nil, nil, false
	}
	if lastSeen == nil {
		lastSeen = connectedSince
	}
	if connectedSince == nil {
		connectedSince = lastSeen
	}
	if lastSeen.Before(since) || until != nil && connectedSince.After(*until) {
		return "", nil, nil, false
	}
	return rowUsername, connectedSince, lastSeen, true
}

func updateUsageBounds(usage *UserUsage, connectedSince, lastSeen *time.Time) {
	if usage.FirstSeen == nil || connectedSince.Before(*usage.FirstSeen) {
		copy := *connectedSince
		usage.FirstSeen = &copy
	}
	if usage.LastSeen == nil || lastSeen.After(*usage.LastSeen) {
		copy := *lastSeen
		usage.LastSeen = &copy
	}
}
