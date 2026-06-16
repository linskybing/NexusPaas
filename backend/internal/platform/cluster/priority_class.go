package cluster

import (
	"context"
	"fmt"
	"maps"
	"strings"

	corev1 "k8s.io/api/core/v1"
	schedulingv1 "k8s.io/api/scheduling/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
)

const (
	PriorityClassManagedByLabel = "app.kubernetes.io/managed-by"
	PriorityClassPartOfLabel    = "app.kubernetes.io/part-of"
	PriorityClassOwnerLabel     = "nexuspaas.io/owner"

	PriorityClassManagedByValue = "platform-backend"
	PriorityClassPartOfValue    = "platform"
	PriorityClassOwnerValue     = "scheduler-quota-service"

	PriorityClassManagedAnnotation = "nexuspaas.io/managed-resource"
	PriorityClassManagedResource   = "priority-class"
)

const (
	PriorityClassActionCreated   = "created"
	PriorityClassActionUpdated   = "updated"
	PriorityClassActionRecreated = "recreated"
	PriorityClassActionAdopted   = "adopted"
	PriorityClassActionUnchanged = "unchanged"
	PriorityClassActionInvalid   = "invalid"
	PriorityClassActionConflict  = "conflict"
	PriorityClassActionFailed    = "failed"
	PriorityClassActionDegraded  = "degraded"
)

type PriorityClassDefinition struct {
	Name             string
	Value            int32
	PreemptionPolicy corev1.PreemptionPolicy
	Description      string
	Labels           map[string]string
	Annotations      map[string]string
}

type PriorityClassSyncResult struct {
	Name   string `json:"name"`
	Action string `json:"action"`
	Reason string `json:"reason,omitempty"`
	Error  string `json:"error,omitempty"`
}

type PriorityClassSyncSummary struct {
	SourceCount int                       `json:"source_count"`
	Created     int                       `json:"created_count"`
	Updated     int                       `json:"updated_count"`
	Recreated   int                       `json:"recreated_count"`
	Adopted     int                       `json:"adopted_count"`
	Unchanged   int                       `json:"unchanged_count"`
	Invalid     int                       `json:"invalid_count"`
	Conflict    int                       `json:"conflict_count"`
	Failed      int                       `json:"failed_count"`
	Degraded    bool                      `json:"degraded"`
	Results     []PriorityClassSyncResult `json:"results"`
}

func (s *PriorityClassSyncSummary) add(result PriorityClassSyncResult) {
	s.Results = append(s.Results, result)
	switch result.Action {
	case PriorityClassActionCreated:
		s.Created++
	case PriorityClassActionUpdated:
		s.Updated++
	case PriorityClassActionRecreated:
		s.Recreated++
	case PriorityClassActionAdopted:
		s.Adopted++
	case PriorityClassActionUnchanged:
		s.Unchanged++
	case PriorityClassActionInvalid:
		s.Invalid++
	case PriorityClassActionConflict:
		s.Conflict++
	case PriorityClassActionFailed:
		s.Failed++
	case PriorityClassActionDegraded:
		s.Degraded = true
	}
}

// SyncPriorityClasses reconciles scheduler-owned PriorityClass definitions into
// Kubernetes. Existing cluster-scoped objects are mutated only when they already
// carry the platform ownership markers or are safely adoptable with identical
// immutable fields and no conflicting owner markers.
func (c *Client) SyncPriorityClasses(ctx context.Context, defs []PriorityClassDefinition) PriorityClassSyncSummary {
	summary := PriorityClassSyncSummary{SourceCount: len(defs)}
	if (c == nil || c.clientset == nil) && len(defs) == 0 {
		summary.Degraded = true
		return summary
	}
	for _, def := range defs {
		summary.add(c.EnsurePriorityClassDefinition(ctx, def))
	}
	return summary
}

func (c *Client) EnsurePriorityClassDefinition(ctx context.Context, def PriorityClassDefinition) PriorityClassSyncResult {
	def = normalizePriorityClassDefinition(def)
	if err := validatePriorityClassDefinition(def); err != nil {
		return PriorityClassSyncResult{Name: def.Name, Action: PriorityClassActionInvalid, Reason: err.Error()}
	}
	if c == nil || c.clientset == nil {
		return PriorityClassSyncResult{Name: def.Name, Action: PriorityClassActionDegraded, Reason: "cluster client unavailable"}
	}

	desired := buildPriorityClass(def)
	existing, err := c.clientset.SchedulingV1().PriorityClasses().Get(ctx, def.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return c.createPriorityClass(ctx, def.Name, desired)
	}
	if err != nil {
		return failedPriorityClassResult(def.Name, "get", err)
	}

	managed, conflictReason := priorityClassManagedByPlatform(existing)
	if !managed {
		return c.adoptPriorityClass(ctx, def.Name, existing, desired, conflictReason)
	}
	return c.reconcileManagedPriorityClass(ctx, def.Name, existing, desired)
}

func (c *Client) createPriorityClass(ctx context.Context, name string, desired *schedulingv1.PriorityClass) PriorityClassSyncResult {
	if _, err := c.clientset.SchedulingV1().PriorityClasses().Create(ctx, desired, metav1.CreateOptions{}); err != nil {
		return failedPriorityClassResult(name, "create", err)
	}
	return PriorityClassSyncResult{Name: name, Action: PriorityClassActionCreated}
}

func (c *Client) adoptPriorityClass(ctx context.Context, name string, existing, desired *schedulingv1.PriorityClass, conflictReason string) PriorityClassSyncResult {
	if conflictReason != "" {
		return PriorityClassSyncResult{Name: name, Action: PriorityClassActionConflict, Reason: conflictReason}
	}
	if !priorityClassImmutableEqual(existing, desired) {
		return PriorityClassSyncResult{Name: name, Action: PriorityClassActionConflict, Reason: "unmanaged object has immutable drift"}
	}
	next := existing.DeepCopy()
	copyPriorityClassMutableFields(next, desired)
	if _, err := c.clientset.SchedulingV1().PriorityClasses().Update(ctx, next, metav1.UpdateOptions{}); err != nil {
		return failedPriorityClassResult(name, "adopt", err)
	}
	return PriorityClassSyncResult{Name: name, Action: PriorityClassActionAdopted}
}

func (c *Client) reconcileManagedPriorityClass(ctx context.Context, name string, existing, desired *schedulingv1.PriorityClass) PriorityClassSyncResult {
	if !priorityClassImmutableEqual(existing, desired) {
		return c.recreatePriorityClass(ctx, name, desired)
	}
	if !priorityClassMutableEqual(existing, desired) {
		return c.updatePriorityClass(ctx, name, existing, desired)
	}
	return PriorityClassSyncResult{Name: name, Action: PriorityClassActionUnchanged}
}

func (c *Client) recreatePriorityClass(ctx context.Context, name string, desired *schedulingv1.PriorityClass) PriorityClassSyncResult {
	if err := c.clientset.SchedulingV1().PriorityClasses().Delete(ctx, name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return failedPriorityClassResult(name, "delete", err)
	}
	if _, err := c.clientset.SchedulingV1().PriorityClasses().Create(ctx, desired, metav1.CreateOptions{}); err != nil {
		return failedPriorityClassResult(name, "recreate", err)
	}
	return PriorityClassSyncResult{Name: name, Action: PriorityClassActionRecreated}
}

func (c *Client) updatePriorityClass(ctx context.Context, name string, existing, desired *schedulingv1.PriorityClass) PriorityClassSyncResult {
	next := existing.DeepCopy()
	copyPriorityClassMutableFields(next, desired)
	if _, err := c.clientset.SchedulingV1().PriorityClasses().Update(ctx, next, metav1.UpdateOptions{}); err != nil {
		return failedPriorityClassResult(name, "update", err)
	}
	return PriorityClassSyncResult{Name: name, Action: PriorityClassActionUpdated}
}

func normalizePriorityClassDefinition(def PriorityClassDefinition) PriorityClassDefinition {
	def.Name = strings.TrimSpace(def.Name)
	def.Description = strings.TrimSpace(def.Description)
	if def.PreemptionPolicy == "" {
		def.PreemptionPolicy = corev1.PreemptLowerPriority
	}
	return def
}

func validatePriorityClassDefinition(def PriorityClassDefinition) error {
	if def.Name == "" {
		return fmt.Errorf("priority class name required")
	}
	if strings.HasPrefix(def.Name, "system-") {
		return fmt.Errorf("system priority classes are not managed")
	}
	if errs := validation.IsDNS1123Subdomain(def.Name); len(errs) > 0 {
		return fmt.Errorf("invalid priority class name")
	}
	if def.PreemptionPolicy != corev1.PreemptLowerPriority && def.PreemptionPolicy != corev1.PreemptNever {
		return fmt.Errorf("invalid preemption policy")
	}
	return nil
}

func buildPriorityClass(def PriorityClassDefinition) *schedulingv1.PriorityClass {
	labels := priorityClassManagedLabels()
	for key, value := range def.Labels {
		if strings.TrimSpace(key) != "" && strings.TrimSpace(value) != "" {
			labels[key] = value
		}
	}
	// Managed markers are authoritative and cannot be overridden by caller labels.
	maps.Copy(labels, priorityClassManagedLabels())

	annotations := map[string]string{PriorityClassManagedAnnotation: PriorityClassManagedResource}
	for key, value := range def.Annotations {
		if strings.TrimSpace(key) != "" && strings.TrimSpace(value) != "" {
			annotations[key] = value
		}
	}
	annotations[PriorityClassManagedAnnotation] = PriorityClassManagedResource

	return &schedulingv1.PriorityClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:        def.Name,
			Labels:      labels,
			Annotations: annotations,
		},
		Value:            def.Value,
		PreemptionPolicy: &def.PreemptionPolicy,
		GlobalDefault:    false,
		Description:      def.Description,
	}
}

func priorityClassManagedLabels() map[string]string {
	return map[string]string{
		PriorityClassManagedByLabel: PriorityClassManagedByValue,
		PriorityClassPartOfLabel:    PriorityClassPartOfValue,
		PriorityClassOwnerLabel:     PriorityClassOwnerValue,
	}
}

func priorityClassManagedByPlatform(pc *schedulingv1.PriorityClass) (bool, string) {
	labels := pc.GetLabels()
	required := priorityClassManagedLabels()
	allPresent := true
	for key, want := range required {
		got, ok := labels[key]
		if ok && got != want {
			return false, "conflicting ownership marker " + key
		}
		if !ok {
			allPresent = false
		}
	}
	if !allPresent {
		return false, ""
	}
	return true, ""
}

func priorityClassImmutableEqual(a, b *schedulingv1.PriorityClass) bool {
	return a.Value == b.Value && priorityClassPolicy(a) == priorityClassPolicy(b)
}

func priorityClassMutableEqual(a, b *schedulingv1.PriorityClass) bool {
	return a.GlobalDefault == b.GlobalDefault &&
		a.Description == b.Description &&
		maps.Equal(a.Labels, b.Labels) &&
		maps.Equal(a.Annotations, b.Annotations)
}

func copyPriorityClassMutableFields(dst, src *schedulingv1.PriorityClass) {
	dst.Labels = maps.Clone(src.Labels)
	dst.Annotations = maps.Clone(src.Annotations)
	dst.GlobalDefault = src.GlobalDefault
	dst.Description = src.Description
}

func priorityClassPolicy(pc *schedulingv1.PriorityClass) corev1.PreemptionPolicy {
	if pc.PreemptionPolicy == nil {
		return corev1.PreemptLowerPriority
	}
	return *pc.PreemptionPolicy
}

func failedPriorityClassResult(name, operation string, err error) PriorityClassSyncResult {
	return PriorityClassSyncResult{
		Name:   name,
		Action: PriorityClassActionFailed,
		Reason: operation + " priority class failed",
		Error:  err.Error(),
	}
}
