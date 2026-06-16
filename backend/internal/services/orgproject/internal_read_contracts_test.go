package orgproject

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestInternalReadContractsRequireServiceKeyAndReturnOwnerRecords(t *testing.T) {
	app, cases := newOrgProjectReadContractFixture(t)

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertOrgProjectReadContractCase(t, app, tc)
		})
	}
}

type orgProjectReadContractCase struct {
	name     string
	listPath string
	getPath  string
	wantID   string
}

func newOrgProjectReadContractFixture(t *testing.T) (*platform.App, []orgProjectReadContractCase) {
	t.Helper()
	app := platform.NewApp(platform.Config{ServiceName: "all", HTTPAddr: ":0", ServiceAPIKey: "svc-key"})
	Register(app)
	ids := orgProjectReadContractIDs{
		projectID: "proj-read",
		userID:    "user/read",
		groupID:   "group-read",
	}
	seedOrgProjectReadContractRecords(t, app, ids)
	return app, orgProjectReadContractCases(ids)
}

type orgProjectReadContractIDs struct {
	projectID string
	userID    string
	groupID   string
}

func seedOrgProjectReadContractRecords(t *testing.T, app *platform.App, ids orgProjectReadContractIDs) {
	t.Helper()
	ctx := context.Background()
	for _, seed := range []struct {
		resource string
		id       string
		data     map[string]any
	}{
		{
			resource: projectsResource,
			id:       ids.projectID,
			data:     map[string]any{"project_name": "read contract", "plan_id": "plan-1"},
		},
		{
			resource: projectMembersResource,
			id:       ids.projectID + "/" + ids.userID,
			data:     map[string]any{"project_id": ids.projectID, "user_id": ids.userID, "role": "user"},
		},
		{
			resource: projectUserQuotasResource,
			id:       ids.projectID + "/" + ids.userID,
			data:     map[string]any{"project_id": ids.projectID, "user_id": ids.userID, "gpu_limit": 2},
		},
		{
			resource: userGroupsResource,
			id:       ids.userID + "/" + ids.groupID,
			data:     map[string]any{"group_id": ids.groupID, "user_id": ids.userID, "role": "user"},
		},
	} {
		data := seed.data
		data["id"] = seed.id
		if _, err := app.Store.Create(ctx, seed.resource, data); err != nil {
			t.Fatalf("seed %s/%s: %v", seed.resource, seed.id, err)
		}
	}
}

func orgProjectReadContractCases(ids orgProjectReadContractIDs) []orgProjectReadContractCase {
	projectUserID := ids.projectID + "/" + ids.userID
	userGroupID := ids.userID + "/" + ids.groupID
	return []orgProjectReadContractCase{
		{name: "projects", listPath: "/internal/org-project/projects", getPath: "/internal/org-project/projects/" + ids.projectID, wantID: ids.projectID},
		{name: "project members", listPath: "/internal/org-project/project-members", getPath: "/internal/org-project/project-members/" + projectUserID, wantID: projectUserID},
		{name: "user quotas", listPath: "/internal/org-project/user-quotas", getPath: "/internal/org-project/user-quotas/" + projectUserID, wantID: projectUserID},
		{name: "user groups", listPath: "/internal/org-project/user-groups", getPath: "/internal/org-project/user-groups/" + userGroupID, wantID: userGroupID},
	}
}

func assertOrgProjectReadContractCase(t *testing.T, app *platform.App, tc orgProjectReadContractCase) {
	t.Helper()
	if rec := serveOrgProjectReadContract(app, tc.listPath, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("list without service key status = %d, want 401", rec.Code)
	}
	list := serveOrgProjectReadContract(app, tc.listPath, "svc-key")
	if list.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200: %s", list.Code, list.Body.String())
	}
	if !orgProjectReadContractListContains(t, list.Body.Bytes(), tc.wantID) {
		t.Fatalf("list body %s does not include %s", list.Body.String(), tc.wantID)
	}

	if rec := serveOrgProjectReadContract(app, tc.getPath, "wrong"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("get with wrong service key status = %d, want 401", rec.Code)
	}
	get := serveOrgProjectReadContract(app, tc.getPath, "svc-key")
	if get.Code != http.StatusOK {
		t.Fatalf("get status = %d, want 200: %s", get.Code, get.Body.String())
	}
	if got := orgProjectReadContractRecordID(t, get.Body.Bytes()); got != tc.wantID {
		t.Fatalf("get id = %q, want %q", got, tc.wantID)
	}
}

func serveOrgProjectReadContract(app *platform.App, path, serviceKey string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if serviceKey != "" {
		req.Header.Set("X-Service-Key", serviceKey)
	}
	app.ServeHTTP(rec, req)
	return rec
}

func orgProjectReadContractListContains(t *testing.T, raw []byte, id string) bool {
	t.Helper()
	var env struct {
		Data []contracts.Record[map[string]any] `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("decode list envelope: %v", err)
	}
	for _, record := range env.Data {
		if record.ID == id || strings.TrimSpace(record.Data["id"].(string)) == id {
			return true
		}
	}
	return false
}

func orgProjectReadContractRecordID(t *testing.T, raw []byte) string {
	t.Helper()
	var env struct {
		Data contracts.Record[map[string]any] `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("decode get envelope: %v", err)
	}
	if env.Data.ID != "" {
		return env.Data.ID
	}
	return strings.TrimSpace(env.Data.Data["id"].(string))
}
