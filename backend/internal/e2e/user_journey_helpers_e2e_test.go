//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func (h *e2eHarness) seedAPIUser(userID, username string, admin bool) string {
	h.t.Helper()
	tokenID := "token-" + userID
	token := "raw-" + userID
	role := "user"
	systemRole := 2
	if admin {
		role = "admin"
		systemRole = 0
	}
	h.createRecord(identityUsersResource, userID, map[string]any{
		"username":    username,
		"role":        role,
		"system_role": systemRole,
		"status":      "online",
	})
	h.createRecord(identityAPITokensResource, tokenID, map[string]any{
		"user_id":    userID,
		"token_hash": platform.HashSecret(token),
		"expires_at": time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
		"revoked":    false,
	})
	h.installStaticUserAPIKey(token, userID, username, role, admin)
	return token
}

func (h *e2eHarness) installStaticUserAPIKey(token, userID, username, role string, admin bool) {
	h.t.Helper()
	for _, service := range h.services {
		if service == nil || service.app == nil {
			continue
		}
		if service.app.Config.APIKeys == nil {
			service.app.Config.APIKeys = map[string]bool{}
		}
		if service.app.Config.APIKeyPrincipals == nil {
			service.app.Config.APIKeyPrincipals = map[string]platform.APIKeyPrincipal{}
		}
		service.app.Config.APIKeys[token] = true
		service.app.Config.APIKeyPrincipals[token] = platform.APIKeyPrincipal{
			ID:       userID,
			UserID:   userID,
			Username: username,
			Role:     role,
			Admin:    admin,
		}
	}
}

func (h *e2eHarness) doJSONWithBearer(serviceName, method, path string, payload any, bearerToken string, want int) testResponse {
	h.t.Helper()
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			h.t.Fatalf("marshal bearer request: %v", err)
		}
		body = bytes.NewReader(raw)
	}
	req := h.newRequest(serviceName, method, path, body, "")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("X-API-Key", bearerToken)
	return h.do(req, want)
}

func (h *e2eHarness) doWithBearer(serviceName, method, path, bearerToken string, want int) testResponse {
	h.t.Helper()
	req := h.newRequest(serviceName, method, path, nil, "")
	req.Header.Set("X-API-Key", bearerToken)
	return h.do(req, want)
}

func e2eDataRecords(t *testing.T, response testResponse) []map[string]any {
	t.Helper()
	env := response.envelope(t)
	items, ok := env["data"].([]any)
	if !ok {
		t.Fatalf("data = %#v, want array", env["data"])
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		row, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("data item = %#v, want object", item)
		}
		out = append(out, row)
	}
	return out
}

func e2eRecordData(t *testing.T, record map[string]any) map[string]any {
	t.Helper()
	data, ok := record["data"].(map[string]any)
	if !ok {
		t.Fatalf("record data = %#v, want object", record["data"])
	}
	return data
}

func e2eResponseRecordData(t *testing.T, response testResponse) map[string]any {
	t.Helper()
	return e2eRecordData(t, response.dataMap(t))
}

func e2eRecordsContainID(records []map[string]any, id string) bool {
	for _, record := range records {
		if textE2E(record["id"]) == id {
			return true
		}
	}
	return false
}

func e2eRecordsContainDataID(records []map[string]any, id string) bool {
	for _, record := range records {
		if textE2E(record["id"]) == id {
			return true
		}
		if data, ok := record["data"].(map[string]any); ok && textE2E(data["id"]) == id {
			return true
		}
	}
	return false
}

func e2eRecordsContainDataValue(records []map[string]any, key, value string) bool {
	for _, record := range records {
		if data, ok := record["data"].(map[string]any); ok && textE2E(data[key]) == value {
			return true
		}
		if textE2E(record[key]) == value {
			return true
		}
	}
	return false
}

func e2eRequireNoRecords(t *testing.T, records []map[string]any, context string) {
	t.Helper()
	if len(records) != 0 {
		t.Fatalf("%s = %#v, want empty filtered list", context, records)
	}
}

func e2eSuffix(runID string) string {
	return truncateID(sanitizeID(runID), 24)
}

func e2eLowerPodName(userID, ideType string) string {
	return "ide-" + strings.ToLower(userID) + "-" + strings.ToLower(ideType)
}
