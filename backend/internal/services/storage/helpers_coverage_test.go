package storage

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestStorageProjectionDriftDetectsMissingOrphanStaleAndSorts(t *testing.T) {
	ctx := context.Background()
	app := platform.NewApp(platform.Config{ServiceName: "all"})

	seedStorageProjectionRecord(t, app, identityUsersResource, map[string]any{"id": "user-missing"})
	seedStorageProjectionRecord(t, app, storageIdentityUsersResource, map[string]any{"id": "user-orphan", "username": "orphan"})
	seedStorageProjectionRecord(t, app, identityUsersResource, map[string]any{"id": "user-stale", "role": "source"})
	seedStorageProjectionRecord(t, app, storageIdentityUsersResource, map[string]any{"id": "user-stale", "role": "local"})
	seedStorageProjectionRecord(t, app, identityUsersResource, map[string]any{"id": "user-clean", "display": "equal"})
	seedStorageProjectionRecord(t, app, storageIdentityUsersResource, map[string]any{"id": "user-clean", "display": "equal"})

	seedStorageProjectionRecord(t, app, identityRolesResource, map[string]any{"id": "role-missing"})
	seedStorageProjectionRecord(t, app, storageIdentityRolesResource, map[string]any{"id": "role-orphan"})
	seedStorageProjectionRecord(t, app, identityRolesResource, map[string]any{"id": "role-stale", "name": "source"})
	seedStorageProjectionRecord(t, app, storageIdentityRolesResource, map[string]any{"id": "role-stale", "name": "local"})
	seedStorageProjectionRecord(t, app, identityRolesResource, map[string]any{"id": "role-clean", "name": "equal"})
	seedStorageProjectionRecord(t, app, storageIdentityRolesResource, map[string]any{"id": "role-clean", "name": "equal"})

	seedStorageProjectionRecord(t, app, orgProjectsResource, map[string]any{"id": "project-missing", "name": "missing"})
	seedStorageProjectionRecord(t, app, storageProjectsResource, map[string]any{"id": "project-orphan", "name": "orphan"})
	seedStorageProjectionRecord(t, app, orgProjectsResource, map[string]any{"id": "project-stale", "name": "source"})
	seedStorageProjectionRecord(t, app, storageProjectsResource, map[string]any{"id": "project-stale", "name": "local"})
	seedStorageProjectionRecord(t, app, orgProjectsResource, map[string]any{"id": "project-clean", "name": "equal"})
	seedStorageProjectionRecord(t, app, storageProjectsResource, map[string]any{"id": "project-clean", "name": "equal"})

	seedStorageProjectionRecord(t, app, orgProjectMembersResource, map[string]any{"project_id": "project-member-missing", "user_id": "user-missing", "role": "source"})
	seedStorageProjectionRecord(t, app, storageProjectMembersResource, map[string]any{"project_id": "project-member-orphan", "user_id": "user-orphan", "role": "local"})
	seedStorageProjectionRecord(t, app, orgProjectMembersResource, map[string]any{"project_id": "project-member-stale", "user_id": "user-stale", "role": "source"})
	seedStorageProjectionRecord(t, app, storageProjectMembersResource, map[string]any{"project_id": "project-member-stale", "user_id": "user-stale", "role": "local"})
	seedStorageProjectionRecord(t, app, orgProjectMembersResource, map[string]any{"project_id": "project-member-clean", "user_id": "user-clean", "role": "member"})
	seedStorageProjectionRecord(t, app, storageProjectMembersResource, map[string]any{"project_id": "project-member-clean", "user_id": "user-clean", "role": "member"})

	seedStorageProjectionRecord(t, app, orgUserGroupsResource, map[string]any{"user_id": "user-group-missing", "group_id": "group-missing", "role": "source"})
	seedStorageProjectionRecord(t, app, storageUserGroupsResource, map[string]any{"user_id": "user-group-orphan", "group_id": "group-orphan", "role": "local"})
	seedStorageProjectionRecord(t, app, orgUserGroupsResource, map[string]any{"user_id": "user-group-stale", "group_id": "group-stale", "role": "source"})
	seedStorageProjectionRecord(t, app, storageUserGroupsResource, map[string]any{"user_id": "user-group-stale", "group_id": "group-stale", "role": "local"})
	seedStorageProjectionRecord(t, app, orgUserGroupsResource, map[string]any{"user_id": "user-group-clean", "group_id": "group-clean", "role": "member"})
	seedStorageProjectionRecord(t, app, storageUserGroupsResource, map[string]any{"user_id": "user-group-clean", "group_id": "group-clean", "role": "member"})

	report, err := storageProjectionDrift(ctx, app)
	if err != nil {
		t.Fatalf("storageProjectionDrift returned error: %v", err)
	}
	if got := len(report.Missing); got != 5 {
		t.Fatalf("missing count = %d, want 5", got)
	}
	if got := len(report.Orphan); got != 5 {
		t.Fatalf("orphan count = %d, want 5", got)
	}
	if got := len(report.Stale); got != 5 {
		t.Fatalf("stale count = %d, want 5", got)
	}

	wantMissing := []storageProjectionDriftFinding{
		{SourceResource: identityRolesResource, LocalResource: storageIdentityRolesResource, ID: "role-missing"},
		{SourceResource: identityUsersResource, LocalResource: storageIdentityUsersResource, ID: "user-missing"},
		{SourceResource: orgProjectMembersResource, LocalResource: storageProjectMembersResource, ID: "project-member-missing:user-missing"},
		{SourceResource: orgProjectsResource, LocalResource: storageProjectsResource, ID: "project-missing"},
		{SourceResource: orgUserGroupsResource, LocalResource: storageUserGroupsResource, ID: "user-group-missing:group-missing"},
	}
	if !reflect.DeepEqual(report.Missing, wantMissing) {
		t.Fatalf("storageProjectionDrift missing = %#v, want %#v", report.Missing, wantMissing)
	}

	wantOrphan := []storageProjectionDriftFinding{
		{SourceResource: identityRolesResource, LocalResource: storageIdentityRolesResource, ID: "role-orphan"},
		{SourceResource: identityUsersResource, LocalResource: storageIdentityUsersResource, ID: "user-orphan"},
		{SourceResource: orgProjectMembersResource, LocalResource: storageProjectMembersResource, ID: "project-member-orphan:user-orphan"},
		{SourceResource: orgProjectsResource, LocalResource: storageProjectsResource, ID: "project-orphan"},
		{SourceResource: orgUserGroupsResource, LocalResource: storageUserGroupsResource, ID: "user-group-orphan:group-orphan"},
	}
	if !reflect.DeepEqual(report.Orphan, wantOrphan) {
		t.Fatalf("storageProjectionDrift orphan = %#v, want %#v", report.Orphan, wantOrphan)
	}

	wantStale := []storageProjectionDriftFinding{
		{SourceResource: identityRolesResource, LocalResource: storageIdentityRolesResource, ID: "role-stale"},
		{SourceResource: identityUsersResource, LocalResource: storageIdentityUsersResource, ID: "user-stale"},
		{SourceResource: orgProjectMembersResource, LocalResource: storageProjectMembersResource, ID: "project-member-stale:user-stale"},
		{SourceResource: orgProjectsResource, LocalResource: storageProjectsResource, ID: "project-stale"},
		{SourceResource: orgUserGroupsResource, LocalResource: storageUserGroupsResource, ID: "user-group-stale:group-stale"},
	}
	if !reflect.DeepEqual(report.Stale, wantStale) {
		t.Fatalf("storageProjectionDrift stale = %#v, want %#v", report.Stale, wantStale)
	}
}

func TestStorageProjectionDriftSkipsBlankCanonicalIDs(t *testing.T) {
	ctx := context.Background()
	app := platform.NewApp(platform.Config{})

	seedStorageProjectionRecordWithoutCanonicalID(t, app, identityUsersResource, map[string]any{"id": "", "description": "ignore"})
	seedStorageProjectionRecordWithoutCanonicalID(t, app, storageIdentityUsersResource, map[string]any{"id": "", "description": "ignore"})
	seedStorageProjectionRecordWithoutCanonicalID(t, app, orgProjectMembersResource, map[string]any{"project_id": "", "user_id": "", "description": "ignore"})
	seedStorageProjectionRecordWithoutCanonicalID(t, app, storageProjectMembersResource, map[string]any{"project_id": "", "user_id": "", "description": "ignore"})
	seedStorageProjectionRecordWithoutCanonicalID(t, app, orgUserGroupsResource, map[string]any{"user_id": "", "group_id": "", "description": "ignore"})
	seedStorageProjectionRecordWithoutCanonicalID(t, app, storageUserGroupsResource, map[string]any{"user_id": "", "group_id": "", "description": "ignore"})
	seedStorageProjectionRecordWithoutCanonicalID(t, app, orgProjectsResource, map[string]any{"id": "", "project_id": "", "description": "ignore"})
	seedStorageProjectionRecordWithoutCanonicalID(t, app, storageProjectsResource, map[string]any{"id": "", "project_id": "", "description": "ignore"})
	seedStorageProjectionRecordWithoutCanonicalID(t, app, identityRolesResource, map[string]any{"id": "", "role_id": "", "name": ""})
	seedStorageProjectionRecordWithoutCanonicalID(t, app, storageIdentityRolesResource, map[string]any{"id": "", "role_id": "", "name": ""})

	report, err := storageProjectionDrift(ctx, app)
	if err != nil {
		t.Fatalf("storageProjectionDrift returned error: %v", err)
	}
	if len(report.Missing) != 0 || len(report.Orphan) != 0 || len(report.Stale) != 0 {
		t.Fatalf("storageProjectionDrift with blank canonical ids = %#v, want no findings", report)
	}
}

func TestStorageProjectionDriftPairCoverageAndNonAccessExclusion(t *testing.T) {
	want := map[string]string{
		storageIdentityUsersResource:  identityUsersResource,
		storageIdentityRolesResource:  identityRolesResource,
		storageProjectsResource:       orgProjectsResource,
		storageProjectMembersResource: orgProjectMembersResource,
		storageUserGroupsResource:     orgUserGroupsResource,
	}
	excluded := map[string]bool{
		groupStorageResource:         true,
		storagePermissionsResource:   true,
		storagePoliciesResource:      true,
		projectBindingsResource:      true,
		projectPermissionsResource:   true,
		userStorageResource:          true,
		fastTransfersResource:        true,
		serviceName + ":mount_plans": true,
		longhornRWXHealthResource:    true,
	}
	got := map[string]string{}
	for _, pair := range storageProjectionDriftPairs {
		if pair.idFn == nil {
			t.Fatalf("projection drift pair %s -> %s has nil id function", pair.sourceResource, pair.localResource)
		}
		if excluded[pair.sourceResource] || excluded[pair.localResource] {
			t.Fatalf("projection drift pair should not include non-access resource: %s -> %s", pair.sourceResource, pair.localResource)
		}
		got[pair.localResource] = pair.sourceResource
	}
	if len(got) != len(want) || !reflect.DeepEqual(got, want) {
		t.Fatalf("projection drift pairs = %#v, want %#v", got, want)
	}
	for excludedResource := range excluded {
		if _, found := got[excludedResource]; found {
			t.Fatalf("projection drift pairs should not include excluded resource: %s", excludedResource)
		}
	}
}

func TestStorageProjectionDriftCanonicalIDNormalization(t *testing.T) {
	ctx := context.Background()
	app := platform.NewApp(platform.Config{ServiceName: "all"})

	seedStorageProjectionRecordWithoutCanonicalID(t, app, orgProjectsResource, map[string]any{"id": "", "project_id": "project-normalized", "project_name": "source"})
	seedStorageProjectionRecord(t, app, storageProjectsResource, map[string]any{"id": "project-normalized", "project_id": "project-normalized", "project_name": "source"})

	seedStorageProjectionRecordWithoutCanonicalID(t, app, orgProjectMembersResource, map[string]any{"id": "", "project_id": "project-normalized", "user_id": "user-normalized", "role": "source"})
	seedStorageProjectionRecord(t, app, storageProjectMembersResource, map[string]any{"id": "project-normalized:user-normalized", "project_id": "project-normalized", "user_id": "user-normalized", "role": "source"})

	seedStorageProjectionRecordWithoutCanonicalID(t, app, orgUserGroupsResource, map[string]any{"id": "", "user_id": "user-normalized", "group_id": "group-normalized", "role": "source"})
	seedStorageProjectionRecord(t, app, storageUserGroupsResource, map[string]any{"user_id": "user-normalized", "group_id": "group-normalized", "role": "source"})

	seedStorageProjectionRecordWithoutCanonicalID(t, app, identityUsersResource, map[string]any{"id": "", "user_id": "", "name": "alice", "email": "alice@acme"})
	seedStorageProjectionRecordWithoutCanonicalID(t, app, storageIdentityUsersResource, map[string]any{"id": "", "user_id": "", "name": "alice", "email": "alice@acme"})

	seedStorageProjectionRecordWithoutCanonicalID(t, app, identityRolesResource, map[string]any{"id": "", "role_id": "viewer", "name": "viewer"})
	seedStorageProjectionRecordWithoutCanonicalID(t, app, storageIdentityRolesResource, map[string]any{"role_id": "viewer", "name": "viewer"})

	report, err := storageProjectionDrift(ctx, app)
	if err != nil {
		t.Fatalf("storageProjectionDrift returned error: %v", err)
	}
	if len(report.Missing) != 0 || len(report.Orphan) != 0 || len(report.Stale) != 0 {
		t.Fatalf("storageProjectionDrift with canonical IDs = %#v, want no findings", report)
	}
}

func TestStorageProjectionDriftFailsClosed(t *testing.T) {
	app := &platform.App{}
	if _, err := storageProjectionDrift(context.Background(), app); !errors.Is(err, errStorageProjectionDriftUnavailable) {
		t.Fatalf("storageProjectionDrift(%#v) error = %v, want %v", app, err, errStorageProjectionDriftUnavailable)
	}
	if _, err := storageProjectionDrift(context.Background(), nil); !errors.Is(err, errStorageProjectionDriftUnavailable) {
		t.Fatalf("storageProjectionDrift(nil) error = %v, want %v", err, errStorageProjectionDriftUnavailable)
	}
}

func TestStorageProjectionDriftServiceAllFallbackTrap(t *testing.T) {
	ctx := context.Background()
	app := platform.NewApp(platform.Config{ServiceName: "all"})
	seedStorageProjectionRecord(t, app, orgProjectsResource, map[string]any{"id": "trap-project"})

	report, err := storageProjectionDrift(ctx, app)
	if err != nil {
		t.Fatalf("storageProjectionDrift returned error: %v", err)
	}
	want := []storageProjectionDriftFinding{{SourceResource: orgProjectsResource, LocalResource: storageProjectsResource, ID: "trap-project"}}
	if !reflect.DeepEqual(report.Missing, want) {
		t.Fatalf("storageProjectionDrift all-service fallback trap = %#v, want %#v", report.Missing, want)
	}
}

func seedStorageProjectionRecord(t *testing.T, app *platform.App, resource string, row map[string]any) {
	t.Helper()
	_, err := app.Store.Create(context.Background(), resource, row)
	if err != nil {
		t.Fatalf("create %s record: %v", resource, err)
	}
}

func seedStorageProjectionRecordWithoutCanonicalID(t *testing.T, app *platform.App, resource string, row map[string]any) {
	t.Helper()
	record, err := app.Store.Create(context.Background(), resource, row)
	if err != nil {
		t.Fatalf("create %s record: %v", resource, err)
	}
	if _, ok := app.Store.Update(context.Background(), resource, record.ID, map[string]any{"id": ""}); !ok {
		t.Fatalf("clear %s record id: update miss %s", resource, record.ID)
	}
}

func TestStorageMergeSortAndBatchHelperEdges(t *testing.T) {
	local := []map[string]any{
		{"id": "local", "name": "local row"},
		{"project_id": "P1", "name": "project row"},
	}
	source := []map[string]any{
		{"id": "local", "name": "duplicate"},
		{"user_id": "U1", "name": "source user"},
		{"name": "source without id"},
	}
	merged := mergeRows(source, local)
	if len(merged) != 4 || merged[0]["name"] != "local row" || merged[1]["name"] != "project row" ||
		merged[2]["name"] != "source user" || merged[3]["name"] != "source without id" {
		t.Fatalf("mergeRows = %#v, want local rows plus non-duplicate source rows", merged)
	}

	sortRows(merged, "name")
	if merged[0]["name"] != "local row" || merged[len(merged)-1]["name"] != "source without id" {
		t.Fatalf("sortRows by name = %#v", merged)
	}

	payload := map[string]any{
		"items":       []any{map[string]any{"id": "I1"}, "skip"},
		"permissions": []any{map[string]any{"id": "P1"}},
	}
	items := payloadItems(payload)
	if len(items) != 2 || items[0]["id"] != "I1" || items[1]["id"] != "P1" {
		t.Fatalf("payloadItems = %#v, want item and permission maps", items)
	}

	if got := batchError("alice", map[string]any{"message": "denied"}); got != "alice: denied" {
		t.Fatalf("batchError map = %q, want alice: denied", got)
	}
	if got := batchError("bob", "failed"); got != "bob: failed" {
		t.Fatalf("batchError fallback = %q, want bob: failed", got)
	}
}

func TestStorageMountPlanItemHelpers(t *testing.T) {
	direct := []map[string]any{{"pvc_id": "pvc1"}}
	if got := mountPlanItems(direct); len(got) != 1 || got[0]["pvc_id"] != "pvc1" {
		t.Fatalf("mountPlanItems direct = %#v", got)
	}
	if direct[0]["pvc_id"] != "pvc1" {
		t.Fatal("mountPlanItems should copy the slice header without mutating input")
	}

	mixed := []any{map[string]any{"pvc_id": "pvc2"}, "skip"}
	if got := mountPlanItems(mixed); len(got) != 1 || got[0]["pvc_id"] != "pvc2" {
		t.Fatalf("mountPlanItems mixed = %#v", got)
	}
	if got := mountPlanPayloadItems(map[string]any{"storageMounts": map[string]any{"pvc_id": "pvc3"}}); len(got) != 1 || got[0]["pvc_id"] != "pvc3" {
		t.Fatalf("mountPlanPayloadItems = %#v, want pvc3", got)
	}
	if got := mountPlanItems("bad"); got != nil {
		t.Fatalf("mountPlanItems unsupported = %#v, want nil", got)
	}
}
