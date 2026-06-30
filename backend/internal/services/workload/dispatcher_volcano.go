package workload

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"github.com/linskybing/nexuspaas/backend/internal/platform/cluster"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	volcanoVCJobAPIVersion    = "batch.volcano.sh/v1alpha1"
	volcanoVCJobKind          = "Job"
	volcanoPodGroupAPIVersion = "scheduling.volcano.sh/v1beta1"
	volcanoPodGroupKind       = "PodGroup"
)

type volcanoTaskTemplate struct {
	sourceName string
	replicas   int64
	template   map[string]any
}

type volcanoDispatchParts struct {
	extras []dispatchManifest
	native []dispatchResource
	tasks  []volcanoTaskTemplate
}

type fallbackPodRequest struct {
	job       map[string]any
	namespace string
	groupName string
	labels    map[string]string
	template  volcanoTaskTemplate
	taskIndex int
	replica   int64
	replicas  int64
}

func shouldSynthesizeVolcano(job map[string]any, cl *cluster.Client) bool {
	return cl != nil &&
		cl.DynamicClient() != nil &&
		!jobRequestsDRA(job) &&
		strings.EqualFold(shared.TextValue(job, "scheduler_name", "schedulerName"), volcanoSchedulerName)
}

func prepareVolcanoDispatchManifests(job map[string]any, resources []dispatchResource, namespace string) ([]dispatchManifest, error) {
	labels := dispatchLabels(job)
	parts, err := collectVolcanoDispatchParts(job, resources, namespace, labels)
	if err != nil {
		return nil, err
	}
	groupName := volcanoGroupName(job, resources, parts.tasks, parts.native)
	return assembleVolcanoDispatchManifests(job, namespace, groupName, labels, parts)
}

func collectVolcanoDispatchParts(
	job map[string]any,
	resources []dispatchResource,
	namespace string,
	labels map[string]string,
) (volcanoDispatchParts, error) {
	parts := volcanoDispatchParts{
		extras: make([]dispatchManifest, 0, len(resources)),
		native: make([]dispatchResource, 0, len(resources)),
		tasks:  make([]volcanoTaskTemplate, 0, len(resources)),
	}
	for _, resource := range resources {
		if err := parts.addResource(job, namespace, labels, resource); err != nil {
			return volcanoDispatchParts{}, err
		}
	}
	return parts, nil
}

func (p *volcanoDispatchParts) addResource(
	job map[string]any,
	namespace string,
	labels map[string]string,
	resource dispatchResource,
) error {
	u, err := dispatchObject(resource)
	if err != nil {
		return err
	}
	switch strings.ToLower(u.GetKind()) {
	case "job":
		return p.addJobResource(job, namespace, labels, resource, u)
	case "pod", "deployment":
		p.native = append(p.native, resource)
		return nil
	default:
		raw, err := prepareDispatchManifest(job, resource, namespace)
		if err != nil {
			return err
		}
		p.extras = append(p.extras, dispatchManifest{Raw: raw})
		return nil
	}
}

func (p *volcanoDispatchParts) addJobResource(
	job map[string]any,
	namespace string,
	labels map[string]string,
	resource dispatchResource,
	u *unstructured.Unstructured,
) error {
	if isVolcanoVCJob(u) {
		raw, err := prepareDispatchManifest(job, resource, namespace)
		if err != nil {
			return err
		}
		p.extras = append(p.extras, dispatchManifest{Raw: raw})
		return nil
	}
	task, err := volcanoTaskFromBatchJob(job, resource, u, labels)
	if err != nil {
		return err
	}
	p.tasks = append(p.tasks, task)
	return nil
}

func assembleVolcanoDispatchManifests(
	job map[string]any,
	namespace string,
	groupName string,
	labels map[string]string,
	parts volcanoDispatchParts,
) ([]dispatchManifest, error) {
	manifests := append([]dispatchManifest{}, parts.extras...)
	if len(parts.tasks) > 0 {
		return appendSynthesizedVCJob(job, namespace, groupName, labels, parts, manifests)
	}
	if len(parts.native) > 0 {
		return appendSynthesizedPodGroup(job, namespace, groupName, labels, parts, manifests)
	}
	return manifests, nil
}

func appendSynthesizedVCJob(
	job map[string]any,
	namespace string,
	groupName string,
	labels map[string]string,
	parts volcanoDispatchParts,
	manifests []dispatchManifest,
) ([]dispatchManifest, error) {
	raw, err := synthesizedVCJob(job, namespace, groupName, labels, parts.tasks)
	if err != nil {
		return nil, err
	}
	fallback, err := synthesizedVCJobFallbackManifests(job, namespace, groupName, labels, parts.tasks)
	if err != nil {
		return nil, err
	}
	manifests = append(manifests, dispatchManifest{Raw: raw, Fallback: fallback})
	return appendNativeWithPodGroup(job, namespace, groupName, parts.native, manifests)
}

func appendSynthesizedPodGroup(
	job map[string]any,
	namespace string,
	groupName string,
	labels map[string]string,
	parts volcanoDispatchParts,
	manifests []dispatchManifest,
) ([]dispatchManifest, error) {
	raw, err := synthesizedPodGroup(job, namespace, groupName, labels, parts.native)
	if err != nil {
		return nil, err
	}
	manifests = append(manifests, dispatchManifest{Raw: raw})
	return appendNativeWithPodGroup(job, namespace, groupName, parts.native, manifests)
}

func appendNativeWithPodGroup(
	job map[string]any,
	namespace string,
	groupName string,
	native []dispatchResource,
	manifests []dispatchManifest,
) ([]dispatchManifest, error) {
	for _, resource := range native {
		raw, err := prepareDispatchManifestWithGroup(job, resource, namespace, groupName)
		if err != nil {
			return nil, err
		}
		manifests = append(manifests, dispatchManifest{Raw: raw})
	}
	return manifests, nil
}

func volcanoTaskFromBatchJob(
	job map[string]any,
	resource dispatchResource,
	u *unstructured.Unstructured,
	labels map[string]string,
) (volcanoTaskTemplate, error) {
	template, found, _ := unstructured.NestedMap(u.Object, "spec", "template")
	if !found {
		return volcanoTaskTemplate{}, fmt.Errorf("%w: job %s spec.template is required", cluster.ErrInvalidManifest, u.GetName())
	}
	if !podTemplateHasContainers(template) {
		return volcanoTaskTemplate{}, fmt.Errorf("%w: job %s has empty pod template containers", cluster.ErrInvalidManifest, u.GetName())
	}
	mergeNestedLabels(template, labels, "metadata", "labels")
	queue := shared.TextValue(job, "queue_name", "queueName")
	if queue != "" {
		mergeNestedStringMap(template, map[string]string{volcanoQueueAnnotationKey: queue}, "metadata", "annotations")
	}
	_ = unstructured.SetNestedField(template, volcanoSchedulerName, "spec", "schedulerName")
	if priorityClass := priorityClassForJob(job); priorityClass != "" {
		_ = unstructured.SetNestedField(template, priorityClass, "spec", "priorityClassName")
	}
	podSpec, _, _ := unstructured.NestedMap(template, "spec")
	if socketPath, found := shared.RuntimeSocketHostPath(podSpec); found {
		return volcanoTaskTemplate{}, fmt.Errorf("%w: user workloads cannot mount container runtime socket %s", cluster.ErrInvalidManifest, socketPath)
	}
	setDispatchAutomountServiceAccountToken(template, []string{"spec"})
	return volcanoTaskTemplate{
		sourceName: shared.FirstNonEmpty(resource.Name, u.GetName()),
		replicas:   batchJobReplicas(u),
		template:   template,
	}, nil
}

func synthesizedVCJob(
	job map[string]any,
	namespace string,
	name string,
	labels map[string]string,
	templates []volcanoTaskTemplate,
) ([]byte, error) {
	tasks := make([]any, 0, len(templates))
	minAvailable := int64(0)
	for i, template := range templates {
		replicas := template.replicas
		if replicas < 1 {
			replicas = 1
		}
		minAvailable += replicas
		tasks = append(tasks, map[string]any{
			"name":         fmt.Sprintf("t%d", i),
			"replicas":     replicas,
			"minAvailable": replicas,
			"template":     template.template,
		})
	}
	if minAvailable < 1 {
		minAvailable = 1
	}
	spec := map[string]any{
		"minAvailable":      minAvailable,
		"schedulerName":     volcanoSchedulerName,
		"priorityClassName": priorityClassForJob(job),
		"tasks":             tasks,
		"plugins":           map[string]any{"env": []any{}},
	}
	if queue := shared.TextValue(job, "queue_name", "queueName"); queue != "" {
		spec["queue"] = queue
	}
	return json.Marshal(map[string]any{
		"apiVersion": volcanoVCJobAPIVersion,
		"kind":       volcanoVCJobKind,
		"metadata": map[string]any{
			"name":      name,
			"namespace": namespace,
			"labels":    labels,
		},
		"spec": spec,
	})
}

func synthesizedVCJobFallbackManifests(
	job map[string]any,
	namespace string,
	name string,
	labels map[string]string,
	templates []volcanoTaskTemplate,
) ([]dispatchManifest, error) {
	raw, err := synthesizedPodGroupForTasks(job, namespace, name, labels, templates)
	if err != nil {
		return nil, err
	}
	manifests := []dispatchManifest{{Raw: raw}}
	pods, err := fallbackPodsForTasks(job, namespace, name, labels, templates)
	if err != nil {
		return nil, err
	}
	return append(manifests, pods...), nil
}

func synthesizedPodGroup(
	job map[string]any,
	namespace string,
	name string,
	labels map[string]string,
	native []dispatchResource,
) ([]byte, error) {
	minMember := int64(0)
	for _, resource := range native {
		minMember += nativeReplicaCount(resource)
	}
	return synthesizedPodGroupWithMinMember(job, namespace, name, labels, minMember)
}

func synthesizedPodGroupForTasks(
	job map[string]any,
	namespace string,
	name string,
	labels map[string]string,
	templates []volcanoTaskTemplate,
) ([]byte, error) {
	minMember := int64(0)
	for _, template := range templates {
		minMember += taskReplicaCount(template)
	}
	return synthesizedPodGroupWithMinMember(job, namespace, name, labels, minMember)
}

func synthesizedPodGroupWithMinMember(
	job map[string]any,
	namespace string,
	name string,
	labels map[string]string,
	minMember int64,
) ([]byte, error) {
	if minMember < 1 {
		minMember = 1
	}
	spec := map[string]any{
		"minMember":         minMember,
		"priorityClassName": priorityClassForJob(job),
	}
	if queue := shared.TextValue(job, "queue_name", "queueName"); queue != "" {
		spec["queue"] = queue
	}
	return json.Marshal(map[string]any{
		"apiVersion": volcanoPodGroupAPIVersion,
		"kind":       volcanoPodGroupKind,
		"metadata": map[string]any{
			"name":      name,
			"namespace": namespace,
			"labels":    labels,
		},
		"spec": spec,
	})
}

func fallbackPodsForTasks(
	job map[string]any,
	namespace string,
	groupName string,
	labels map[string]string,
	templates []volcanoTaskTemplate,
) ([]dispatchManifest, error) {
	count := int64(0)
	for _, template := range templates {
		count += taskReplicaCount(template)
	}
	manifests := make([]dispatchManifest, 0, count)
	for i, template := range templates {
		replicas := taskReplicaCount(template)
		for replica := int64(0); replica < replicas; replica++ {
			raw, err := fallbackPodForTask(fallbackPodRequest{
				job:       job,
				namespace: namespace,
				groupName: groupName,
				labels:    labels,
				template:  template,
				taskIndex: i,
				replica:   replica,
				replicas:  replicas,
			})
			if err != nil {
				return nil, err
			}
			manifests = append(manifests, dispatchManifest{Raw: raw})
		}
	}
	return manifests, nil
}

func fallbackPodForTask(request fallbackPodRequest) ([]byte, error) {
	copied, err := cloneJSONMap(request.template.template)
	if err != nil {
		return nil, fmt.Errorf("clone fallback pod template %s: %w", request.template.sourceName, err)
	}
	metadata, _, _ := unstructured.NestedMap(copied, "metadata")
	if metadata == nil {
		metadata = map[string]any{}
	}
	spec, found, _ := unstructured.NestedMap(copied, "spec")
	if !found {
		return nil, fmt.Errorf("%w: job %s fallback pod spec is required", cluster.ErrInvalidManifest, request.template.sourceName)
	}
	pod := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata":   metadata,
		"spec":       spec,
	}}
	if pod.GetName() == "" && request.replicas == 1 {
		pod.SetName(request.template.sourceName)
	}
	if pod.GetName() == "" || request.replicas > 1 {
		pod.SetName(fallbackPodName(request.job, request.taskIndex, request.replica))
	}
	pod.SetNamespace(request.namespace)
	mergeObjectLabels(pod, request.labels)
	queue := shared.TextValue(request.job, "queue_name", "queueName")
	applyFallbackPodQueueMetadata(pod, queue)
	setAnnotation(pod, queue)
	setPodGroupObjectMetadata(pod, request.groupName)
	raw, err := json.Marshal(pod.Object)
	if err != nil {
		return nil, fmt.Errorf(dispatcherMarshalResourceError, pod.GetName(), err)
	}
	return raw, nil
}

func fallbackPodName(job map[string]any, taskIndex int, replica int64) string {
	jobID := shared.FirstNonEmpty(shared.TextValue(job, "job_id", "jobId"), shared.TextValue(job, "id"))
	return sanitizeDNSLabel(fmt.Sprintf("%s-%d-%d", strings.ToLower(jobID), taskIndex, replica), "pod", 63)
}

func applyFallbackPodQueueMetadata(u *unstructured.Unstructured, queue string) {
	labels := u.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	if queue != "" {
		labels[platformJobQueueLabelKey] = queue
		labels[platformQueueNameLabelKey] = queue
	}
	if _, ok := labels[platformPreemptibleLabelKey]; !ok {
		labels[platformPreemptibleLabelKey] = "false"
	}
	u.SetLabels(labels)
}

func cloneJSONMap(in map[string]any) (map[string]any, error) {
	raw, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func volcanoGroupName(
	job map[string]any,
	resources []dispatchResource,
	tasks []volcanoTaskTemplate,
	native []dispatchResource,
) string {
	jobID := strings.ToLower(shared.FirstNonEmpty(shared.TextValue(job, "job_id", "jobId"), shared.TextValue(job, "id")))
	fallback := ""
	switch {
	case len(tasks) > 0:
		fallback = tasks[0].sourceName
	case len(native) > 0:
		fallback = native[0].Name
	case len(resources) > 0:
		fallback = resources[0].Name
	}
	return sanitizeDNSLabel(jobID, fallback, 55)
}

func batchJobReplicas(u *unstructured.Unstructured) int64 {
	if value := positiveNestedInt64(u.Object, "spec", "parallelism"); value > 0 {
		return value
	}
	if value := positiveNestedInt64(u.Object, "spec", "completions"); value > 0 {
		return value
	}
	return 1
}

func taskReplicaCount(template volcanoTaskTemplate) int64 {
	if template.replicas > 0 {
		return template.replicas
	}
	return 1
}

func nativeReplicaCount(resource dispatchResource) int64 {
	u, err := dispatchObject(resource)
	if err != nil {
		return 1
	}
	if strings.EqualFold(u.GetKind(), "Deployment") {
		if replicas := positiveNestedInt64(u.Object, "spec", "replicas"); replicas > 0 {
			return replicas
		}
	}
	return 1
}

func podTemplateHasContainers(template map[string]any) bool {
	for _, key := range []string{"containers", "initContainers"} {
		items, found, _ := unstructured.NestedSlice(template, "spec", key)
		if found && len(items) > 0 {
			return true
		}
	}
	return false
}

func positiveNestedInt64(obj map[string]any, fields ...string) int64 {
	value, found, _ := unstructured.NestedFieldNoCopy(obj, fields...)
	if !found || value == nil {
		return 0
	}
	switch typed := value.(type) {
	case int:
		return int64(typed)
	case int32:
		return int64(typed)
	case int64:
		return typed
	case float64:
		return int64(typed)
	case json.Number:
		parsed, err := typed.Int64()
		if err == nil {
			return parsed
		}
	}
	return 0
}

func mergeNestedLabels(obj map[string]any, labels map[string]string, fields ...string) {
	mergeNestedStringMap(obj, labels, fields...)
}

func mergeNestedStringMap(obj map[string]any, values map[string]string, fields ...string) {
	if len(values) == 0 {
		return
	}
	existing, _, _ := unstructured.NestedStringMap(obj, fields...)
	if existing == nil {
		existing = map[string]string{}
	}
	for key, value := range values {
		existing[key] = value
	}
	_ = unstructured.SetNestedStringMap(obj, existing, fields...)
}

func sanitizeDNSLabel(name, fallback string, maxLen int) string {
	if maxLen <= 0 {
		maxLen = 63
	}
	cleaned := cleanDNSLabel(name, maxLen)
	if cleaned != "" {
		return cleaned
	}
	cleaned = cleanDNSLabel(fallback, maxLen)
	if cleaned != "" {
		return cleaned
	}
	return "a"
}

func cleanDNSLabel(value string, maxLen int) string {
	sanitized := strings.Map(func(r rune) rune {
		r = unicode.ToLower(r)
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			return r
		}
		return '-'
	}, value)
	for strings.Contains(sanitized, "--") {
		sanitized = strings.ReplaceAll(sanitized, "--", "-")
	}
	sanitized = strings.Trim(sanitized, "-")
	if len(sanitized) > maxLen {
		sanitized = strings.TrimRight(sanitized[:maxLen], "-")
	}
	return sanitized
}
