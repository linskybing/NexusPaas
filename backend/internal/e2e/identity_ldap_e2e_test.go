//go:build e2e

package e2e

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"

	"github.com/go-ldap/ldap/v3"
)

func TestIdentityLDAPStrategyMirrorSyncE2E(t *testing.T) {
	if strings.TrimSpace(os.Getenv("TEST_LIVE_LDAP_IDENTITY")) != "1" {
		t.Skip("set TEST_LIVE_LDAP_IDENTITY=1 to run live OpenLDAP identity e2e")
	}
	ldapCfg := liveLDAPConfigFromEnv()
	h := newHarness(t)
	identity := h.startExtraServiceWithConfig("identity-ldap", identityService, nil, func(cfg *platform.Config) {
		cfg.LDAPEnabled = true
		cfg.LDAPHost = ldapCfg.host
		cfg.LDAPPort = ldapCfg.port
		cfg.LDAPUseTLS = ldapCfg.useTLS
		cfg.LDAPBindDN = ldapCfg.bindDN
		cfg.LDAPBindPassword = ldapCfg.bindPassword
		cfg.LDAPUserSearchBase = ldapCfg.searchBase
		cfg.LDAPUserFilter = ldapCfg.userFilter
		cfg.LDAPMirrorSyncInterval = 5 * time.Minute
	})

	preseeded := envDefault("TEST_LDAP_USER", "ldapalice")
	preseededPassword := envDefault("TEST_LDAP_PASSWORD", "ldappass")
	h.createRecord(identityUsersResource, "admin-"+h.runID, map[string]any{
		"username":      "admin-" + h.runID,
		"password_hash": platform.HashSecret("admin-local-" + h.runID),
		"status":        "online",
		"role":          "admin",
		"role_id":       "RO2600001",
		"system_role":   0,
	})
	h.createRecord(identityUsersResource, "US"+h.runID+"ldap", map[string]any{
		"username":      preseeded,
		"password_hash": platform.HashSecret("local-only-" + h.runID),
		"status":        "offline",
		"role":          "user",
		"role_id":       "RO2600004",
		"system_role":   2,
	})
	login := h.doURLJSON(identity.url, http.MethodPost, "/api/v1/login", map[string]any{
		"username": preseeded,
		"password": preseededPassword,
	}, "", http.StatusOK)
	if token := login.dataMap(t)["token"]; !strings.HasPrefix(fmt.Sprint(token), "access.") {
		t.Fatalf("ldap login token = %#v, want access token", token)
	}

	createdUsername := "ldap" + h.runID
	createdPassword := "created-pass-" + h.runID
	h.doURLJSON(identity.url, http.MethodPost, "/api/v1/register", map[string]any{
		"username":  createdUsername,
		"password":  createdPassword,
		"email":     createdUsername + "@example.org",
		"full_name": "LDAP Created",
	}, "", http.StatusOK)
	requireLDAPEntry(t, ldapCfg, createdUsername, map[string]string{
		"cn":   "LDAP Created",
		"mail": createdUsername + "@example.org",
	})

	created := findE2EUserByUsername(t, h, createdUsername)
	h.updateRecord(identityUsersResource, created.ID, map[string]any{"password_hash": platform.HashSecret("local-only-after-create")})
	h.doURLJSON(identity.url, http.MethodPost, "/api/v1/login", map[string]any{
		"username": createdUsername,
		"password": createdPassword,
	}, "", http.StatusOK)

	h.doURLJSON(identity.url, http.MethodPut, "/api/v1/users/"+created.ID, map[string]any{
		"full_name":   "LDAP Updated",
		"email":       createdUsername + ".updated@example.org",
		"role":        "manager",
		"system_role": 1,
	}, h.apiKey, http.StatusOK)
	requireLDAPEntry(t, ldapCfg, createdUsername, map[string]string{
		"cn":        "LDAP Updated",
		"mail":      createdUsername + ".updated@example.org",
		"gidNumber": "5003",
	})

	mirrorUsername := "mirror" + h.runID
	h.createRecord(identityUsersResource, "US"+h.runID+"mirror", map[string]any{
		"username":      mirrorUsername,
		"email":         mirrorUsername + "@example.org",
		"full_name":     "Mirror User",
		"password_hash": platform.HashSecret("mirror-local"),
		"status":        "offline",
		"role":          "user",
		"role_id":       "RO2600004",
		"system_role":   2,
	})
	identity.app.RunMaintenanceOnce(h.ctx, time.Second)
	requireLDAPEntry(t, ldapCfg, mirrorUsername, map[string]string{"cn": "Mirror User"})

	h.doURLJSON(identity.url, http.MethodDelete, "/api/v1/users/"+created.ID, nil, h.apiKey, http.StatusOK)
	requireLDAPMissing(t, ldapCfg, createdUsername)
}

type liveLDAPConfig struct {
	host         string
	port         int
	useTLS       bool
	bindDN       string
	bindPassword string
	searchBase   string
	userFilter   string
}

func liveLDAPConfigFromEnv() liveLDAPConfig {
	port := 1389
	if _, err := fmt.Sscanf(envDefault("LDAP_PORT", "1389"), "%d", &port); err != nil {
		port = 1389
	}
	return liveLDAPConfig{
		host:         envDefault("LDAP_HOST", "localhost"),
		port:         port,
		useTLS:       strings.EqualFold(envDefault("LDAP_USE_TLS", "false"), "true"),
		bindDN:       envDefault("LDAP_BIND_DN", "cn=admin,dc=example,dc=org"),
		bindPassword: envDefault("LDAP_BIND_PASSWORD", "adminpassword"),
		searchBase:   envDefault("LDAP_USER_SEARCH_BASE", "ou=users,dc=example,dc=org"),
		userFilter:   envDefault("LDAP_USER_FILTER", "(uid=%s)"),
	}
}

func requireLDAPEntry(t *testing.T, cfg liveLDAPConfig, username string, want map[string]string) {
	t.Helper()
	entry := searchLDAPEntry(t, cfg, username)
	if entry == nil {
		t.Fatalf("LDAP user %q missing", username)
	}
	for attr, expected := range want {
		if got := entry.GetAttributeValue(attr); got != expected {
			t.Fatalf("LDAP user %q attr %s = %q, want %q", username, attr, got, expected)
		}
	}
}

func requireLDAPMissing(t *testing.T, cfg liveLDAPConfig, username string) {
	t.Helper()
	if entry := searchLDAPEntry(t, cfg, username); entry != nil {
		t.Fatalf("LDAP user %q still exists: dn=%s", username, entry.DN)
	}
}

func searchLDAPEntry(t *testing.T, cfg liveLDAPConfig, username string) *ldap.Entry {
	t.Helper()
	conn := liveLDAPConn(t, cfg)
	defer func() { _ = conn.Close() }()
	filter := fmt.Sprintf(cfg.userFilter, ldap.EscapeFilter(username))
	search := ldap.NewSearchRequest(cfg.searchBase, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 2, 2, false, filter, []string{"cn", "mail", "gidNumber", "uid"}, nil)
	result, err := conn.Search(search)
	if err != nil {
		t.Fatalf("search LDAP user %q: %v", username, err)
	}
	if len(result.Entries) == 0 {
		return nil
	}
	return result.Entries[0]
}

func liveLDAPConn(t *testing.T, cfg liveLDAPConfig) *ldap.Conn {
	t.Helper()
	scheme := "ldap"
	if cfg.useTLS {
		scheme = "ldaps"
	}
	opts := []ldap.DialOpt{ldap.DialWithDialer(&net.Dialer{Timeout: 2 * time.Second})}
	if cfg.useTLS {
		opts = append(opts, ldap.DialWithTLSConfig(&tls.Config{MinVersion: tls.VersionTLS12}))
	}
	conn, err := ldap.DialURL(fmt.Sprintf("%s://%s:%d", scheme, cfg.host, cfg.port), opts...)
	if err != nil {
		t.Fatalf("dial LDAP: %v", err)
	}
	conn.SetTimeout(2 * time.Second)
	if err := conn.Bind(cfg.bindDN, cfg.bindPassword); err != nil {
		_ = conn.Close()
		t.Fatalf("bind LDAP admin: %v", err)
	}
	return conn
}

func findE2EUserByUsername(t *testing.T, h *e2eHarness, username string) contracts.Record[map[string]any] {
	t.Helper()
	for _, record := range h.listRecords(identityUsersResource) {
		if strings.EqualFold(fmt.Sprint(record.Data["username"]), username) {
			return record
		}
	}
	t.Fatalf("missing identity user %q", username)
	return contracts.Record[map[string]any]{}
}
