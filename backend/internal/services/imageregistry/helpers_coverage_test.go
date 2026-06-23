package imageregistry

import (
	"context"
	"errors"
	"net/http"
	"reflect"
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

func TestImageRegistryProjectionDriftDetectsMissingOrphanStaleCleanAndSorts(t *testing.T) {
	ctx := context.Background()
	app := platform.NewApp(platform.Config{ServiceName: "all"})

	seedImageProjectionDriftRecord(t, ctx, app, orgProjectsResource, map[string]any{"id": "project-orphan-check", "project_name": "source-only"})
	seedImageProjectionDriftRecord(t, ctx, app, imageProjectsResource, map[string]any{"id": "project-orphan", "project_name": "local-only"})
	seedImageProjectionDriftRecord(t, ctx, app, orgProjectsResource, map[string]any{"id": "project-stale", "project_name": "source"})
	seedImageProjectionDriftRecord(t, ctx, app, imageProjectsResource, map[string]any{"id": "project-stale", "project_name": "local"})
	seedImageProjectionDriftRecord(t, ctx, app, orgProjectsResource, map[string]any{"id": "project-clean", "project_name": "clean"})
	seedImageProjectionDriftRecord(t, ctx, app, imageProjectsResource, map[string]any{"id": "project-clean", "project_name": "clean"})

	seedImageProjectionDriftRecord(t, ctx, app, identityRolesResource, map[string]any{"id": "role-clean", "role_id": "role-clean", "name": "viewer"})
	seedImageProjectionDriftRecord(t, ctx, app, imageIdentityRolesResource, map[string]any{"id": "role-clean", "role_id": "role-clean", "name": "viewer"})
	seedImageProjectionDriftRecord(t, ctx, app, imageIdentityRolesResource, map[string]any{"id": "role-orphan", "role_id": "role-orphan", "name": "orphan"})

	seedImageProjectionDriftRecord(t, ctx, app, orgProjectMembersResource, map[string]any{"project_id": "project-missing", "user_id": "user-missing", "role": "user"})
	seedImageProjectionDriftRecord(t, ctx, app, orgUserGroupsResource, map[string]any{"user_id": "user-stale", "group_id": "group-stale", "role": "source"})
	seedImageProjectionDriftRecord(t, ctx, app, imageUserGroupsResource, map[string]any{"id": "user-stale:group-stale", "user_id": "user-stale", "group_id": "group-stale", "role": "local"})

	seedImageProjectionDriftRecord(t, ctx, app, identityUsersResource, map[string]any{"id": "user-missing", "user_id": "user-missing", "username": "missing"})

	report, err := imageProjectionDrift(ctx, app)
	if err != nil {
		t.Fatalf("imageProjectionDrift returned error: %v", err)
	}

	wantMissing := []imageProjectionDriftFinding{
		{SourceResource: identityUsersResource, LocalResource: imageIdentityUsersResource, ID: "user-missing"},
		{SourceResource: orgProjectMembersResource, LocalResource: imageProjectMembersResource, ID: "project-missing:user-missing"},
		{SourceResource: orgProjectsResource, LocalResource: imageProjectsResource, ID: "project-orphan-check"},
	}
	if !reflect.DeepEqual(report.Missing, wantMissing) {
		t.Fatalf("missing findings = %#v, want %#v", report.Missing, wantMissing)
	}

	wantOrphan := []imageProjectionDriftFinding{
		{SourceResource: identityRolesResource, LocalResource: imageIdentityRolesResource, ID: "role-orphan"},
		{SourceResource: orgProjectsResource, LocalResource: imageProjectsResource, ID: "project-orphan"},
	}
	if !reflect.DeepEqual(report.Orphan, wantOrphan) {
		t.Fatalf("orphan findings = %#v, want %#v", report.Orphan, wantOrphan)
	}

	wantStale := []imageProjectionDriftFinding{
		{SourceResource: orgProjectsResource, LocalResource: imageProjectsResource, ID: "project-stale"},
		{SourceResource: orgUserGroupsResource, LocalResource: imageUserGroupsResource, ID: "user-stale:group-stale"},
	}
	if !reflect.DeepEqual(report.Stale, wantStale) {
		t.Fatalf("stale findings = %#v, want %#v", report.Stale, wantStale)
	}
}

func TestImageRegistryProjectionDriftNormalizesCanonicalID(t *testing.T) {
	ctx := context.Background()
	app := platform.NewApp(platform.Config{})

	sourceProjectRecordID := seedImageProjectionDriftRecord(t, ctx, app, orgProjectsResource, map[string]any{
		"id":           "source-record-project",
		"project_id":   "project-normalized",
		"project_name": "Normalized",
	})
	updateImageProjectionDriftRecord(t, ctx, app, orgProjectsResource, sourceProjectRecordID, map[string]any{
		"id":           "",
		"project_id":   "project-normalized",
		"project_name": "Normalized",
	})
	seedImageProjectionDriftRecord(t, ctx, app, imageProjectsResource, map[string]any{
		"id":           "project-normalized",
		"project_id":   "project-normalized",
		"project_name": "Normalized",
	})

	sourceMemberRecordID := seedImageProjectionDriftRecord(t, ctx, app, orgProjectMembersResource, map[string]any{
		"id":         "source-record-member",
		"project_id": "project-normalized",
		"user_id":    "user-normalized",
		"role":       "manager",
	})
	updateImageProjectionDriftRecord(t, ctx, app, orgProjectMembersResource, sourceMemberRecordID, map[string]any{
		"id":         "",
		"project_id": "project-normalized",
		"user_id":    "user-normalized",
		"role":       "manager",
	})
	seedImageProjectionDriftRecord(t, ctx, app, imageProjectMembersResource, map[string]any{
		"id":         "project-normalized:user-normalized",
		"project_id": "project-normalized",
		"user_id":    "user-normalized",
		"role":       "manager",
	})

	report, err := imageProjectionDrift(ctx, app)
	if err != nil {
		t.Fatalf("imageProjectionDrift returned error: %v", err)
	}
	if len(report.Missing) != 0 || len(report.Orphan) != 0 || len(report.Stale) != 0 {
		t.Fatalf("imageProjectionDrift with canonical ids = %#v, want no findings", report)
	}
}

func TestImageRegistryProjectionDriftSkipsBlankCanonicalIDs(t *testing.T) {
	ctx := context.Background()
	app := platform.NewApp(platform.Config{})

	sourceRecordID := seedImageProjectionDriftRecord(t, ctx, app, identityRolesResource, map[string]any{"id": "blank-source-record", "description": "ignored"})
	updateImageProjectionDriftRecord(t, ctx, app, identityRolesResource, sourceRecordID, map[string]any{"id": "", "description": "ignored"})
	localRecordID := seedImageProjectionDriftRecord(t, ctx, app, imageIdentityRolesResource, map[string]any{"id": "blank-local-record", "description": "ignored"})
	updateImageProjectionDriftRecord(t, ctx, app, imageIdentityRolesResource, localRecordID, map[string]any{"id": "", "description": "ignored"})

	report, err := imageProjectionDrift(ctx, app)
	if err != nil {
		t.Fatalf("imageProjectionDrift returned error: %v", err)
	}
	if len(report.Missing) != 0 || len(report.Orphan) != 0 || len(report.Stale) != 0 {
		t.Fatalf("imageProjectionDrift with blank canonical ids = %#v, want no findings", report)
	}
}

func TestImageRegistryProjectionDriftNilAppOrStoreFailsClosed(t *testing.T) {
	ctx := context.Background()
	for _, app := range []*platform.App{nil, {}} {
		if _, err := imageProjectionDrift(ctx, app); !errors.Is(err, errImageProjectionDriftUnavailable) {
			t.Fatalf("imageProjectionDrift(%#v) error = %v, want %v", app, err, errImageProjectionDriftUnavailable)
		}
	}
}

func TestImageRegistryProjectionDriftPairsCoverExpectedResources(t *testing.T) {
	want := map[string]string{
		identityUsersResource:     imageIdentityUsersResource,
		identityRolesResource:     imageIdentityRolesResource,
		orgProjectsResource:       imageProjectsResource,
		orgProjectMembersResource: imageProjectMembersResource,
		orgUserGroupsResource:     imageUserGroupsResource,
	}
	if len(imageProjectionDriftPairs) != len(want) {
		t.Fatalf("imageProjectionDriftPairs length = %d, want %d", len(imageProjectionDriftPairs), len(want))
	}

	excluded := map[string]bool{
		projectImagesResource: true,
		imageRequestsResource: true,
		imageCatalogResource:  true,
		imageBuildsResource:   true,
		imageSyncResource:     true,
	}
	got := map[string]string{}
	for _, pair := range imageProjectionDriftPairs {
		if pair.idFn == nil {
			t.Fatalf("imageProjectionDriftPairs contains nil idFn for %s -> %s", pair.sourceResource, pair.localResource)
		}
		if excluded[pair.sourceResource] || excluded[pair.localResource] {
			t.Fatalf("imageProjectionDriftPairs includes out-of-scope resource: %#v", pair)
		}
		got[pair.sourceResource] = pair.localResource
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("imageProjectionDriftPairs = %#v, want %#v", got, want)
	}
}

func seedImageProjectionDriftRecord(t *testing.T, ctx context.Context, app *platform.App, resource string, data map[string]any) string {
	t.Helper()
	record, err := app.Store.Create(ctx, resource, data)
	if err != nil {
		t.Fatalf("create %s record: %v", resource, err)
	}
	return record.ID
}

func updateImageProjectionDriftRecord(t *testing.T, ctx context.Context, app *platform.App, resource, id string, data map[string]any) {
	t.Helper()
	if _, ok := app.Store.Update(ctx, resource, id, data); !ok {
		t.Fatalf("update %s/%s missed", resource, id)
	}
}

type fakeImageHarborAdapter struct {
	result contracts.AdapterResult
	err    error
}

func (f fakeImageHarborAdapter) Call(context.Context, string, bool) (contracts.AdapterResult, error) {
	return f.result, f.err
}
