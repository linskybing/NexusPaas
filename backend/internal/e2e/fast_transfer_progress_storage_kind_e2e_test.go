//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"net"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/platform/cluster"
	"github.com/linskybing/nexuspaas/backend/internal/services/k8scontrol"
	storageservice "github.com/linskybing/nexuspaas/backend/internal/services/storage"
)

const (
	fastTransferProgressStorageKindEnv         = "TEST_LIVE_FAST_TRANSFER_PROGRESS_STORAGE_KIND"
	fastTransferProgressStorageCallbackBaseEnv = "TEST_KIND_PROGRESS_STORAGE_CALLBACK_BASE_URL"
	fastTransferProgressStorageToK8sKey        = "fast-transfer-e2e-storage-to-k8s-key"
	fastTransfersE2EResource                   = "storage-service:fast_transfers"
)

func TestFastTransferProgressStorageKindE2E(t *testing.T) {
	if strings.TrimSpace(os.Getenv(fastTransferProgressStorageKindEnv)) != "1" {
		t.Skip("set " + fastTransferProgressStorageKindEnv + "=1 to run live FastTransfer progress-to-storage kind e2e")
	}
	requireLiveKubeconfig(t)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cl, err := cluster.NewFromEnv("proj")
	if err != nil {
		t.Fatalf("create live Kubernetes client: %v", err)
	}
	if cl == nil {
		t.Fatal("live Kubernetes client is unavailable")
	}
	if err := cl.Ping(ctx); err != nil {
		t.Fatalf("ping live Kubernetes cluster: %v", err)
	}

	suffix := strconv.FormatInt(time.Now().UTC().UnixNano(), 36)
	namespace := "ft-storage-" + suffix
	createFastTransferMoverAdmissionNamespace(t, ctx, cl, namespace)
	waitFastTransferProgressCallbackDefaultServiceAccount(t, ctx, cl, namespace)
	createFastTransferMoverExecutionPVC(t, ctx, cl, namespace, "source-pvc")
	createFastTransferMoverExecutionPVC(t, ctx, cl, namespace, "target-pvc")
	runFastTransferMoverExecutionPVCPod(t, ctx, cl, namespace, "seed-"+suffix, "source-pvc", fmt.Sprintf(`set -eu
mkdir -p /pvc/data/source
printf %%s %q > /pvc/data/source/hello.txt`, fastTransferMoverExecutionContent))
	waitFastTransferMoverExecutionPVCBound(t, ctx, cl, namespace, "source-pvc")

	listener, localStorageURL, callbackBaseURL := fastTransferProgressStorageListener(t)
	k8sServer := httptest.NewServer(newFastTransferProgressStorageK8sControlApp(cl, callbackBaseURL))
	t.Cleanup(k8sServer.Close)

	store := platform.NewStore()
	events := platform.NewEventBus()
	projectID, userID := "p-"+suffix, "u-"+suffix
	seedFastTransferStartAccess(t, store, projectID, userID)
	storageServer := httptest.NewUnstartedServer(newFastTransferProgressStorageApp(store, events, k8sServer.URL))
	storageServer.Listener = listener
	storageServer.Start()
	t.Cleanup(storageServer.Close)

	name := "copy-" + suffix
	transferID := projectID + ":" + namespace + ":" + name
	first := postFastTransferStart(t, localStorageURL, projectID, userID, fastTransferStartRequest(namespace, name, "idem-"+suffix))
	assertFastTransferStartRecord(t, first, transferID, namespace)
	assertFastTransferStartQueuedEvent(t, events, transferID)

	jobNamespace := textE2E(first["mover_job_namespace"])
	jobName := textE2E(first["mover_job_name"])
	job := getFastTransferMoverAdmissionJob(t, ctx, cl, jobNamespace, jobName)
	assertFastTransferMoverAdmissionJob(t, job, namespace, jobName, transferID)
	assertFastTransferProgressCallbackEnv(t, job, callbackBaseURL+"/internal/storage/projects/"+projectID+"/transfers/"+namespace+"/"+name+"/progress")

	waitFastTransferMoverExecutionJobComplete(t, ctx, cl, jobNamespace, jobName)
	assertFastTransferMoverExecutionPodSucceeded(t, ctx, cl, jobNamespace, jobName)
	runFastTransferMoverExecutionPVCPod(t, ctx, cl, namespace, "verify-"+suffix, "target-pvc", fmt.Sprintf(`set -eu
got="$(cat /pvc/data/target/hello.txt)"
[ "$got" = %q ]`, fastTransferMoverExecutionContent))

	record := waitFastTransferProgressStorageSucceeded(t, ctx, store, transferID)
	if textE2E(record["status"]) != "succeeded" || numberValue(record["progress_pct"]) != 100 {
		t.Fatalf("transfer progress = %#v, want succeeded/100", record)
	}
	assertFastTransferProgressStorageEvent(t, events, "FastTransferProgressed", transferID)
	assertFastTransferProgressStorageEvent(t, events, "FastTransferCompleted", transferID)
}

func newFastTransferProgressStorageApp(store *platform.Store, events *platform.EventBus, k8sControlURL string) *platform.App {
	app := platform.NewApp(platform.Config{
		ServiceName:             "storage-service",
		HTTPAddr:                ":0",
		RequireAuth:             false,
		ServiceFallbackDisabled: true,
		ServiceIdentityName:     "storage-service",
		ServiceIdentityKey:      fastTransferProgressStorageToK8sKey,
		ServiceTrustedIdentities: map[string]platform.ServiceTrustedIdentity{
			"k8s-control-service": {Key: fastTransferProgressCallbackKey, Audiences: []string{"storage-service"}},
		},
		ServiceURLs: map[string]string{"k8s-control-service": k8sControlURL},
	}, platform.WithStore(store), platform.WithEventBus(events))
	app.RegisterService(storageservice.Spec())
	storageservice.Register(app)
	return app
}

func newFastTransferProgressStorageK8sControlApp(cl *cluster.Client, callbackBaseURL string) *platform.App {
	app := platform.NewApp(platform.Config{
		ServiceName:             "k8s-control-service",
		HTTPAddr:                ":0",
		RequireAuth:             true,
		ServiceFallbackDisabled: true,
		ServiceIdentityName:     "k8s-control-service",
		ServiceIdentityKey:      fastTransferProgressCallbackKey,
		ServiceTrustedIdentities: map[string]platform.ServiceTrustedIdentity{
			"storage-service": {Key: fastTransferProgressStorageToK8sKey, Audiences: []string{"k8s-control-service"}},
		},
		ServiceURLs:            map[string]string{"storage-service": callbackBaseURL},
		FastTransferMoverImage: cluster.FastTransferMoverDefaultImage,
	}, platform.WithCluster(cl))
	app.RegisterService(k8scontrol.Spec())
	k8scontrol.Register(app)
	return app
}

func fastTransferProgressStorageListener(t *testing.T) (net.Listener, string, string) {
	t.Helper()
	configured := strings.TrimRight(strings.TrimSpace(os.Getenv(fastTransferProgressStorageCallbackBaseEnv)), "/")
	port := ""
	if configured != "" {
		parsed, err := url.Parse(configured)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.Port() == "" {
			t.Fatalf("%s must be an absolute URL with an explicit port, got %q", fastTransferProgressStorageCallbackBaseEnv, configured)
		}
		port = parsed.Port()
	}
	listenAddr := "0.0.0.0:0"
	if port != "" {
		listenAddr = "0.0.0.0:" + port
	}
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		t.Fatalf("listen for host-reachable storage callback on %s: %v", listenAddr, err)
	}
	actualPort := strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)
	localURL := "http://127.0.0.1:" + actualPort
	if configured == "" {
		configured = "http://host.docker.internal:" + actualPort
	}
	return listener, localURL, configured
}

func waitFastTransferProgressStorageSucceeded(t *testing.T, ctx context.Context, store *platform.Store, transferID string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		record, found := store.Get(ctx, fastTransfersE2EResource, transferID)
		if found && textE2E(record.Data["status"]) == "succeeded" && numberValue(record.Data["progress_pct"]) == 100 {
			return record.Data
		}
		if err := ctx.Err(); err != nil {
			t.Fatalf("wait for FastTransfer %s succeeded: %v", transferID, err)
		}
		if time.Now().After(deadline) {
			if found {
				t.Fatalf("FastTransfer %s did not reach succeeded/100: %#v", transferID, record.Data)
			}
			t.Fatalf("FastTransfer %s was not found", transferID)
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func assertFastTransferProgressStorageEvent(t *testing.T, events *platform.EventBus, name, transferID string) {
	t.Helper()
	for _, event := range events.Outbox() {
		if event.Name == name && event.Source == "storage-service" && textE2E(event.Data["transfer_id"]) == transferID {
			return
		}
	}
	t.Fatalf("outbox missing %s for %s: %#v", name, transferID, eventNames(events.Outbox()))
}
