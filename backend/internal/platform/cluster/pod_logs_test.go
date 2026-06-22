package cluster

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestListJobPodLogsUsesExplicitPodsAndBoundsReads(t *testing.T) {
	c := newFakeClient(
		logPod("proj-p1", "wanted", map[string]string{LabelJobID: "j1"}),
		logPod("proj-p1", "ignored", map[string]string{LabelJobID: "j1"}),
	)

	lines, err := c.ListJobPodLogs(context.Background(), "proj-p1", "j1", PodLogOptions{PodNames: []string{"wanted"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 || lines[0].Pod != "wanted" || lines[0].Message != "fake logs" {
		t.Fatalf("lines = %#v, want one fake log from explicit pod", lines)
	}
	assertPodLogOptions(t, c, "main", DefaultPodLogTailLines, DefaultPodLogLimitBytes)
	if hasAction(c, "list", "pods", "") {
		t.Fatal("explicit pod lookup should not fall back to list pods")
	}
}

func TestListJobPodLogsFallsBackToJobLabelSelector(t *testing.T) {
	c := newFakeClient(
		logPod("proj-p1", "one", map[string]string{LabelJobID: "j1"}),
		logPod("proj-p1", "two", map[string]string{LabelJobID: "j1"}),
		logPod("proj-p1", "other", map[string]string{LabelJobID: "j2"}),
	)

	lines, err := c.ListJobPodLogs(context.Background(), "proj-p1", "j1", PodLogOptions{TailLines: 3, LimitBytes: 99})
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 2 {
		t.Fatalf("lines = %#v, want two matching pod logs", lines)
	}
	assertPodLogOptions(t, c, "main", 3, 99)
}

func TestListJobPodLogsCapsAggregateReads(t *testing.T) {
	c := newFakeClient(
		logPodWithContainers("proj-p1", "one", map[string]string{LabelJobID: "j1"}, "main", "sidecar"),
		logPodWithContainers("proj-p1", "two", map[string]string{LabelJobID: "j1"}, "main"),
		logPodWithContainers("proj-p1", "three", map[string]string{LabelJobID: "j1"}, "main"),
	)

	lines, err := c.ListJobPodLogs(context.Background(), "proj-p1", "j1", PodLogOptions{
		MaxPods:       2,
		MaxContainers: 2,
		MaxLines:      2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 2 {
		t.Fatalf("lines = %#v, want cap at two aggregate lines", lines)
	}
	if got := countActions(c, "get", "pods", "log"); got != 2 {
		t.Fatalf("pods/log actions = %d, want aggregate container cap 2", got)
	}
}

func TestParsePodLogLinesSplitsRFC3339Timestamp(t *testing.T) {
	lines := parsePodLogLines("ns", "pod", "main", "2026-06-22T03:04:05Z hello\nplain line\n")
	if len(lines) != 2 {
		t.Fatalf("lines = %#v, want two", lines)
	}
	if lines[0].Timestamp != "2026-06-22T03:04:05Z" || lines[0].Message != "hello" {
		t.Fatalf("timestamped line = %#v", lines[0])
	}
	if lines[1].Timestamp != "" || lines[1].Message != "plain line" {
		t.Fatalf("plain line = %#v", lines[1])
	}
}

func logPod(namespace, name string, labels map[string]string) *corev1.Pod {
	return logPodWithContainers(namespace, name, labels, "main")
}

func logPodWithContainers(namespace, name string, labels map[string]string, containers ...string) *corev1.Pod {
	specContainers := make([]corev1.Container, 0, len(containers))
	for _, container := range containers {
		specContainers = append(specContainers, corev1.Container{Name: container})
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name, Labels: labels},
		Spec:       corev1.PodSpec{Containers: specContainers},
	}
}

func assertPodLogOptions(t *testing.T, c *Client, container string, tail, limit int64) {
	t.Helper()
	for _, action := range c.Clientset().(*k8sfake.Clientset).Actions() {
		if action.GetVerb() != "get" || action.GetResource().Resource != "pods" || action.GetSubresource() != "log" {
			continue
		}
		generic, ok := action.(k8stesting.GenericAction)
		if !ok {
			t.Fatalf("log action = %T, want GenericAction", action)
		}
		opts, ok := generic.GetValue().(*corev1.PodLogOptions)
		if !ok {
			t.Fatalf("log action value = %T, want *PodLogOptions", generic.GetValue())
		}
		if opts.Container != container || opts.TailLines == nil || *opts.TailLines != tail ||
			opts.LimitBytes == nil || *opts.LimitBytes != limit || opts.Follow {
			t.Fatalf("pod log options = %#v, want container=%q tail=%d limit=%d follow=false", opts, container, tail, limit)
		}
		return
	}
	t.Fatal("missing pods/log action")
}

func hasAction(c *Client, verb, resource, subresource string) bool {
	return countActions(c, verb, resource, subresource) > 0
}

func countActions(c *Client, verb, resource, subresource string) int {
	count := 0
	for _, action := range c.Clientset().(*k8sfake.Clientset).Actions() {
		if action.GetVerb() == verb && action.GetResource().Resource == resource && action.GetSubresource() == subresource {
			count++
		}
	}
	return count
}
