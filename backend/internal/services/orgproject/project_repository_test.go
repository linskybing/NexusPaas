package orgproject

import (
	"context"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestOrgProjectRepositoryProjectLifecycle(t *testing.T) {
	ctx := context.Background()
	app := platform.NewApp(platform.Config{ServiceName: serviceName})
	repo := projectRepository(app)

	project, err := repo.CreateProject(ctx, map[string]any{
		"id":           "P1",
		"project_name": "training",
		"owner_id":     "G1",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if project.ID != "P1" || project.Data["project_id"] != "P1" || project.Data["ProjectName"] != "training" {
		t.Fatalf("created project = %#v, want normalized aliases", project)
	}
	if _, err := repo.CreateProject(ctx, map[string]any{"id": "P1"}); !platform.IsCreateConflict(err) {
		t.Fatalf("duplicate create err = %v, want create conflict", err)
	}
	if found, ok := repo.FindProject(ctx, "P1"); !ok || found.Data["name"] != "training" {
		t.Fatalf("find project = %#v ok=%v, want P1", found, ok)
	}

	old, updated, ok := repo.UpdateProject(ctx, "P1", map[string]any{"description": "updated"})
	if !ok {
		t.Fatal("update project returned ok=false")
	}
	if old.Data["description"] != nil || updated.Data["description"] != "updated" {
		t.Fatalf("update old=%#v new=%#v, want old/new split", old.Data, updated.Data)
	}

	now := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
	_, workspace, ok := repo.UpdateWorkspaceSettings(ctx, "P1", 3600, now)
	if !ok || workspace.Data["max_ide_runtime_seconds"] != 3600 || workspace.Data["MaxIDERuntimeSeconds"] != 3600 {
		t.Fatalf("workspace update = %#v ok=%v, want mirrored runtime cap", workspace.Data, ok)
	}
}

func TestOrgProjectRepositoryDirectMemberAndQuotaLifecycle(t *testing.T) {
	ctx := context.Background()
	app := platform.NewApp(platform.Config{ServiceName: serviceName})
	repo := projectRepository(app)

	createDirectMemberForRepoTest(t, ctx, repo)
	assertDirectMemberRoleUpdateForRepoTest(t, ctx, repo)
	upsertQuotaForRepoTest(t, ctx, repo)
	assertDirectMemberDeleteRemovesQuotaForRepoTest(t, ctx, repo)
}

func createDirectMemberForRepoTest(t *testing.T, ctx context.Context, repo orgProjectRepository) {
	t.Helper()
	member, err := repo.CreateDirectProjectMember(ctx, map[string]any{
		"id":         projectMemberID("P1", "U1"),
		"project_id": "P1",
		"user_id":    "U1",
		"role":       "user",
	})
	if err != nil {
		t.Fatalf("create direct member: %v", err)
	}
	if member.ID != "P1:U1" {
		t.Fatalf("member id = %q, want composite id", member.ID)
	}
	if _, err := repo.CreateDirectProjectMember(ctx, member.Data); !platform.IsCreateConflict(err) {
		t.Fatalf("duplicate member err = %v, want create conflict", err)
	}
}

func assertDirectMemberRoleUpdateForRepoTest(t *testing.T, ctx context.Context, repo orgProjectRepository) {
	t.Helper()
	old, updated, ok := repo.UpdateDirectProjectMemberRole(ctx, "P1", "U1", "manager", time.Now().UTC())
	if !ok || old.Data["role"] != "user" || updated.Data["role"] != "manager" {
		t.Fatalf("member role old=%#v new=%#v ok=%v, want user->manager", old.Data, updated.Data, ok)
	}
}

func upsertQuotaForRepoTest(t *testing.T, ctx context.Context, repo orgProjectRepository) {
	t.Helper()
	quota, err := repo.UpsertProjectUserQuota(ctx, map[string]any{
		"id":              projectQuotaID("P1", "U1"),
		"project_id":      "P1",
		"user_id":         "U1",
		"gpu_limit":       float64(1),
		"cpu_limit":       float64(4),
		"memory_limit_gb": float64(16),
	})
	if err != nil {
		t.Fatalf("create quota: %v", err)
	}
	if quota.Data["gpu_limit"] != float64(1) {
		t.Fatalf("quota = %#v, want gpu limit", quota.Data)
	}
	quota, err = repo.UpsertProjectUserQuota(ctx, map[string]any{
		"id":         projectQuotaID("P1", "U1"),
		"project_id": "P1",
		"user_id":    "U1",
		"gpu_limit":  float64(2),
	})
	if err != nil {
		t.Fatalf("update quota: %v", err)
	}
	if quota.Data["gpu_limit"] != float64(2) {
		t.Fatalf("updated quota = %#v, want gpu limit 2", quota.Data)
	}
	if got, ok := repo.GetProjectUserQuota(ctx, "P1", "U1"); !ok || got.Data["gpu_limit"] != float64(2) {
		t.Fatalf("get quota = %#v ok=%v, want updated quota", got.Data, ok)
	}
}

func assertDirectMemberDeleteRemovesQuotaForRepoTest(t *testing.T, ctx context.Context, repo orgProjectRepository) {
	t.Helper()
	deleted, ok := repo.DeleteDirectProjectMemberAndQuota(ctx, "P1", "U1")
	if !ok || deleted.Data["role"] != "manager" {
		t.Fatalf("delete member = %#v ok=%v, want deleted manager", deleted.Data, ok)
	}
	if _, ok := repo.FindDirectProjectMember(ctx, "P1", "U1"); ok {
		t.Fatal("direct member still exists after delete")
	}
	if _, ok := repo.GetProjectUserQuota(ctx, "P1", "U1"); ok {
		t.Fatal("quota still exists after direct member delete")
	}
	if repo.DeleteProjectUserQuota(ctx, "P1", "U1") {
		t.Fatal("missing quota delete should be idempotent false")
	}
}

func TestOrgProjectRepositoryProjectDeleteCascade(t *testing.T) {
	ctx := context.Background()
	app := platform.NewApp(platform.Config{ServiceName: serviceName})
	repo := projectRepository(app)
	createOrgRecords(t, app, projectsResource, []map[string]any{{"id": "P1"}, {"id": "P2"}})
	createOrgRecords(t, app, projectMembersResource, []map[string]any{
		{"id": "P1:U1", "project_id": "P1", "user_id": "U1"},
		{"id": "P2:U1", "project_id": "P2", "user_id": "U1"},
	})
	createOrgRecords(t, app, projectUserQuotasResource, []map[string]any{
		{"id": "P1:U1", "project_id": "P1", "user_id": "U1"},
		{"id": "P2:U1", "project_id": "P2", "user_id": "U1"},
	})
	createOrgRecords(t, app, gpuClaimsResource, []map[string]any{
		{"id": "P1:ns:claim", "project_id": "P1", "name": "claim"},
		{"id": "P2:ns:claim", "project_id": "P2", "name": "claim"},
	})

	project, result, ok := repo.DeleteProjectCascade(ctx, "P1")
	if !ok || project.ID != "P1" {
		t.Fatalf("delete cascade project=%#v ok=%v, want P1", project, ok)
	}
	if result.ProjectMembers != 1 || result.UserQuotas != 1 || result.GPUClaims != 1 {
		t.Fatalf("delete cascade result = %#v, want one of each child", result)
	}
	assertOrgRecordMissing(t, app, projectsResource, "P1")
	assertOrgRecordMissing(t, app, projectMembersResource, "P1:U1")
	assertOrgRecordMissing(t, app, projectUserQuotasResource, "P1:U1")
	assertOrgRecordMissing(t, app, gpuClaimsResource, "P1:ns:claim")
	if _, ok := app.Store.Get(ctx, projectsResource, "P2"); !ok {
		t.Fatal("delete cascade removed unrelated project")
	}
}

func TestOrgProjectRepositoryPlanBinding(t *testing.T) {
	ctx := context.Background()
	app := platform.NewApp(platform.Config{ServiceName: serviceName})
	repo := projectRepository(app)
	createOrgRecords(t, app, projectsResource, []map[string]any{
		{"id": "P1", "plan_id": "old", "resource_plan_id": "old"},
		{"id": "P2", "plan_id": "new", "resource_plan_id": "new"},
	})

	old, updated, ok := repo.BindProjectPlan(ctx, "P1", "new", time.Now().UTC())
	if !ok || old.Data["plan_id"] != "old" || updated.Data["plan_id"] != "new" || updated.Data["resource_plan_id"] != "new" {
		t.Fatalf("bind old=%#v new=%#v ok=%v, want old->new plan", old.Data, updated.Data, ok)
	}
	updates := repo.ClearProjectsPlan(ctx, "new", time.Now().UTC())
	if len(updates) != 2 {
		t.Fatalf("clear updates = %#v, want P1 and P2", updates)
	}
	for _, update := range updates {
		if update.New.Data["plan_id"] != "" || update.New.Data["resource_plan_id"] != "" {
			t.Fatalf("cleared update = %#v, want empty plan fields", update.New.Data)
		}
	}
}
