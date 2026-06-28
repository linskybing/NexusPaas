package cluster

import (
	"context"
	"errors"
	"strings"
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestFastTransferMoverCreatesSafeJob(t *testing.T) {
	cl := New(fake.NewSimpleClientset(), "proj")

	result := cl.EnsureFastTransferMoverJob(context.Background(), fastTransferMoverTestOptions())
	if result.Action != FastTransferMoverActionCreated {
		t.Fatalf("result = %#v, want created", result)
	}

	job := getFastTransferMoverJob(t, cl, "project-p1", "fast-transfer-copy1")
	assertFastTransferMoverJob(t, job)
}

func TestFastTransferMoverAddsBestEffortProgressCallback(t *testing.T) {
	cl := New(fake.NewSimpleClientset(), "proj")
	opts := fastTransferMoverTestOptions()
	opts.ProgressURL = "http://storage/internal/progress"
	opts.ProgressServiceName = "k8s-control-service"
	opts.ProgressServiceKey = "secret-key"

	result := cl.EnsureFastTransferMoverJob(context.Background(), opts)
	if result.Action != FastTransferMoverActionCreated {
		t.Fatalf("result = %#v, want created", result)
	}

	container := getFastTransferMoverJob(t, cl, "project-p1", "fast-transfer-copy1").Spec.Template.Spec.Containers[0]
	env := fastTransferMoverEnvMap(container.Env)
	if env["NEXUSPAAS_FAST_TRANSFER_PROGRESS_URL"] != opts.ProgressURL ||
		env["NEXUSPAAS_FAST_TRANSFER_PROGRESS_SERVICE_NAME"] != opts.ProgressServiceName ||
		env["NEXUSPAAS_FAST_TRANSFER_PROGRESS_KEY"] != opts.ProgressServiceKey {
		t.Fatalf("env = %#v, want progress callback env", env)
	}
	script := container.Args[0]
	for _, want := range []string{
		`wget -q -O-`,
		`--header "X-Service-Name: ${NEXUSPAAS_FAST_TRANSFER_PROGRESS_SERVICE_NAME}"`,
		`--header "X-Service-Key: ${NEXUSPAAS_FAST_TRANSFER_PROGRESS_KEY}"`,
		`post_progress '{"status":"running","progress_pct":1}'`,
		`post_progress '{"status":"succeeded","progress_pct":100}'`,
		`|| true`,
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("script missing %q:\n%s", want, script)
		}
	}
	if strings.Contains(script, opts.ProgressServiceKey) {
		t.Fatalf("script contains literal progress key: %s", script)
	}
	if strings.Index(script, `"status":"running"`) > strings.Index(script, `rsync -a --delete --`) ||
		strings.Index(script, `"status":"succeeded"`) < strings.Index(script, `rsync -a --delete --`) {
		t.Fatalf("script progress order is wrong:\n%s", script)
	}
}

func TestFastTransferMoverOmitsIncompleteProgressCallback(t *testing.T) {
	opts := fastTransferMoverTestOptions()
	opts.ProgressURL = "http://storage/internal/progress"

	job := buildFastTransferMoverJob(normalizeFastTransferMoverOptions(opts))
	if env := job.Spec.Template.Spec.Containers[0].Env; len(env) != 0 {
		t.Fatalf("env = %#v, want no callback env without key", env)
	}
}

func TestFastTransferMoverAlreadyExistsForMatchingTransfer(t *testing.T) {
	existing := buildFastTransferMoverJob(normalizeFastTransferMoverOptions(fastTransferMoverTestOptions()))
	cl := New(fake.NewSimpleClientset(existing), "proj")

	result := cl.EnsureFastTransferMoverJob(context.Background(), fastTransferMoverTestOptions())
	if result.Action != FastTransferMoverActionAlreadyExists {
		t.Fatalf("result = %#v, want already_exists", result)
	}
}

func TestFastTransferMoverConflictDoesNotMutate(t *testing.T) {
	existing := buildFastTransferMoverJob(normalizeFastTransferMoverOptions(fastTransferMoverTestOptions()))
	existing.Annotations[fastTransferMoverTransferID] = "other-transfer"
	cl := New(fake.NewSimpleClientset(existing), "proj")

	result := cl.EnsureFastTransferMoverJob(context.Background(), fastTransferMoverTestOptions())
	if result.Action != FastTransferMoverActionFailed || !strings.Contains(result.Reason, "conflicting") {
		t.Fatalf("result = %#v, want failed conflict", result)
	}
	got := getFastTransferMoverJob(t, cl, "project-p1", "fast-transfer-copy1")
	if got.Annotations[fastTransferMoverTransferID] != "other-transfer" {
		t.Fatalf("conflicting job was mutated: %#v", got.Annotations)
	}
}

func TestFastTransferMoverRejectsUnsafeInputs(t *testing.T) {
	cases := []FastTransferMoverJobOptions{
		func() FastTransferMoverJobOptions {
			opts := fastTransferMoverTestOptions()
			opts.Tool = "sh"
			return opts
		}(),
		func() FastTransferMoverJobOptions {
			opts := fastTransferMoverTestOptions()
			opts.Source.Path = "/data/source;rm"
			return opts
		}(),
		func() FastTransferMoverJobOptions {
			opts := fastTransferMoverTestOptions()
			opts.Source.PVC = "../host"
			return opts
		}(),
	}
	cl := New(fake.NewSimpleClientset(), "proj")
	for _, opts := range cases {
		result := cl.EnsureFastTransferMoverJob(context.Background(), opts)
		if result.Action != FastTransferMoverActionInvalid {
			t.Fatalf("opts = %#v result = %#v, want invalid", opts, result)
		}
	}
}

func TestFastTransferMoverReportsCreateFailure(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	clientset.PrependReactor("create", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("api denied")
	})
	cl := New(clientset, "proj")

	result := cl.EnsureFastTransferMoverJob(context.Background(), fastTransferMoverTestOptions())
	if result.Action != FastTransferMoverActionFailed || result.Error == "" {
		t.Fatalf("result = %#v, want failed error", result)
	}
}

func fastTransferMoverTestOptions() FastTransferMoverJobOptions {
	return FastTransferMoverJobOptions{
		ProjectID:  "P1",
		TransferID: "P1:project-p1:copy1",
		Namespace:  "project-p1",
		Name:       "copy1",
		Source:     FastTransferMoverEndpoint{Namespace: "project-p1", PVC: "dataset-pvc", Path: "/data/source"},
		Target:     FastTransferMoverEndpoint{Namespace: "project-p1", PVC: "scratch-pvc", Path: "/data/target"},
		Tool:       FastTransferMoverToolRsync,
		Image:      "rsync:test",
	}
}

func getFastTransferMoverJob(t *testing.T, cl *Client, namespace, name string) *batchv1.Job {
	t.Helper()
	job, err := cl.Clientset().BatchV1().Jobs(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get mover job: %v", err)
	}
	return job
}

func assertFastTransferMoverJob(t *testing.T, job *batchv1.Job) {
	t.Helper()
	pod := job.Spec.Template.Spec
	if pod.AutomountServiceAccountToken == nil || *pod.AutomountServiceAccountToken {
		t.Fatalf("automountServiceAccountToken = %#v, want false", pod.AutomountServiceAccountToken)
	}
	if pod.RestartPolicy != corev1.RestartPolicyNever {
		t.Fatalf("restartPolicy = %q, want Never", pod.RestartPolicy)
	}
	if len(pod.Containers) != 1 {
		t.Fatalf("containers = %#v, want one", pod.Containers)
	}
	container := pod.Containers[0]
	if container.SecurityContext != nil && container.SecurityContext.Privileged != nil && *container.SecurityContext.Privileged {
		t.Fatalf("container is privileged: %#v", container.SecurityContext)
	}
	if len(container.Command) != 2 || container.Command[0] != "/bin/sh" || container.Command[1] != "-c" {
		t.Fatalf("command = %#v, want fixed shell", container.Command)
	}
	if len(container.Args) != 1 {
		t.Fatalf("args = %#v, want one restricted script", container.Args)
	}
	mkdirCommand := `mkdir -p "/mnt/target/data/target/"`
	rsyncCommand := `rsync -a --delete -- "/mnt/source/data/source/" "/mnt/target/data/target/"`
	mkdirIndex, rsyncIndex := strings.Index(container.Args[0], mkdirCommand), strings.Index(container.Args[0], rsyncCommand)
	if mkdirIndex < 0 || rsyncIndex < 0 || mkdirIndex > rsyncIndex {
		t.Fatalf("args = %#v, want target mkdir before allowlisted rsync", container.Args)
	}
	if len(pod.Volumes) != 2 || pod.Volumes[0].HostPath != nil || pod.Volumes[1].HostPath != nil {
		t.Fatalf("volumes = %#v, want PVC-only volumes", pod.Volumes)
	}
}

func fastTransferMoverEnvMap(env []corev1.EnvVar) map[string]string {
	values := map[string]string{}
	for _, entry := range env {
		values[entry.Name] = entry.Value
	}
	return values
}
