package schedulerquota

import (
	"context"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/platform/cluster"
	corev1 "k8s.io/api/core/v1"
	schedulingv1 "k8s.io/api/scheduling/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestPriorityClassDefinitionFromRecord(t *testing.T) {
	record := recordFromData(map[string]any{
		"id":                "pc1",
		"name":              "platform-batch-high",
		"value":             "10000",
		"preemption_policy": "Never",
		"description":       "high priority",
	})
	def, result := priorityClassDefinitionFromRecord(record)
	if result.Action != "" {
		t.Fatalf("unexpected parse result: %#v", result)
	}
	if def.Name != "platform-batch-high" || def.Value != 10000 || def.PreemptionPolicy != corev1.PreemptNever || def.Description != "high priority" {
		t.Fatalf("definition = %#v", def)
	}
}

func TestPriorityClassDefinitionFromRecordRejectsMalformedRecords(t *testing.T) {
	cases := []map[string]any{
		{"id": "missing-value", "name": "platform-a"},
		{"id": "bad-value", "name": "platform-a", "value": "abc"},
		{"id": "fractional-value", "name": "platform-a", "value": 1.2},
		{"id": "bad-policy", "name": "platform-a", "value": 1, "preemption_policy": "Sometimes"},
	}
	for _, data := range cases {
		_, result := priorityClassDefinitionFromRecord(recordFromData(data))
		if result.Action != cluster.PriorityClassActionInvalid {
			t.Fatalf("record %#v result = %#v, want invalid", data, result)
		}
	}
}

func TestPriorityClassSyncRunCreatesSummaryEventAndPriorityClass(t *testing.T) {
	cl := cluster.New(fake.NewSimpleClientset(), "proj")
	app := platform.NewApp(platform.Config{
		ServiceName:               serviceName,
		PriorityClassSyncInterval: time.Minute,
		RequireAuth:               true,
	}, platform.WithCluster(cl))
	createSchedulerRecord(t, app, priorityClassesResource, map[string]any{
		"id":          "pc1",
		"name":        "platform-batch-low",
		"value":       10,
		"description": "low",
	})

	checkedAt := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	if err := runPriorityClassSync(context.Background(), app, checkedAt); err != nil {
		t.Fatalf("runPriorityClassSync: %v", err)
	}
	pc, err := cl.Clientset().SchedulingV1().PriorityClasses().Get(context.Background(), "platform-batch-low", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get priority class: %v", err)
	}
	if pc.Value != 10 || pc.Labels[cluster.PriorityClassOwnerLabel] != cluster.PriorityClassOwnerValue {
		t.Fatalf("priority class = %#v", pc)
	}
	summary := priorityClassSyncSummaryRecord(t, app)
	if summary["status"] != "synced" || summary["source_count"] != 1 || summary["created_count"] != 1 {
		t.Fatalf("summary = %#v, want synced created", summary)
	}
	if len(app.Events.Outbox()) != 1 || app.Events.Outbox()[0].Name != priorityClassSyncCompletedName {
		t.Fatalf("events = %#v, want %s", app.Events.Outbox(), priorityClassSyncCompletedName)
	}
}

func TestPriorityClassSyncRunReportsEmptyInvalidConflictAndDegraded(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: serviceName, RequireAuth: true})
	if err := runPriorityClassSync(context.Background(), app, time.Now().UTC()); err != nil {
		t.Fatalf("empty sync: %v", err)
	}
	if summary := priorityClassSyncSummaryRecord(t, app); summary["status"] != "no_records" || summary["source_count"] != 0 {
		t.Fatalf("empty summary = %#v", summary)
	}

	app = platform.NewApp(platform.Config{ServiceName: serviceName, RequireAuth: true})
	createSchedulerRecord(t, app, priorityClassesResource, map[string]any{"id": "bad", "name": "system-node-critical", "value": 1})
	if err := runPriorityClassSync(context.Background(), app, time.Now().UTC()); err != nil {
		t.Fatalf("invalid sync: %v", err)
	}
	if summary := priorityClassSyncSummaryRecord(t, app); summary["status"] != "invalid" || summary["invalid_count"] != 1 {
		t.Fatalf("invalid summary = %#v", summary)
	}

	policy := corev1.PreemptLowerPriority
	cl := cluster.New(fake.NewSimpleClientset(&schedulingv1.PriorityClass{
		ObjectMeta:       metav1.ObjectMeta{Name: "platform-conflict"},
		Value:            1,
		PreemptionPolicy: &policy,
		Description:      "unmanaged",
	}), "proj")
	app = platform.NewApp(platform.Config{ServiceName: serviceName, RequireAuth: true}, platform.WithCluster(cl))
	createSchedulerRecord(t, app, priorityClassesResource, map[string]any{"id": "conflict", "name": "platform-conflict", "value": 2})
	if err := runPriorityClassSync(context.Background(), app, time.Now().UTC()); err != nil {
		t.Fatalf("conflict sync: %v", err)
	}
	summary := priorityClassSyncSummaryRecord(t, app)
	if summary["status"] != "conflict" || summary["conflict_count"] != 1 {
		t.Fatalf("conflict summary = %#v", summary)
	}
	pc, _ := cl.Clientset().SchedulingV1().PriorityClasses().Get(context.Background(), "platform-conflict", metav1.GetOptions{})
	if pc.Value != 1 || pc.Description != "unmanaged" {
		t.Fatalf("unmanaged priority class mutated: %#v", pc)
	}

	app = platform.NewApp(platform.Config{ServiceName: serviceName, RequireAuth: true})
	createSchedulerRecord(t, app, priorityClassesResource, map[string]any{"id": "degraded", "name": "platform-degraded", "value": 1})
	if err := runPriorityClassSync(context.Background(), app, time.Now().UTC()); err != nil {
		t.Fatalf("degraded sync: %v", err)
	}
	if summary := priorityClassSyncSummaryRecord(t, app); summary["status"] != "degraded" || summary["degraded"] != true {
		t.Fatalf("degraded summary = %#v", summary)
	}
}

func TestPriorityClassSyncMaintenanceRegistrationIsOwnerGated(t *testing.T) {
	scheduler := platform.NewApp(platform.Config{ServiceName: serviceName, RequireAuth: true})
	Register(scheduler)
	if !containsTask(scheduler.MaintenanceTaskNames(), priorityClassSyncTaskName) {
		t.Fatalf("scheduler maintenance tasks = %v, want %s", scheduler.MaintenanceTaskNames(), priorityClassSyncTaskName)
	}

	workload := platform.NewApp(platform.Config{ServiceName: "workload-service", RequireAuth: true})
	Register(workload)
	if containsTask(workload.MaintenanceTaskNames(), priorityClassSyncTaskName) {
		t.Fatalf("unowned service registered %s: %v", priorityClassSyncTaskName, workload.MaintenanceTaskNames())
	}
}

func priorityClassSyncSummaryRecord(t *testing.T, app *platform.App) map[string]any {
	t.Helper()
	record, found := app.Store.Get(context.Background(), priorityClassSyncRunsResource, priorityClassSyncLatestRunID)
	if !found {
		t.Fatalf("missing %s/%s", priorityClassSyncRunsResource, priorityClassSyncLatestRunID)
	}
	return record.Data
}

func recordFromData(data map[string]any) contracts.Record[map[string]any] {
	return contracts.Record[map[string]any]{ID: data["id"].(string), Data: data}
}

func containsTask(tasks []string, want string) bool {
	for _, task := range tasks {
		if task == want {
			return true
		}
	}
	return false
}
