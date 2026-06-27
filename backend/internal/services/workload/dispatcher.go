package workload

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/platform/cluster"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	jobStatusSubmitted    = "submitted"
	jobStatusWaitingInfra = "waiting_infra"
	jobStatusQueued       = "queued"
	jobStatusRunning      = "running"
	jobStatusCompleted    = "completed"
	jobStatusFailed       = "failed"

	defaultDispatcherSchedulerName = "default-scheduler"
	dispatcherMaxJobsPerRun        = 32
	volcanoSchedulerName           = "volcano"
	dispatcherRetryMaxAttempts     = 20
	dispatcherRetryBaseDelay       = 30 * time.Second
	dispatcherRetryMaxDelay        = 10 * time.Minute
	dispatcherMarshalResourceError = "marshal resource %s: %w"
	volcanoQueueAnnotationKey      = "volcano.sh/queue-name"
	volcanoGroupAnnotationKey      = "scheduling.volcano.sh/group-name"
	schedulingGroupAnnotationKey   = "scheduling.k8s.io/group-name"
	volcanoPodGroupLabelKey        = "volcano.sh/podgroup"
	platformJobQueueLabelKey       = "platform-go/job-queue"
	platformQueueNameLabelKey      = "platform-go/queue-name"
	platformPreemptibleLabelKey    = "platform-go/preemptible"
)

type dispatchResource struct {
	Name string
	Kind string
	Raw  []byte
}

type dispatchManifest struct {
	Raw      []byte
	Fallback []dispatchManifest
}

type dispatchCandidate struct {
	record contracts.Record[map[string]any]
	dueAt  time.Time
}

func dispatchSubmittedWorkloads(ctx context.Context, cl *cluster.Client, store platform.RecordStore, now time.Time) error {
	return dispatchSubmittedWorkloadsWithStorageMountClient(ctx, cl, store, nil, now)
}

func dispatchSubmittedWorkloadsWithStorageMountClient(ctx context.Context, cl *cluster.Client, store platform.RecordStore, storageMounts storageMountPlanResolver, now time.Time) error {
	return dispatchSubmittedWorkloadsWithStorageClients(ctx, cl, store, storageMounts, nil, now)
}

func dispatchSubmittedWorkloadsWithStorageClients(ctx context.Context, cl *cluster.Client, store platform.RecordStore, storageMounts storageMountPlanResolver, dataPlanes dataPlanePlanResolver, now time.Time) error {
	jobs := jobRepositoryFromStore(store)
	if jobs == nil {
		return nil
	}
	candidates := jobs.ListDispatchCandidates(ctx, now)
	if len(candidates) > dispatcherMaxJobsPerRun {
		candidates = candidates[:dispatcherMaxJobsPerRun]
	}
	for _, candidate := range candidates {
		dispatchJob(ctx, cl, store, storageMounts, dataPlanes, candidate.record, now)
	}
	return nil
}

func dispatchCandidates(ctx context.Context, store platform.RecordStore, now time.Time) []dispatchCandidate {
	jobs := jobRepositoryFromStore(store)
	if jobs == nil {
		return nil
	}
	return jobs.ListDispatchCandidates(ctx, now)
}

func dispatchJob(ctx context.Context, cl *cluster.Client, store platform.RecordStore, storageMounts storageMountPlanResolver, dataPlanes dataPlanePlanResolver, record contracts.Record[map[string]any], now time.Time) {
	if cl == nil {
		deferDispatchForInfrastructure(ctx, store, record, now, cluster.ErrUnavailable)
		return
	}
	namespace := shared.TextValue(record.Data, "namespace", "Namespace")
	if namespace == "" {
		failDispatchedJob(ctx, store, record.ID, "namespace is required")
		return
	}
	resources, err := dispatchResources(record.Data)
	if err != nil {
		failDispatchedJob(ctx, store, record.ID, err.Error())
		return
	}
	if len(resources) == 0 {
		failDispatchedJob(ctx, store, record.ID, "no workload resources found")
		return
	}
	if err := cl.EnsureNamespace(ctx, namespace); err != nil {
		deferDispatchForInfrastructure(ctx, store, record, now, err)
		return
	}
	storagePlan, err := resolveDispatchStorageMountPlan(ctx, storageMounts, record.Data, namespace)
	if err != nil {
		handleDispatchCreateError(ctx, store, record, now, err)
		return
	}
	if err := ensureDispatchPVCMounts(ctx, cl, storagePlan, namespace); err != nil {
		handleDispatchCreateError(ctx, store, record, now, err)
		return
	}
	dataPlanePlan, err := resolveDispatchDataPlanePlan(ctx, dataPlanes, record.Data, namespace)
	if err != nil {
		handleDispatchCreateError(ctx, store, record, now, err)
		return
	}
	if err := ensureDispatchDataPlanePVCMounts(ctx, cl, dataPlanePlan, namespace); err != nil {
		handleDispatchCreateError(ctx, store, record, now, err)
		return
	}
	manifests, err := prepareDispatchManifests(record.Data, resources, namespace, cl, storagePlan, dataPlanePlan)
	if err != nil {
		rollbackDispatch(ctx, cl, namespace, record.ID)
		failDispatchedJob(ctx, store, record.ID, err.Error())
		return
	}
	created := make([]map[string]any, 0, len(manifests))
	for _, manifest := range manifests {
		objects, err := createDispatchManifest(ctx, cl, namespace, manifest)
		if err != nil {
			rollbackDispatch(ctx, cl, namespace, record.ID)
			handleDispatchCreateError(ctx, store, record, now, err)
			return
		}
		created = append(created, objects...)
	}
	markDispatchedJobRunning(ctx, store, record.ID, now, created)
}

func createDispatchManifest(
	ctx context.Context,
	cl *cluster.Client,
	namespace string,
	manifest dispatchManifest,
) ([]map[string]any, error) {
	obj, err := cl.CreateByJSON(ctx, namespace, manifest.Raw)
	if err == nil {
		return []map[string]any{createdDispatchResource(obj)}, nil
	}
	if len(manifest.Fallback) == 0 {
		return nil, err
	}
	slog.Warn("dispatcher: primary manifest create failed, using fallback", "error", err)
	created := make([]map[string]any, 0, len(manifest.Fallback))
	for _, fallback := range manifest.Fallback {
		objects, fallbackErr := createDispatchManifest(ctx, cl, namespace, fallback)
		if fallbackErr != nil {
			return created, fallbackErr
		}
		created = append(created, objects...)
	}
	return created, nil
}

func createdDispatchResource(obj cluster.CreatedObject) map[string]any {
	return map[string]any{"kind": obj.Kind, "namespace": obj.Namespace, "name": obj.Name}
}

func prepareDispatchManifests(job map[string]any, resources []dispatchResource, namespace string, cl *cluster.Client, storagePlan storageMountPlan, dataPlanePlan dataPlanePlan) ([]dispatchManifest, error) {
	resources, err := prepareStorageMountDispatchResources(storagePlan, resources)
	if err != nil {
		return nil, err
	}
	resources, err = prepareDataPlaneDispatchResources(dataPlanePlan, resources)
	if err != nil {
		return nil, err
	}
	resources, err = preparePlacementDispatchResources(job, resources)
	if err != nil {
		return nil, err
	}
	resources, err = prepareAcceleratorDispatchResources(job, resources)
	if err != nil {
		return nil, err
	}
	resources, err = prepareNetworkDispatchResources(job, resources)
	if err != nil {
		return nil, err
	}
	resources, err = prepareStreamingDispatchResources(job, resources)
	if err != nil {
		return nil, err
	}
	prefix, resources, err := prepareDRADispatchManifests(job, resources, namespace)
	if err != nil {
		return nil, err
	}
	if shouldSynthesizeVolcano(job, cl) {
		manifests, err := prepareVolcanoDispatchManifests(job, resources, namespace)
		if err != nil {
			return nil, err
		}
		return append(prefix, manifests...), nil
	}
	manifests := make([]dispatchManifest, 0, len(prefix)+len(resources))
	manifests = append(manifests, prefix...)
	for _, resource := range resources {
		raw, err := prepareDispatchManifest(job, resource, namespace)
		if err != nil {
			return nil, err
		}
		manifests = append(manifests, dispatchManifest{Raw: raw})
	}
	return manifests, nil
}

func dispatchResources(data map[string]any) ([]dispatchResource, error) {
	rawResources, ok := firstPresent(data, "resources", "Resources")
	if !ok {
		if payload, ok := firstPresent(data, "submission_payload", "submissionPayload", "SubmissionPayload"); ok {
			if payloadMap, ok := payload.(map[string]any); ok {
				rawResources, _ = firstPresent(payloadMap, "resources", "Resources")
			}
		}
	}
	items := resourceItems(rawResources)
	out := make([]dispatchResource, 0, len(items))
	for _, item := range items {
		raw, err := dispatchResourceRaw(item)
		if err != nil {
			return nil, err
		}
		if len(raw) == 0 {
			continue
		}
		kind := shared.FirstNonEmpty(shared.TextValue(item, "kind", "Kind"), dispatchResourceKind(raw))
		if strings.EqualFold(kind, "Secret") {
			return nil, fmt.Errorf("raw Kubernetes Secret resources are rejected; use the platform secret API or an approved ExternalSecret profile")
		}
		out = append(out, dispatchResource{
			Name: shared.TextValue(item, "name", "Name"),
			Kind: kind,
			Raw:  raw,
		})
	}
	return out, nil
}

func dispatchResourceKind(raw []byte) string {
	var obj map[string]any
	if json.Unmarshal(raw, &obj) != nil {
		return ""
	}
	return shared.TextValue(obj, "kind", "Kind")
}

func resourceItems(value any) []map[string]any {
	switch typed := value.(type) {
	case []map[string]any:
		return append([]map[string]any{}, typed...)
	case []any:
		out := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if data, ok := item.(map[string]any); ok {
				out = append(out, data)
			}
		}
		return out
	case string:
		var decoded []map[string]any
		if json.Unmarshal([]byte(strings.TrimSpace(typed)), &decoded) == nil {
			return decoded
		}
	}
	return nil
}

func dispatchResourceRaw(data map[string]any) ([]byte, error) {
	for _, key := range []string{"json_data", "jsonData", "json", "object", "manifest"} {
		raw, ok := data[key]
		if !ok || raw == nil {
			continue
		}
		if text, ok := raw.(string); ok {
			return []byte(strings.TrimSpace(text)), nil
		}
		body, err := json.Marshal(raw)
		if err != nil {
			return nil, fmt.Errorf(dispatcherMarshalResourceError, shared.TextValue(data, "name", "Name"), err)
		}
		return body, nil
	}
	if shared.TextValue(data, "apiVersion") != "" || shared.TextValue(data, "kind", "Kind") != "" {
		body, err := json.Marshal(data)
		if err != nil {
			return nil, fmt.Errorf(dispatcherMarshalResourceError, shared.TextValue(data, "name", "Name"), err)
		}
		return body, nil
	}
	return nil, nil
}

func prepareDispatchManifest(job map[string]any, resource dispatchResource, namespace string) ([]byte, error) {
	return prepareDispatchManifestWithGroup(job, resource, namespace, "")
}

func prepareDispatchManifestWithGroup(job map[string]any, resource dispatchResource, namespace, groupName string) ([]byte, error) {
	u, err := dispatchObject(resource)
	if err != nil {
		return nil, err
	}
	if u.GetKind() != "Namespace" {
		u.SetNamespace(namespace)
	}
	labels := dispatchLabels(job)
	mergeObjectLabels(u, labels)
	mergePodTemplateLabels(u, labels)
	applyDispatchScheduling(u, job, groupName)
	applyDispatchRuntimeLimit(u, job)
	raw, err := json.Marshal(u.Object)
	if err != nil {
		return nil, fmt.Errorf(dispatcherMarshalResourceError, u.GetName(), err)
	}
	return raw, nil
}

func dispatchObject(resource dispatchResource) (*unstructured.Unstructured, error) {
	var obj map[string]any
	if err := json.Unmarshal(resource.Raw, &obj); err != nil {
		return nil, fmt.Errorf("parse resource %s: %w", resource.Name, err)
	}
	u := &unstructured.Unstructured{Object: obj}
	if u.GetKind() == "" && resource.Kind != "" {
		u.SetKind(resource.Kind)
	}
	if u.GetKind() == "" {
		return nil, fmt.Errorf("resource %s kind is required", resource.Name)
	}
	if u.GetName() == "" {
		if resource.Name == "" {
			return nil, fmt.Errorf("resource name is required")
		}
		u.SetName(resource.Name)
	}
	return u, nil
}

func dispatchLabels(job map[string]any) map[string]string {
	labels := map[string]string{}
	if jobID := shared.TextValue(job, "job_id", "jobId", "id"); jobID != "" {
		labels[cluster.LabelJobID] = jobID
	}
	if projectID := shared.TextValue(job, "project_id", "projectId"); projectID != "" {
		labels[cluster.LabelProjectID] = projectID
	}
	if userID := shared.TextValue(job, "user_id", "userId"); userID != "" {
		labels[cluster.LabelUserID] = userID
	}
	if gpu := shared.IntValue(job, "gpu_count", "gpuCount"); gpu > 0 {
		labels[cluster.LabelGPUCount] = strconv.Itoa(gpu)
	}
	if workloadJobPreemptible(job) {
		labels[platformPreemptibleLabelKey] = "true"
	}
	if seconds, ok := dispatchRuntimeLimitSeconds(job); ok {
		labels[cluster.RuntimeLimitSecondsKey] = strconv.FormatInt(seconds, 10)
	}
	return labels
}

func applyDispatchRuntimeLimit(u *unstructured.Unstructured, job map[string]any) {
	seconds, ok := dispatchRuntimeLimitSeconds(job)
	if !ok {
		return
	}
	switch strings.ToLower(u.GetKind()) {
	case "pod":
		capActiveDeadlineSeconds(u, seconds, "spec", "activeDeadlineSeconds")
	case "job":
		if strings.EqualFold(u.GetAPIVersion(), "batch/v1") {
			capActiveDeadlineSeconds(u, seconds, "spec", "activeDeadlineSeconds")
		}
	case "deployment":
		// Deployments reconcile Pods through ReplicaSets; runtime cleanup deletes
		// the labeled Deployment controller instead of relying on Pod deadlines.
		return
	}
}

func dispatchRuntimeLimitSeconds(job map[string]any) (int64, bool) {
	seconds := shared.IntValue(job, "runtime_limit_seconds", "runtimeLimitSeconds", "max_runtime_seconds", "maxRuntimeSeconds")
	if seconds <= 0 {
		return 0, false
	}
	return int64(seconds), true
}

func capActiveDeadlineSeconds(u *unstructured.Unstructured, seconds int64, fields ...string) {
	if current, found := nestedPositiveInt64(u.Object, fields...); found && current > 0 && current <= seconds {
		return
	}
	_ = unstructured.SetNestedField(u.Object, seconds, fields...)
}

func nestedPositiveInt64(obj map[string]any, fields ...string) (int64, bool) {
	value, found, _ := unstructured.NestedFieldNoCopy(obj, fields...)
	if !found {
		return 0, false
	}
	switch typed := value.(type) {
	case int64:
		return typed, typed > 0
	case int:
		return int64(typed), typed > 0
	case float64:
		return int64(typed), typed > 0
	case json.Number:
		n, err := typed.Int64()
		return n, err == nil && n > 0
	default:
		return 0, false
	}
}

func mergeObjectLabels(u *unstructured.Unstructured, labels map[string]string) {
	if len(labels) == 0 {
		return
	}
	current := u.GetLabels()
	if current == nil {
		current = map[string]string{}
	}
	for key, value := range labels {
		current[key] = value
	}
	u.SetLabels(current)
}

func mergePodTemplateLabels(u *unstructured.Unstructured, labels map[string]string) {
	if len(labels) == 0 {
		return
	}
	switch strings.ToLower(u.GetKind()) {
	case "deployment":
		existing, _, _ := unstructured.NestedStringMap(u.Object, "spec", "template", "metadata", "labels")
		if existing == nil {
			existing = map[string]string{}
		}
		for key, value := range labels {
			existing[key] = value
		}
		_ = unstructured.SetNestedStringMap(u.Object, existing, "spec", "template", "metadata", "labels")
	case "job":
		if isVolcanoVCJob(u) {
			mergeVCJobTaskTemplateLabels(u, labels)
			return
		}
		existing, _, _ := unstructured.NestedStringMap(u.Object, "spec", "template", "metadata", "labels")
		if existing == nil {
			existing = map[string]string{}
		}
		for key, value := range labels {
			existing[key] = value
		}
		_ = unstructured.SetNestedStringMap(u.Object, existing, "spec", "template", "metadata", "labels")
	}
}

func applyDispatchScheduling(u *unstructured.Unstructured, job map[string]any, groupName string) {
	queue := shared.TextValue(job, "queue_name", "queueName")
	scheduler := dispatchSchedulerName(u, job)
	priorityClass := priorityClassForJob(job)
	if isVolcanoVCJob(u) {
		applyVCJobDispatchScheduling(u, queue, scheduler, priorityClass)
		return
	}
	if isVolcanoPodGroup(u) {
		applyPodGroupDispatchScheduling(u, queue, priorityClass)
		return
	}
	switch strings.ToLower(u.GetKind()) {
	case "pod":
		if scheduler != "" {
			_ = unstructured.SetNestedField(u.Object, scheduler, "spec", "schedulerName")
		}
		if priorityClass != "" {
			_ = unstructured.SetNestedField(u.Object, priorityClass, "spec", "priorityClassName")
		}
		setAnnotation(u, queue)
		setPodGroupObjectMetadata(u, groupName)
	case "deployment", "job":
		if scheduler != "" {
			_ = unstructured.SetNestedField(u.Object, scheduler, "spec", "template", "spec", "schedulerName")
		}
		if priorityClass != "" {
			_ = unstructured.SetNestedField(u.Object, priorityClass, "spec", "template", "spec", "priorityClassName")
		}
		setTemplateAnnotation(u, queue)
		setPodGroupTemplateMetadata(u, groupName)
	}
}

func dispatchSchedulerName(u *unstructured.Unstructured, job map[string]any) string {
	if jobRequestsDRA(job) {
		return defaultDispatcherSchedulerName
	}
	configured := shared.TextValue(job, "scheduler_name", "schedulerName")
	if configured != "" {
		return configured
	}
	if isVolcanoVCJob(u) {
		return volcanoSchedulerName
	}
	return defaultDispatcherSchedulerName
}

func isVolcanoVCJob(u *unstructured.Unstructured) bool {
	return strings.EqualFold(u.GetKind(), "Job") && strings.HasPrefix(strings.ToLower(u.GetAPIVersion()), "batch.volcano.sh/")
}

func isVolcanoPodGroup(u *unstructured.Unstructured) bool {
	return strings.EqualFold(u.GetKind(), "PodGroup") && strings.HasPrefix(strings.ToLower(u.GetAPIVersion()), "scheduling.volcano.sh/")
}

func applyVCJobDispatchScheduling(u *unstructured.Unstructured, queue, scheduler, priorityClass string) {
	if queue != "" {
		_ = unstructured.SetNestedField(u.Object, queue, "spec", "queue")
		setVCJobTaskTemplateAnnotation(u, queue)
	}
	if scheduler != "" {
		_ = unstructured.SetNestedField(u.Object, scheduler, "spec", "schedulerName")
	}
	if priorityClass != "" {
		_ = unstructured.SetNestedField(u.Object, priorityClass, "spec", "priorityClassName")
		setVCJobTaskTemplateSpecField(u, priorityClass, "priorityClassName")
	}
}

func applyPodGroupDispatchScheduling(u *unstructured.Unstructured, queue, priorityClass string) {
	if queue != "" {
		_ = unstructured.SetNestedField(u.Object, queue, "spec", "queue")
	}
	if priorityClass != "" {
		_ = unstructured.SetNestedField(u.Object, priorityClass, "spec", "priorityClassName")
	}
}

func mergeVCJobTaskTemplateLabels(u *unstructured.Unstructured, labels map[string]string) {
	updateVCJobTasks(u, func(task map[string]any) {
		existing, _, _ := unstructured.NestedStringMap(task, "template", "metadata", "labels")
		if existing == nil {
			existing = map[string]string{}
		}
		for key, value := range labels {
			existing[key] = value
		}
		_ = unstructured.SetNestedStringMap(task, existing, "template", "metadata", "labels")
	})
}

func setVCJobTaskTemplateAnnotation(u *unstructured.Unstructured, queue string) {
	updateVCJobTasks(u, func(task map[string]any) {
		existing, _, _ := unstructured.NestedStringMap(task, "template", "metadata", "annotations")
		if existing == nil {
			existing = map[string]string{}
		}
		existing[volcanoQueueAnnotationKey] = queue
		_ = unstructured.SetNestedStringMap(task, existing, "template", "metadata", "annotations")
	})
}

func setVCJobTaskTemplateSpecField(u *unstructured.Unstructured, value, field string) {
	updateVCJobTasks(u, func(task map[string]any) {
		_ = unstructured.SetNestedField(task, value, "template", "spec", field)
	})
}

func updateVCJobTasks(u *unstructured.Unstructured, update func(map[string]any)) {
	tasks, found, _ := unstructured.NestedSlice(u.Object, "spec", "tasks")
	if !found {
		return
	}
	changed := false
	for i := range tasks {
		task, ok := tasks[i].(map[string]any)
		if !ok {
			continue
		}
		update(task)
		tasks[i] = task
		changed = true
	}
	if changed {
		_ = unstructured.SetNestedSlice(u.Object, tasks, "spec", "tasks")
	}
}

func setPodGroupObjectMetadata(u *unstructured.Unstructured, groupName string) {
	groupName = strings.TrimSpace(groupName)
	if groupName == "" {
		return
	}
	annotations := u.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[volcanoGroupAnnotationKey] = groupName
	annotations[schedulingGroupAnnotationKey] = groupName
	u.SetAnnotations(annotations)
	labels := u.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels[volcanoPodGroupLabelKey] = groupName
	u.SetLabels(labels)
}

func setPodGroupTemplateMetadata(u *unstructured.Unstructured, groupName string) {
	groupName = strings.TrimSpace(groupName)
	if groupName == "" {
		return
	}
	annotations, _, _ := unstructured.NestedStringMap(u.Object, "spec", "template", "metadata", "annotations")
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[volcanoGroupAnnotationKey] = groupName
	annotations[schedulingGroupAnnotationKey] = groupName
	_ = unstructured.SetNestedStringMap(u.Object, annotations, "spec", "template", "metadata", "annotations")
	labels, _, _ := unstructured.NestedStringMap(u.Object, "spec", "template", "metadata", "labels")
	if labels == nil {
		labels = map[string]string{}
	}
	labels[volcanoPodGroupLabelKey] = groupName
	_ = unstructured.SetNestedStringMap(u.Object, labels, "spec", "template", "metadata", "labels")
}

func setAnnotation(u *unstructured.Unstructured, queue string) {
	if queue == "" {
		return
	}
	annotations := u.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[volcanoQueueAnnotationKey] = queue
	u.SetAnnotations(annotations)
}

func setTemplateAnnotation(u *unstructured.Unstructured, queue string) {
	if queue == "" {
		return
	}
	existing, _, _ := unstructured.NestedStringMap(u.Object, "spec", "template", "metadata", "annotations")
	if existing == nil {
		existing = map[string]string{}
	}
	existing[volcanoQueueAnnotationKey] = queue
	_ = unstructured.SetNestedStringMap(u.Object, existing, "spec", "template", "metadata", "annotations")
}

func priorityClassForJob(job map[string]any) string {
	priority := jobPriority(job)
	switch {
	case priority >= 1000000:
		return "platform-critical"
	case priority >= 500000:
		return "platform-interactive-high"
	case priority >= 100000:
		return "platform-interactive"
	case priority >= 10000:
		return "platform-batch-high"
	case priority >= 1000:
		return "platform-batch-medium"
	default:
		return "platform-batch-low"
	}
}

func markDispatchedJobRunning(ctx context.Context, store platform.RecordStore, id string, now time.Time, objects []map[string]any) {
	jobs := jobRepositoryFromStore(store)
	if jobs == nil || !jobs.MarkDispatchRunning(ctx, id, jobDispatchRunningUpdate{At: now, CreatedResources: objects}) {
		slog.Warn("dispatcher: failed to mark job running", "job_id", id)
	}
}

func failDispatchedJob(ctx context.Context, store platform.RecordStore, id, reason string) {
	jobs := jobRepositoryFromStore(store)
	if jobs == nil || !jobs.MarkDispatchFailed(ctx, id, jobDispatchFailedUpdate{Reason: reason, CompletedAt: time.Now().UTC()}) {
		slog.Warn("dispatcher: failed to mark job failed", "job_id", id)
	}
}

func handleDispatchCreateError(ctx context.Context, store platform.RecordStore, record contracts.Record[map[string]any], now time.Time, err error) {
	if dispatchPermanentError(err) {
		failDispatchedJob(ctx, store, record.ID, err.Error())
		return
	}
	deferDispatchForInfrastructure(ctx, store, record, now, err)
}

func deferDispatchForInfrastructure(ctx context.Context, store platform.RecordStore, record contracts.Record[map[string]any], now time.Time, err error) {
	nextRetryCount := shared.IntValue(record.Data, "retry_count", "retryCount") + 1
	if nextRetryCount >= dispatcherRetryMaxAttempts {
		failDispatchedJob(ctx, store, record.ID, fmt.Sprintf("infrastructure recovery retry limit reached after %d attempts: %v", nextRetryCount, err))
		return
	}
	delay := dispatcherBackoff(nextRetryCount)
	nextRetryAt := now.Add(delay).UTC()
	reason := fmt.Sprintf("waiting for workload infrastructure recovery (attempt %d/%d, retry at %s): %v",
		nextRetryCount, dispatcherRetryMaxAttempts, nextRetryAt.Format(time.RFC3339), err)
	jobs := jobRepositoryFromStore(store)
	if jobs == nil || !jobs.DeferForInfrastructureRecovery(ctx, record.ID, jobInfrastructureRecoveryUpdate{
		RetryCount:  nextRetryCount,
		NextRetryAt: nextRetryAt,
		Reason:      reason,
	}) {
		slog.Warn("dispatcher: failed to defer job for infrastructure recovery", "job_id", record.ID)
	}
}

func dispatcherBackoff(attempt int) time.Duration {
	if attempt <= 1 {
		return dispatcherRetryBaseDelay
	}
	delay := dispatcherRetryBaseDelay
	for i := 1; i < attempt && delay < dispatcherRetryMaxDelay; i++ {
		delay *= 2
	}
	if delay > dispatcherRetryMaxDelay {
		return dispatcherRetryMaxDelay
	}
	return delay
}

func rollbackDispatch(ctx context.Context, cl *cluster.Client, namespace, jobID string) {
	if cl == nil || namespace == "" || jobID == "" {
		return
	}
	if _, err := cl.CleanupJobResources(ctx, namespace, jobID); err != nil {
		slog.Warn("dispatcher: cleanup after dispatch failure failed", "job_id", jobID, "namespace", namespace, "error", err)
	}
}

func dispatchPermanentError(err error) bool {
	return errors.Is(err, cluster.ErrInvalidManifest) || errors.Is(err, cluster.ErrUnsupportedKind)
}

func nextRetryDue(data map[string]any, now time.Time) (time.Time, bool) {
	value, ok := firstPresent(data, "next_retry_at", "nextRetryAt", "NextRetryAt")
	if !ok || value == nil {
		return jobCreatedAt(data, now), true
	}
	switch typed := value.(type) {
	case time.Time:
		return typed, !typed.After(now)
	case string:
		if strings.TrimSpace(typed) == "" {
			return jobCreatedAt(data, now), true
		}
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(typed))
		return parsed, err == nil && !parsed.After(now)
	default:
		return jobCreatedAt(data, now), true
	}
}

func jobPriority(data map[string]any) int {
	return shared.IntValue(data, "priority_value", "priorityValue", "priority", "Priority")
}

func registerJobDispatcher(app *platform.App) {
	storageMounts, err := newStorageMountPlanClient(app)
	if err != nil {
		storageMounts = func(context.Context, string, storageMountPlanRequest) (storageMountPlan, error) {
			return storageMountPlan{}, err
		}
	}
	dataPlanes, err := newDataPlanePlanClient(app)
	if err != nil {
		dataPlanes = func(context.Context, string, dataPlanePlanRequest) (dataPlanePlan, error) {
			return dataPlanePlan{}, err
		}
	}
	app.RegisterMaintenanceTaskForService(serviceName, "workload-dispatcher", func(ctx context.Context) error {
		return dispatchSubmittedWorkloadsWithStorageClients(ctx, app.Cluster, app.Store, storageMounts, dataPlanes, time.Now().UTC())
	})
}
