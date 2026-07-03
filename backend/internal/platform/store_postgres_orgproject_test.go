package platform

import (
	"testing"
	"time"
)

func TestTypedPostgresResourceRoutesOrgProjectAggregates(t *testing.T) {
	spec, ok := typedPostgresResourceFor(orgProjectProjectsResource)
	if !ok || spec.table != "org_projects" {
		t.Fatalf("typedPostgresResourceFor(%q) = %#v, %v, want org_projects table", orgProjectProjectsResource, spec, ok)
	}
	spec, ok = typedPostgresResourceFor(orgProjectMembersResource)
	if !ok || spec.table != "org_project_members" {
		t.Fatalf("typedPostgresResourceFor(%q) = %#v, %v, want org_project_members table", orgProjectMembersResource, spec, ok)
	}
	if _, ok := typedPostgresResourceFor("org-project-service:user_quotas"); ok {
		t.Fatal("only migrated org-project resources should route to a typed table")
	}
}

func TestOrgProjectInsertColumnsPromoteAuthzFields(t *testing.T) {
	cols := orgProjectInsertColumns(map[string]any{
		"project_name": "proj-a", "g_id": "G1", "created_by": "U1",
	}, "", time.Time{})
	got := map[string]any{}
	for _, c := range cols {
		got[c.column] = c.value
	}
	for col, want := range map[string]any{"project_name": "proj-a", "owner_id": "G1", "created_by": "U1"} {
		if got[col] != want {
			t.Fatalf("insert column %q = %v, want %v", col, got[col], want)
		}
	}

	member := map[string]any{}
	for _, c := range orgProjectMemberInsertColumns(map[string]any{"project_id": "P1", "user_id": "U2"}, "", time.Time{}) {
		member[c.column] = c.value
	}
	for col, want := range map[string]any{"project_id": "P1", "user_id": "U2", "role": "user"} {
		if member[col] != want {
			t.Fatalf("member insert column %q = %v, want %v", col, member[col], want)
		}
	}
}

func TestOrgProjectUpdateColumnsOnlyTouchPresentFields(t *testing.T) {
	cols := orgProjectMemberUpdateColumns(map[string]any{"role": "manager"})
	if len(cols) != 1 || cols[0].column != "role" || cols[0].value != "manager" {
		t.Fatalf("update columns = %#v, want only role=manager", cols)
	}
}
