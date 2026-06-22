package cluster

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

const (
	DefaultPodLogTailLines     int64 = 200
	DefaultPodLogLimitBytes    int64 = 65536
	DefaultPodLogMaxPods             = 8
	DefaultPodLogMaxContainers       = 16
	DefaultPodLogMaxLines            = 200
)

type PodLogOptions struct {
	TailLines     int64
	LimitBytes    int64
	MaxPods       int
	MaxContainers int
	MaxLines      int
	PodNames      []string
}

type PodLogLine struct {
	Namespace string
	Pod       string
	Container string
	Line      int
	Message   string
	Timestamp string
}

func (c *Client) ListJobPodLogs(ctx context.Context, namespace, jobID string, opts PodLogOptions) ([]PodLogLine, error) {
	namespace = strings.TrimSpace(namespace)
	jobID = strings.TrimSpace(jobID)
	if c == nil || c.clientset == nil || namespace == "" || jobID == "" {
		return nil, nil
	}
	bounds := podLogBounds(opts)
	pods, err := c.jobLogPods(ctx, namespace, jobID, opts.PodNames)
	if err != nil {
		return nil, err
	}
	if len(pods) > bounds.maxPods {
		pods = pods[:bounds.maxPods]
	}
	lines := make([]PodLogLine, 0)
	containers := 0
	for i := range pods {
		if len(lines) >= bounds.maxLines || containers >= bounds.maxContainers {
			break
		}
		podLines, used := c.podLogLines(ctx, &pods[i], bounds, bounds.maxContainers-containers, bounds.maxLines-len(lines))
		containers += used
		lines = append(lines, podLines...)
	}
	return lines, nil
}

type podLogReadBounds struct {
	tail          int64
	limit         int64
	maxPods       int
	maxContainers int
	maxLines      int
}

func podLogBounds(opts PodLogOptions) podLogReadBounds {
	tail := opts.TailLines
	if tail <= 0 {
		tail = DefaultPodLogTailLines
	}
	limit := opts.LimitBytes
	if limit <= 0 {
		limit = DefaultPodLogLimitBytes
	}
	maxPods := opts.MaxPods
	if maxPods <= 0 {
		maxPods = DefaultPodLogMaxPods
	}
	maxContainers := opts.MaxContainers
	if maxContainers <= 0 {
		maxContainers = DefaultPodLogMaxContainers
	}
	maxLines := opts.MaxLines
	if maxLines <= 0 {
		maxLines = DefaultPodLogMaxLines
	}
	return podLogReadBounds{tail: tail, limit: limit, maxPods: maxPods, maxContainers: maxContainers, maxLines: maxLines}
}

func (c *Client) jobLogPods(ctx context.Context, namespace, jobID string, names []string) ([]corev1.Pod, error) {
	seen := map[string]bool{}
	pods := make([]corev1.Pod, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		pod, err := c.clientset.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			continue
		}
		if pod.Labels[LabelJobID] == jobID {
			pods = append(pods, *pod)
		}
	}
	if len(pods) > 0 {
		return pods, nil
	}
	selector := labels.SelectorFromSet(labels.Set{LabelJobID: jobID}).String()
	list, err := c.clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, fmt.Errorf("list job log pods: %w", err)
	}
	return list.Items, nil
}

func (c *Client) podLogLines(ctx context.Context, pod *corev1.Pod, bounds podLogReadBounds, maxContainers, maxLines int) ([]PodLogLine, int) {
	if pod == nil || maxContainers <= 0 || maxLines <= 0 {
		return nil, 0
	}
	lines := make([]PodLogLine, 0)
	containers := 0
	for _, container := range pod.Spec.Containers {
		if containers >= maxContainers || len(lines) >= maxLines {
			break
		}
		containers++
		raw, err := c.clientset.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &corev1.PodLogOptions{
			Container:  container.Name,
			TailLines:  &bounds.tail,
			LimitBytes: &bounds.limit,
			Timestamps: true,
			Follow:     false,
		}).DoRaw(ctx)
		if err != nil {
			continue
		}
		for _, line := range parsePodLogLines(pod.Namespace, pod.Name, container.Name, string(raw)) {
			if len(lines) >= maxLines {
				break
			}
			lines = append(lines, line)
		}
	}
	return lines, containers
}

func parsePodLogLines(namespace, pod, container, raw string) []PodLogLine {
	parts := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	lines := make([]PodLogLine, 0, len(parts))
	for _, part := range parts {
		message := strings.TrimRight(part, "\r")
		if strings.TrimSpace(message) == "" {
			continue
		}
		timestamp, text := splitKubernetesLogTimestamp(message)
		lines = append(lines, PodLogLine{
			Namespace: namespace,
			Pod:       pod,
			Container: container,
			Line:      len(lines) + 1,
			Message:   text,
			Timestamp: timestamp,
		})
	}
	return lines
}

func splitKubernetesLogTimestamp(line string) (string, string) {
	first, rest, ok := strings.Cut(line, " ")
	if !ok {
		return "", line
	}
	if _, err := time.Parse(time.RFC3339Nano, first); err != nil {
		return "", line
	}
	return first, rest
}
