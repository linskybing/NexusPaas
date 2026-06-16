//go:build e2e

package e2e

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/platform/cluster"
	"github.com/linskybing/nexuspaas/backend/internal/services"
)

// okBackingChecker passes every backing-dependency check so the cluster readiness gate
// can be exercised in isolation without standing up Postgres/Redis/MinIO.
type okBackingChecker struct{}

func (okBackingChecker) Check(context.Context, platform.BackingDependency) error { return nil }

// TestClusterReadinessGateE2E proves, against live Docker Desktop Kubernetes, that a
// production deployment hosting a cluster-dependent service (workload-service) reports
// ready only when a real cluster client is wired, and fails closed (503) when the
// cluster client is absent — the previously silent degradation in problem.md #5.
func TestClusterReadinessGateE2E(t *testing.T) {
	if strings.TrimSpace(os.Getenv("TEST_LIVE_K8S_CLUSTER_READINESS")) != "1" {
		t.Skip("TEST_LIVE_K8S_CLUSTER_READINESS=1 not set; skipping live Kubernetes cluster-readiness e2e")
	}
	ensureDefaultKubeconfig(t)

	cl, err := cluster.NewFromEnv("proj")
	if err != nil {
		t.Fatalf("create cluster client: %v", err)
	}
	if cl == nil {
		t.Skip("no Kubernetes client available")
	}

	cfg := platform.Config{
		ServiceName: "workload-service",
		HTTPAddr:    ":0",
		Production:  true,
		RequireAuth: false,
	}

	// With a live cluster client → ready.
	withCluster := platform.NewApp(cfg, platform.WithBackingChecker(okBackingChecker{}), platform.WithCluster(cl))
	services.RegisterAll(withCluster)
	if code, body := readyz(withCluster); code != http.StatusOK {
		t.Fatalf("with live cluster /readyz = %d, want 200: %s", code, body)
	}

	// Without a cluster client → fail closed with a cluster reason.
	withoutCluster := platform.NewApp(cfg, platform.WithBackingChecker(okBackingChecker{}))
	services.RegisterAll(withoutCluster)
	code, body := readyz(withoutCluster)
	if code != http.StatusServiceUnavailable {
		t.Fatalf("without cluster /readyz = %d, want 503: %s", code, body)
	}
	if !strings.Contains(body, "cluster") {
		t.Fatalf("without cluster /readyz body = %s, want a cluster reason", body)
	}
}

func readyz(app *platform.App) (int, string) {
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	return rec.Code, rec.Body.String()
}
