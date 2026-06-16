package cluster

import (
	"context"
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	schedulingv1 "k8s.io/api/scheduling/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestPriorityClassCreatesMissingManagedClass(t *testing.T) {
	cl := New(fake.NewSimpleClientset(), "proj")

	result := cl.EnsurePriorityClassDefinition(context.Background(), PriorityClassDefinition{
		Name:             "platform-batch-low",
		Value:            10,
		PreemptionPolicy: corev1.PreemptLowerPriority,
		Description:      "batch low",
	})
	if result.Action != PriorityClassActionCreated {
		t.Fatalf("action = %#v, want created", result)
	}
	pc := getPriorityClass(t, cl, "platform-batch-low")
	if pc.Value != 10 || priorityClassPolicy(pc) != corev1.PreemptLowerPriority || pc.Description != "batch low" {
		t.Fatalf("created priority class = %#v", pc)
	}
	assertPriorityClassManaged(t, pc)
}

func TestPriorityClassUpdatesManagedMutableDrift(t *testing.T) {
	policy := corev1.PreemptLowerPriority
	existing := managedPriorityClass("platform-batch-medium", 1000, policy)
	existing.Description = "old"
	cl := New(fake.NewSimpleClientset(existing), "proj")

	result := cl.EnsurePriorityClassDefinition(context.Background(), PriorityClassDefinition{
		Name:             "platform-batch-medium",
		Value:            1000,
		PreemptionPolicy: corev1.PreemptLowerPriority,
		Description:      "new",
		Labels:           map[string]string{"nexuspaas.io/e2e": "true"},
	})
	if result.Action != PriorityClassActionUpdated {
		t.Fatalf("action = %#v, want updated", result)
	}
	pc := getPriorityClass(t, cl, "platform-batch-medium")
	if pc.Description != "new" || pc.Labels["nexuspaas.io/e2e"] != "true" {
		t.Fatalf("updated priority class = %#v, want description and labels updated", pc)
	}
	assertPriorityClassManaged(t, pc)
}

func TestPriorityClassRecreatesManagedImmutableDrift(t *testing.T) {
	oldPolicy := corev1.PreemptLowerPriority
	cl := New(fake.NewSimpleClientset(managedPriorityClass("platform-batch-high", 1000, oldPolicy)), "proj")
	deleteCount := 0
	cl.Clientset().(*fake.Clientset).PrependReactor("delete", "priorityclasses", func(action k8stesting.Action) (bool, runtime.Object, error) {
		deleteCount++
		return false, nil, nil
	})

	result := cl.EnsurePriorityClassDefinition(context.Background(), PriorityClassDefinition{
		Name:             "platform-batch-high",
		Value:            10000,
		PreemptionPolicy: corev1.PreemptNever,
		Description:      "high",
	})
	if result.Action != PriorityClassActionRecreated {
		t.Fatalf("action = %#v, want recreated", result)
	}
	if deleteCount != 1 {
		t.Fatalf("delete count = %d, want 1", deleteCount)
	}
	pc := getPriorityClass(t, cl, "platform-batch-high")
	if pc.Value != 10000 || priorityClassPolicy(pc) != corev1.PreemptNever {
		t.Fatalf("recreated priority class = %#v, want new immutable fields", pc)
	}
	assertPriorityClassManaged(t, pc)
}

func TestPriorityClassUnmanagedImmutableDriftConflictsWithoutMutation(t *testing.T) {
	policy := corev1.PreemptLowerPriority
	existing := &schedulingv1.PriorityClass{
		ObjectMeta:       metav1.ObjectMeta{Name: "platform-batch-low"},
		Value:            10,
		PreemptionPolicy: &policy,
		Description:      "unmanaged",
	}
	cl := New(fake.NewSimpleClientset(existing), "proj")

	result := cl.EnsurePriorityClassDefinition(context.Background(), PriorityClassDefinition{
		Name:             "platform-batch-low",
		Value:            100,
		PreemptionPolicy: corev1.PreemptLowerPriority,
		Description:      "should-not-apply",
	})
	if result.Action != PriorityClassActionConflict {
		t.Fatalf("action = %#v, want conflict", result)
	}
	pc := getPriorityClass(t, cl, "platform-batch-low")
	if pc.Value != 10 || pc.Description != "unmanaged" || pc.Labels[PriorityClassManagedByLabel] != "" {
		t.Fatalf("unmanaged priority class was mutated: %#v", pc)
	}
}

func TestPriorityClassSafelyAdoptsUnmanagedIdenticalImmutableFields(t *testing.T) {
	policy := corev1.PreemptLowerPriority
	existing := &schedulingv1.PriorityClass{
		ObjectMeta:       metav1.ObjectMeta{Name: "platform-batch-low"},
		Value:            10,
		PreemptionPolicy: &policy,
		Description:      "old",
	}
	cl := New(fake.NewSimpleClientset(existing), "proj")

	result := cl.EnsurePriorityClassDefinition(context.Background(), PriorityClassDefinition{
		Name:             "platform-batch-low",
		Value:            10,
		PreemptionPolicy: corev1.PreemptLowerPriority,
		Description:      "adopted",
	})
	if result.Action != PriorityClassActionAdopted {
		t.Fatalf("action = %#v, want adopted", result)
	}
	pc := getPriorityClass(t, cl, "platform-batch-low")
	if pc.Value != 10 || pc.Description != "adopted" {
		t.Fatalf("adopted priority class = %#v, want immutable preserved and mutable updated", pc)
	}
	assertPriorityClassManaged(t, pc)
}

func TestPriorityClassRejectsInvalidNamesAndPolicies(t *testing.T) {
	cl := New(fake.NewSimpleClientset(), "proj")
	cases := []PriorityClassDefinition{
		{Name: "", Value: 1},
		{Name: "system-node-critical", Value: 1},
		{Name: "Bad_Name", Value: 1},
		{Name: "platform-bad-policy", Value: 1, PreemptionPolicy: corev1.PreemptionPolicy("Sometimes")},
	}
	for _, def := range cases {
		result := cl.EnsurePriorityClassDefinition(context.Background(), def)
		if result.Action != PriorityClassActionInvalid {
			t.Fatalf("EnsurePriorityClassDefinition(%#v) = %#v, want invalid", def, result)
		}
	}
	if list, err := cl.Clientset().SchedulingV1().PriorityClasses().List(context.Background(), metav1.ListOptions{}); err != nil || len(list.Items) != 0 {
		t.Fatalf("invalid definitions mutated cluster: items=%#v err=%v", list, err)
	}
}

func TestPriorityClassSyncSummaryCountsConflictAndDegraded(t *testing.T) {
	policy := corev1.PreemptLowerPriority
	unmanaged := &schedulingv1.PriorityClass{
		ObjectMeta:       metav1.ObjectMeta{Name: "platform-conflict"},
		Value:            1,
		PreemptionPolicy: &policy,
	}
	cl := New(fake.NewSimpleClientset(unmanaged), "proj")
	summary := cl.SyncPriorityClasses(context.Background(), []PriorityClassDefinition{
		{Name: "platform-created", Value: 1},
		{Name: "platform-conflict", Value: 2},
		{Name: "system-node-critical", Value: 1},
	})
	if summary.SourceCount != 3 || summary.Created != 1 || summary.Conflict != 1 || summary.Invalid != 1 {
		t.Fatalf("summary = %#v, want created/conflict/invalid counts", summary)
	}

	degraded := New(nil, "proj").SyncPriorityClasses(context.Background(), []PriorityClassDefinition{{Name: "platform-degraded", Value: 1}})
	if !degraded.Degraded || len(degraded.Results) != 1 || degraded.Results[0].Action != PriorityClassActionDegraded {
		t.Fatalf("degraded summary = %#v", degraded)
	}
}

func TestPriorityClassKubernetesOperationErrorIsReported(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	clientset.PrependReactor("create", "priorityclasses", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("api denied")
	})
	cl := New(clientset, "proj")

	result := cl.EnsurePriorityClassDefinition(context.Background(), PriorityClassDefinition{Name: "platform-denied", Value: 1})
	if result.Action != PriorityClassActionFailed || result.Error == "" {
		t.Fatalf("result = %#v, want failed with error", result)
	}
}

func managedPriorityClass(name string, value int32, policy corev1.PreemptionPolicy) *schedulingv1.PriorityClass {
	return &schedulingv1.PriorityClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Labels:      priorityClassManagedLabels(),
			Annotations: map[string]string{PriorityClassManagedAnnotation: PriorityClassManagedResource},
		},
		Value:            value,
		PreemptionPolicy: &policy,
		GlobalDefault:    false,
		Description:      "managed",
	}
}

func getPriorityClass(t *testing.T, cl *Client, name string) *schedulingv1.PriorityClass {
	t.Helper()
	pc, err := cl.Clientset().SchedulingV1().PriorityClasses().Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get priority class %s: %v", name, err)
	}
	return pc
}

func assertPriorityClassManaged(t *testing.T, pc *schedulingv1.PriorityClass) {
	t.Helper()
	for key, want := range priorityClassManagedLabels() {
		if got := pc.Labels[key]; got != want {
			t.Fatalf("managed label %s = %q, want %q on %#v", key, got, want, pc.Labels)
		}
	}
	if got := pc.Annotations[PriorityClassManagedAnnotation]; got != PriorityClassManagedResource {
		t.Fatalf("managed annotation = %q, want %q", got, PriorityClassManagedResource)
	}
}
