package authorizationpolicy

import (
	"context"
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/platform/cluster"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestBuildPolicyConfigMapDataRestrictiveMissingProject(t *testing.T) {
	data := buildPolicyConfigMapData(policyDataBuildInput{imageCheckEnabled: true})

	want := map[string]string{
		"maxJobRuntimeSeconds":  "0",
		"gpuLimit":              "0",
		"imageCheckEnabled":     "true",
		"timeAllowed":           "false",
		"gpuNamespaceUsage":     "0",
		"allowedProxyImages":    ",",
		"allowedMirroredImages": ",",
		"syncedMirroredImages":  ",",
		"publishedBuiltImages":  ",",
	}
	for key, value := range want {
		if data[key] != value {
			t.Fatalf("%s = %q, want %q in %#v", key, data[key], value, data)
		}
	}
}

func TestBuildPolicyConfigMapDataPlanTimeImagesAndUsage(t *testing.T) {
	now := time.Date(2026, 6, 15, 1, 0, 0, 0, time.UTC) // Monday 01:00 UTC.
	data := buildPolicyConfigMapData(policyDataBuildInput{
		project: map[string]any{
			"id":                      "P1",
			"plan_id":                 "PL1",
			"max_job_runtime_seconds": 3600,
		},
		plan: map[string]any{
			"id":           "PL1",
			"gpu_limit":    2.5,
			"valid_from":   now.Add(-time.Hour).Format(time.RFC3339),
			"valid_until":  now.Add(time.Hour).Format(time.RFC3339),
			"week_windows": `[{"start":0,"end":7200}]`,
		},
		imageRules: []map[string]any{
			{"id": "proxy", "enabled": true, "repository": "repo/proxy", "tag": "v1"},
			{"id": "mirrored", "enabled": true, "image_reference": "repo/mirror:v2", "delivery_mode": "mirrored"},
			{"id": "synced", "enabled": true, "image_reference": "repo/synced:v3", "mode": "synced_mirrored"},
			{"id": "built", "enabled": true, "image_reference": "repo/built:v4", "deliveryMode": "published"},
			{"id": "disabled", "enabled": false, "image_reference": "repo/disabled:v5"},
		},
		now:               now,
		imageCheckEnabled: true,
		gpuUsage:          1.5,
	})

	assertPolicyDataValue(t, data, "maxJobRuntimeSeconds", "3600")
	assertPolicyDataValue(t, data, "gpuLimit", "2.5")
	assertPolicyDataValue(t, data, "imageCheckEnabled", "true")
	assertPolicyDataValue(t, data, "timeAllowed", "true")
	assertPolicyDataValue(t, data, "gpuNamespaceUsage", "1.5")
	assertPolicyDataValue(t, data, "allowedProxyImages", ",repo/proxy:v1,")
	assertPolicyDataValue(t, data, "allowedMirroredImages", ",repo/mirror:v2,")
	assertPolicyDataValue(t, data, "syncedMirroredImages", ",repo/synced:v3,")
	assertPolicyDataValue(t, data, "publishedBuiltImages", ",repo/built:v4,")
}

func TestPolicyDataProjectionProjectsPlansAndImages(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: serviceName})
	ctx := context.Background()

	publishPolicyDataTestEvent(t, app, "ProjectCreated", "org-project-service", map[string]any{"id": "P1", "plan_id": "PL1"})
	publishPolicyDataTestEvent(t, app, "PlanChanged", "scheduler-quota-service", map[string]any{"action": "bound_project", "project_id": "P1", "plan_id": "PL1"})
	publishPolicyDataTestEvent(t, app, "PlanChanged", "scheduler-quota-service", map[string]any{"action": "created", "id": "PL1", "gpu_limit": 2})
	publishPolicyDataTestEvent(t, app, "ImagePublished", "image-registry-service", map[string]any{"id": "P1:T1", "project_id": "P1", "tag_id": "T1", "image_reference": "repo/app:v1", "enabled": true})

	syncPolicyDataReadModels(ctx, app)

	if _, ok := app.Store.Get(ctx, policyDataProjectsResource, "P1"); !ok {
		t.Fatal("project read model was not projected")
	}
	if _, ok := app.Store.Get(ctx, policyDataPlansResource, "PL1"); !ok {
		t.Fatal("plan read model was not projected")
	}
	if _, ok := app.Store.Get(ctx, policyDataPlansResource, "P1"); ok {
		t.Fatal("bound_project event created a partial plan read model")
	}
	if _, ok := app.Store.Get(ctx, policyDataImageAllowListsResource, "P1:T1"); !ok {
		t.Fatal("image allow-list read model was not projected")
	}

	publishPolicyDataTestEvent(t, app, "ProjectDeleted", "org-project-service", map[string]any{"id": "P1"})
	publishPolicyDataTestEvent(t, app, "PlanChanged", "scheduler-quota-service", map[string]any{"action": "deleted", "id": "PL1"})
	publishPolicyDataTestEvent(t, app, "ProjectImageRemoved", "image-registry-service", map[string]any{"id": "P1:T1"})
	syncPolicyDataReadModels(ctx, app)

	if _, ok := app.Store.Get(ctx, policyDataProjectsResource, "P1"); ok {
		t.Fatal("project read model was not deleted")
	}
	if _, ok := app.Store.Get(ctx, policyDataPlansResource, "PL1"); ok {
		t.Fatal("plan read model was not deleted")
	}
	if _, ok := app.Store.Get(ctx, policyDataImageAllowListsResource, "P1:T1"); ok {
		t.Fatal("image allow-list read model was not deleted")
	}
}

func TestPolicyDataSourceFallbackOnlyWhenCoHosted(t *testing.T) {
	ctx := context.Background()
	isolated := platform.NewApp(platform.Config{ServiceName: serviceName})
	createPolicyRecords(t, isolated, policySourceProjectsResource, []map[string]any{{"id": "P1"}})
	if got := authorizationPolicyProjectionRepo(isolated).ListPolicyProjects(ctx); len(got) != 0 {
		t.Fatalf("isolated fallback records = %#v, want none", got)
	}

	cohosted := platform.NewApp(platform.Config{ServiceName: "all"})
	createPolicyRecords(t, cohosted, policySourceProjectsResource, []map[string]any{{"id": "P1"}})
	got := authorizationPolicyProjectionRepo(cohosted).ListPolicyProjects(ctx)
	if len(got) != 1 || policyProjectID(got[0]) != "P1" {
		t.Fatalf("cohosted fallback records = %#v, want source P1", got)
	}
}

func TestPolicyDataSyncMaintenanceWritesConfigMaps(t *testing.T) {
	ctx := context.Background()
	projectID := "p1"
	namespace := "proj-p1-alice"
	clusterClient := cluster.New(fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}},
		policyDataUsagePod(namespace, "gpu-job", projectID, "1.25"),
	), "proj")
	app := platform.NewApp(platform.Config{
		ServiceName:         serviceName,
		ImageCheckEnabled:   true,
		MaintenanceInterval: time.Minute,
	}, platform.WithCluster(clusterClient))
	Register(app)

	if !slices.Equal(app.MaintenanceTaskNames(), []string{policyDataTaskName}) {
		t.Fatalf("maintenance tasks = %v, want only %s", app.MaintenanceTaskNames(), policyDataTaskName)
	}
	createPolicyRecords(t, app, policyDataProjectsResource, []map[string]any{{
		"id":                      projectID,
		"plan_id":                 "PL1",
		"max_job_runtime_seconds": 1800,
	}})
	createPolicyRecords(t, app, policyDataPlansResource, []map[string]any{{
		"id":        "PL1",
		"gpu_limit": 4,
	}})
	createPolicyRecords(t, app, policyDataImageAllowListsResource, []map[string]any{{
		"id":              projectID + ":T1",
		"project_id":      projectID,
		"image_reference": "repo/app:v1",
		"enabled":         true,
	}})

	app.RunMaintenanceOnce(ctx, time.Minute)

	got, err := clusterClient.Clientset().CoreV1().ConfigMaps(namespace).Get(ctx, cluster.PolicyDataConfigMapName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	assertPolicyDataValue(t, got.Data, "maxJobRuntimeSeconds", "1800")
	assertPolicyDataValue(t, got.Data, "gpuLimit", "4")
	assertPolicyDataValue(t, got.Data, "imageCheckEnabled", "true")
	assertPolicyDataValue(t, got.Data, "timeAllowed", "true")
	assertPolicyDataValue(t, got.Data, "gpuNamespaceUsage", "1.25")
	assertPolicyDataValue(t, got.Data, "allowedProxyImages", ",repo/app:v1,")
}

func TestPolicyPlanWindowsCanDenyRuntime(t *testing.T) {
	now := time.Date(2026, 6, 15, 3, 0, 0, 0, time.UTC)
	data := buildPolicyConfigMapData(policyDataBuildInput{
		project: map[string]any{"id": "P1", "plan_id": "PL1"},
		plan:    map[string]any{"id": "PL1", "week_windows": []any{map[string]any{"start": 0, "end": 3600}}},
		now:     now,
	})
	assertPolicyDataValue(t, data, "timeAllowed", "false")
}

func TestPolicyDataLookupHelpers(t *testing.T) {
	ctx := context.Background()
	app := platform.NewApp(platform.Config{ServiceName: "all"})
	createPolicyRecords(t, app, policyDataPlansResource, []map[string]any{{"id": "PL1", "gpu_limit": json.Number("2.5")}})
	createPolicyRecords(t, app, policyDataImageAllowListsResource, []map[string]any{
		{"id": "P1:T1", "project_id": "P1", "image_reference": "repo/project:v1", "enabled": true},
		{"id": "wild", "project_id": "*", "repository": "repo/wild", "delivery_mode": "mirrored", "enabled": true},
		{"id": "other", "project_id": "P2", "image_reference": "repo/other:v1", "enabled": true},
	})

	plan := policyPlanForProject(ctx, app, map[string]any{"plan_id": "PL1"})
	if policyPlanID(plan) != "PL1" {
		t.Fatalf("policyPlanForProject = %#v, want PL1", plan)
	}
	rules := policyImageRulesForProject(ctx, app, "P1")
	if len(rules) != 2 {
		t.Fatalf("policyImageRulesForProject = %#v, want project and wildcard rules", rules)
	}
	if namespaces, err := policyProjectNamespaces(ctx, nil, "P1"); err != nil || namespaces != nil {
		t.Fatalf("nil namespace lookup = %#v/%v, want nil", namespaces, err)
	}
	if usage := policyGPUUsageForNamespaces(ctx, nil, []string{"proj-p1"}, time.Now()); usage != 0 {
		t.Fatalf("nil GPU usage = %g, want 0", usage)
	}
}

func TestPolicyImageListHelperBranches(t *testing.T) {
	images := policyImageLists([]map[string]any{
		{"enabled": true, "repository": "repo/a"},
		{"enabled": true, "repository": "repo/a", "tag": "latest"},
		{"enabled": true, "image": "repo/b:v2", "mode": "synced"},
		{"enabled": true, "fullName": "repo/c:v3", "mode": "published-built"},
		{"enabled": true, "image_reference": "repo/d:v4", "mode": "unsupported"},
		{"enabled": false, "image_reference": "repo/disabled:v1"},
	})
	assertPolicyDataValue(t, images, "allowedProxyImages", ",repo/a:*,repo/a:latest,")
	assertPolicyDataValue(t, images, "syncedMirroredImages", ",repo/b:v2,")
	assertPolicyDataValue(t, images, "publishedBuiltImages", ",repo/c:v3,")
	assertPolicyDataValue(t, images, "allowedMirroredImages", ",")
}

func TestPolicyPlanTimeAndWindowBranches(t *testing.T) {
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	if policyPlanActive(map[string]any{}, now) {
		t.Fatal("empty plan should not be active")
	}
	if policyPlanActive(map[string]any{"id": "future", "valid_from": now.Add(time.Hour).Format(time.RFC3339)}, now) {
		t.Fatal("future plan should not be active")
	}
	if policyPlanActive(map[string]any{"id": "expired", "valid_until": now.Add(-time.Hour)}, now) {
		t.Fatal("expired plan should not be active")
	}
	if !policyPlanActive(map[string]any{"id": "active", "weekWindows": []any{map[string]any{"start": 0, "end": 604800}}}, now) {
		t.Fatal("active all-week plan should be active")
	}
	if policyWeekWindows(map[string]any{"week_windows": 42}) != nil {
		t.Fatal("non-window value should not decode week windows")
	}
	if policyPlanPayloadHasRuntimeFields(map[string]any{"id": "PL2"}) {
		t.Fatal("plan payload without runtime fields should not project")
	}
	if policyNonNegativeInt(map[string]any{"limit": -2}, "limit") != 0 {
		t.Fatal("negative policy int should clamp to zero")
	}
}

func TestPolicyNumberParsingBranches(t *testing.T) {
	numberCases := []map[string]any{
		{"n": float32(1.5)},
		{"n": int(2)},
		{"n": int64(3)},
		{"n": json.Number("4.5")},
		{"n": "6.25"},
	}
	for _, tc := range numberCases {
		if policyNumberValue(tc, "n") == 0 {
			t.Fatalf("policyNumberValue(%#v) returned zero", tc)
		}
	}
	if policyNumberValue(map[string]any{"n": "bad"}, "n") != 0 {
		t.Fatal("invalid policy number should return zero")
	}
	intCases := []any{int(1), int32(2), float64(3), json.Number("4"), "5"}
	for _, value := range intCases {
		if policyInt64Value(value, -1) < 1 {
			t.Fatalf("policyInt64Value(%#v) did not parse", value)
		}
	}
	if policyInt64Value("bad", -1) != -1 {
		t.Fatal("invalid policy int should return fallback")
	}
}

func policyDataUsagePod(namespace, name, projectID, gpu string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
			Labels: map[string]string{
				cluster.LabelJobID:           "J1",
				cluster.LabelProjectID:       projectID,
				cluster.LabelUserID:          "U1",
				cluster.LabelDRAEffectiveGPU: gpu,
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name: "main",
				Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("1"),
					corev1.ResourceMemory: resource.MustParse("1Gi"),
				}},
			}},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}
}

func publishPolicyDataTestEvent(t *testing.T, app *platform.App, name, source string, data map[string]any) {
	t.Helper()
	if err := app.Events.Publish(context.Background(), contracts.Event{
		EventID:       platform.NewUUID(),
		Name:          name,
		Source:        source,
		OccurredAt:    time.Now().UTC(),
		TraceID:       platform.NewUUID(),
		SchemaVersion: 1,
		Data:          data,
	}); err != nil {
		t.Fatal(err)
	}
}

func assertPolicyDataValue(t *testing.T, data map[string]string, key, want string) {
	t.Helper()
	if data[key] != want {
		t.Fatalf("%s = %q, want %q in %#v", key, data[key], want, data)
	}
}
