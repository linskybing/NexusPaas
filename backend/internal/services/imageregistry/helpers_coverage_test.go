package imageregistry

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestImageRegistryHarborAdapterBranches(t *testing.T) {
	app := newImageRegistryTestApp(t)
	app.Adapters["harbor"] = fakeImageHarborAdapter{result: contracts.AdapterResult{Adapter: "harbor", Operation: "customStatus"}}

	code, data, degraded := getHarborStatus(app, imageRequest(http.MethodGet, "/api/v1/harbor-status", "", "ADMIN"), platform.RouteSpec{OperationID: "customStatus"})
	assertImageStatus(t, code, data, http.StatusOK)
	if degraded != nil {
		t.Fatalf("degraded = %#v, want nil for successful adapter", degraded)
	}
	if data.(map[string]any)["status"] != "ok" {
		t.Fatalf("harbor status = %#v, want ok map", data)
	}

	app.Adapters["harbor"] = fakeImageHarborAdapter{err: errors.New("harbor timeout")}
	result, degraded := callHarbor(app, imageRequest(http.MethodGet, "/api/v1/harbor-status", "", "ADMIN"), platform.RouteSpec{Method: http.MethodGet}, "fallback")
	if degraded == nil || degraded.Code != "adapter_unavailable" || !result.Degraded {
		t.Fatalf("callHarbor error result=%#v degraded=%#v, want unavailable degraded result", result, degraded)
	}
	if app.Metrics.Counter("harbor_degraded") != 1 {
		t.Fatalf("harbor_degraded counter = %d, want 1", app.Metrics.Counter("harbor_degraded"))
	}
}

func TestImageRegistryProjectMemberRoleAndReferenceHelpers(t *testing.T) {
	app := newImageRegistryTestApp(t)
	req := imageRequest(http.MethodGet, "/api/v1/projects/P1/images", "", "U3")
	createImageRecords(t, app, orgProjectMembersResource, []map[string]any{
		{"id": "P1:U3", "project_id": "P1", "user_id": "U3", "role": "manager"},
	})
	project, found := findProject(app, req, "P1")
	if !found {
		t.Fatal("missing project P1")
	}
	if role := projectRole(app, req, project, "U3"); role != "manager" {
		t.Fatalf("projectRole via project member = %q, want manager", role)
	}

	ref := imageReference(map[string]any{"repository": "team/app"})
	if ref != "docker.io/team/app:latest" {
		t.Fatalf("imageReference from parts = %q, want docker.io/team/app:latest", ref)
	}
	if got := canonicalImageRef("team/app"); got != "docker.io/team/app:latest" {
		t.Fatalf("canonicalImageRef = %q, want docker.io/team/app:latest", got)
	}
	if got := registryFromReference("registry.local/team/app:v1"); got != "registry.local" {
		t.Fatalf("registryFromReference = %q, want registry.local", got)
	}
	if got := repositoryFromReference("registry.local/team/app:v1"); got != "team/app" {
		t.Fatalf("repositoryFromReference = %q, want team/app", got)
	}
	if got := tagFromReference("registry.local/team/app"); got != defaultTag {
		t.Fatalf("tagFromReference missing tag = %q, want %s", got, defaultTag)
	}
}

func TestImageRegistryProjectionMergeAndReadModelIDs(t *testing.T) {
	local := []map[string]any{{"project_id": "P1", "user_id": "U1", "role": "manager"}}
	source := []map[string]any{
		{"project_id": "P1", "user_id": "U1", "role": "user"},
		{"project_id": "P1", "user_id": "U2", "role": "user"},
		{"role": "orphan"},
	}
	merged := mergeRows(imageProjectMembersResource, source, local)
	if len(merged) != 3 || merged[0]["role"] != "manager" || merged[1]["user_id"] != "U2" || merged[2]["role"] != "orphan" {
		t.Fatalf("mergeRows = %#v, want local override plus new/orphan source", merged)
	}

	for _, tc := range []struct {
		resource string
		data     map[string]any
		want     string
	}{
		{resource: imageProjectsResource, data: map[string]any{"p_id": "P1"}, want: "P1"},
		{resource: imageProjectMembersResource, data: map[string]any{"projectId": "P1", "userId": "U1"}, want: "P1:U1"},
		{resource: imageUserGroupsResource, data: map[string]any{"uid": "U1", "gid": "G1"}, want: "U1:G1"},
		{resource: imageIdentityUsersResource, data: map[string]any{"name": "alice"}, want: "alice"},
		{resource: imageIdentityRolesResource, data: map[string]any{"role_id": "R1"}, want: "R1"},
	} {
		if got := imageReadModelID(tc.resource, tc.data); got != tc.want {
			t.Fatalf("imageReadModelID(%s) = %q, want %q", tc.resource, got, tc.want)
		}
	}

	resource, data, deleted, ok := imageProjection(contracts.Event{Name: "GroupMembershipChanged", Data: map[string]any{"user_id": "U1", "group_id": "G1", "action": "deleted"}})
	if !ok || !deleted || resource != imageUserGroupsResource || data["user_id"] != "U1" {
		t.Fatalf("group membership projection resource=%s data=%#v deleted=%v ok=%v", resource, data, deleted, ok)
	}
}

type fakeImageHarborAdapter struct {
	result contracts.AdapterResult
	err    error
}

func (f fakeImageHarborAdapter) Call(context.Context, string, bool) (contracts.AdapterResult, error) {
	return f.result, f.err
}
