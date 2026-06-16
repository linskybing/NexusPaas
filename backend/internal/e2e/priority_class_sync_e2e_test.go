//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/platform/cluster"
	"github.com/linskybing/nexuspaas/backend/internal/services"
	corev1 "k8s.io/api/core/v1"
	schedulingv1 "k8s.io/api/scheduling/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

const (
	e2ePriorityClassesResource = "scheduler-quota-service:priority_classes"
	e2ePrioritySyncRuns        = "scheduler-quota-service:priority_class_sync_runs"
	e2ePrioritySyncEvent       = "PriorityClassSyncCompleted"
	e2ePriorityRunLabel        = "nexuspaas.io/e2e-run"
)

func TestPriorityClassSyncWorkerE2E(t *testing.T) {
	ctx := context.Background()
	runID := "pcsync" + sanitizeID(time.Now().UTC().Format("150405.000000000"))
	createName := "nexuspaas-e2e-priority-" + runID + "-create"
	updateName := "nexuspaas-e2e-priority-" + runID + "-update"
	recreateName := "nexuspaas-e2e-priority-" + runID + "-recreate"
	conflictName := "nexuspaas-e2e-priority-" + runID + "-conflict"
	lower := corev1.PreemptLowerPriority

	cl := cluster.New(fake.NewSimpleClientset(
		e2eManagedPriorityClass(updateName, 100, lower, "old", runID),
		e2eManagedPriorityClass(recreateName, 50, lower, "old", runID),
		&schedulingv1.PriorityClass{
			ObjectMeta:       metav1.ObjectMeta{Name: conflictName, Labels: map[string]string{e2ePriorityRunLabel: runID}},
			Value:            1,
			PreemptionPolicy: &lower,
			Description:      "unmanaged",
		},
	), "proj")
	app := platform.NewApp(priorityClassSyncE2EConfig(), platform.WithCluster(cl))
	services.RegisterAll(app)
	seedPriorityClassSyncRecords(t, app, runID, createName, updateName, recreateName, conflictName)

	app.RunMaintenanceOnce(ctx, time.Second)

	assertPriorityClassValue(t, cl, createName, 10, "created")
	assertPriorityClassValue(t, cl, updateName, 100, "updated")
	assertPriorityClassValue(t, cl, recreateName, 500, "recreated")
	conflict := getE2EPriorityClass(t, cl, conflictName)
	if conflict.Value != 1 || conflict.Labels[cluster.PriorityClassOwnerLabel] != "" {
		t.Fatalf("unmanaged conflict class mutated: %#v", conflict)
	}
	summary := getPriorityClassSyncSummary(t, app)
	if summary["status"] != "conflict" ||
		summary["source_count"] != 4 ||
		summary["created_count"] != 1 ||
		summary["updated_count"] != 1 ||
		summary["recreated_count"] != 1 ||
		summary["conflict_count"] != 1 {
		t.Fatalf("summary = %#v, want create/update/recreate/conflict counts", summary)
	}
	if countE2EEvents(app, e2ePrioritySyncEvent) != 1 {
		t.Fatalf("%s events = %d, want 1", e2ePrioritySyncEvent, countE2EEvents(app, e2ePrioritySyncEvent))
	}
}

func TestPriorityClassSyncWorkerLiveK8sE2E(t *testing.T) {
	if strings.TrimSpace(os.Getenv("TEST_LIVE_K8S_PRIORITY_CLASS_SYNC")) != "1" {
		t.Skip("TEST_LIVE_K8S_PRIORITY_CLASS_SYNC=1 not set; skipping live Kubernetes priority-class sync e2e")
	}
	ensureDefaultKubeconfigNoSkip(t)
	ctx := context.Background()
	cl, err := cluster.NewFromEnv("proj")
	if err != nil {
		t.Fatalf("create cluster client: %v", err)
	}
	if cl == nil {
		t.Fatal("no Kubernetes client available")
	}

	runID := "pcsync" + sanitizeID(time.Now().UTC().Format("150405.000000000"))
	createName := "nexuspaas-e2e-priority-" + runID + "-create"
	updateName := "nexuspaas-e2e-priority-" + runID + "-update"
	recreateName := "nexuspaas-e2e-priority-" + runID + "-recreate"
	conflictName := "nexuspaas-e2e-priority-" + runID + "-conflict"
	names := []string{createName, updateName, recreateName, conflictName}
	cleanup := func() {
		leftovers := cleanupE2EPriorityClasses(ctx, cl, runID, names)
		if len(leftovers) > 0 {
			t.Errorf("leftover priority classes after cleanup: %s", strings.Join(leftovers, ","))
		}
	}
	t.Cleanup(cleanup)
	cleanup()

	lower := corev1.PreemptLowerPriority
	if _, err := cl.Clientset().SchedulingV1().PriorityClasses().Create(ctx, e2eManagedPriorityClass(updateName, 100, lower, "old", runID), metav1.CreateOptions{}); err != nil {
		t.Fatalf("create managed mutable fixture: %v", err)
	}
	if _, err := cl.Clientset().SchedulingV1().PriorityClasses().Create(ctx, e2eManagedPriorityClass(recreateName, 50, lower, "old", runID), metav1.CreateOptions{}); err != nil {
		t.Fatalf("create managed immutable fixture: %v", err)
	}
	if _, err := cl.Clientset().SchedulingV1().PriorityClasses().Create(ctx, &schedulingv1.PriorityClass{
		ObjectMeta:       metav1.ObjectMeta{Name: conflictName, Labels: map[string]string{e2ePriorityRunLabel: runID}},
		Value:            1,
		PreemptionPolicy: &lower,
		Description:      "unmanaged",
	}, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create unmanaged conflict fixture: %v", err)
	}

	app := platform.NewApp(priorityClassSyncE2EConfig(), platform.WithCluster(cl))
	services.RegisterAll(app)
	seedPriorityClassSyncRecords(t, app, runID, createName, updateName, recreateName, conflictName)
	app.RunMaintenanceOnce(ctx, time.Second)

	assertPriorityClassValue(t, cl, createName, 10, "created")
	assertPriorityClassValue(t, cl, updateName, 100, "updated")
	assertPriorityClassValue(t, cl, recreateName, 500, "recreated")
	conflict := getE2EPriorityClass(t, cl, conflictName)
	if conflict.Value != 1 || conflict.Description != "unmanaged" || conflict.Labels[cluster.PriorityClassOwnerLabel] != "" {
		t.Fatalf("unmanaged live conflict class mutated: %#v", conflict)
	}
	summary := getPriorityClassSyncSummary(t, app)
	if summary["created_count"] != 1 || summary["updated_count"] != 1 || summary["recreated_count"] != 1 || summary["conflict_count"] != 1 {
		t.Fatalf("live summary = %#v, want create/update/recreate/conflict", summary)
	}
	cleanup()
}

func seedPriorityClassSyncRecords(t *testing.T, app *platform.App, runID, createName, updateName, recreateName, conflictName string) {
	t.Helper()
	for _, item := range []struct {
		name        string
		value       int
		description string
	}{
		{createName, 10, "created"},
		{updateName, 100, "updated"},
		{recreateName, 500, "recreated"},
		{conflictName, 2, "should-not-apply"},
	} {
		createE2ERecord(t, app, e2ePriorityClassesResource, map[string]any{
			"id":          item.name,
			"name":        item.name,
			"value":       item.value,
			"description": item.description,
			"labels":      map[string]any{e2ePriorityRunLabel: runID},
		})
	}
}

func priorityClassSyncE2EConfig() platform.Config {
	return platform.Config{
		ServiceName:             schedulerQuotaService,
		RequireAuth:             false,
		ServiceAPIKey:           "priority-class-e2e-service-key",
		ServiceFallbackDisabled: true,
		ServiceURLs: map[string]string{
			workloadService: "http://127.0.0.1:1",
		},
	}
}

func e2eManagedPriorityClass(name string, value int32, policy corev1.PreemptionPolicy, description, runID string) *schedulingv1.PriorityClass {
	return &schedulingv1.PriorityClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				cluster.PriorityClassManagedByLabel: cluster.PriorityClassManagedByValue,
				cluster.PriorityClassPartOfLabel:    cluster.PriorityClassPartOfValue,
				cluster.PriorityClassOwnerLabel:     cluster.PriorityClassOwnerValue,
				e2ePriorityRunLabel:                 runID,
			},
			Annotations: map[string]string{
				cluster.PriorityClassManagedAnnotation: cluster.PriorityClassManagedResource,
			},
		},
		Value:            value,
		PreemptionPolicy: &policy,
		GlobalDefault:    false,
		Description:      description,
	}
}

func assertPriorityClassValue(t *testing.T, cl *cluster.Client, name string, want int32, wantDescription string) {
	t.Helper()
	pc := getE2EPriorityClass(t, cl, name)
	if pc.Value != want || pc.Description != wantDescription {
		t.Fatalf("%s = value:%d description:%q, want %d/%q", name, pc.Value, pc.Description, want, wantDescription)
	}
	if pc.Labels[cluster.PriorityClassOwnerLabel] != cluster.PriorityClassOwnerValue {
		t.Fatalf("%s missing owner label: %#v", name, pc.Labels)
	}
}

func getE2EPriorityClass(t *testing.T, cl *cluster.Client, name string) *schedulingv1.PriorityClass {
	t.Helper()
	pc, err := cl.Clientset().SchedulingV1().PriorityClasses().Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get priority class %s: %v", name, err)
	}
	return pc
}

func getPriorityClassSyncSummary(t *testing.T, app *platform.App) map[string]any {
	t.Helper()
	record, found := app.Store.Get(context.Background(), e2ePrioritySyncRuns, "latest")
	if !found {
		t.Fatalf("missing %s/latest", e2ePrioritySyncRuns)
	}
	return record.Data
}

func cleanupE2EPriorityClasses(ctx context.Context, cl *cluster.Client, runID string, names []string) []string {
	var leftovers []string
	for _, name := range names {
		if !strings.HasPrefix(name, "nexuspaas-e2e-priority-") {
			continue
		}
		pc, err := cl.Clientset().SchedulingV1().PriorityClasses().Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			continue
		}
		if err != nil {
			leftovers = append(leftovers, fmt.Sprintf("%s(get:%v)", name, err))
			continue
		}
		if pc.Labels[e2ePriorityRunLabel] != runID {
			continue
		}
		if err := cl.Clientset().SchedulingV1().PriorityClasses().Delete(ctx, name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			leftovers = append(leftovers, fmt.Sprintf("%s(delete:%v)", name, err))
			continue
		}
		if _, err := cl.Clientset().SchedulingV1().PriorityClasses().Get(ctx, name, metav1.GetOptions{}); err == nil {
			leftovers = append(leftovers, name)
		}
	}
	return leftovers
}
