package platform

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/platform/cluster"
	"k8s.io/client-go/kubernetes/fake"
)

func clusterRequiringSpec(name string, requires bool) ServiceSpec {
	return ServiceSpec{Name: name, RequiresCluster: requires}
}

func readyzCode(t *testing.T, app *App) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	return rec.Code, rec.Body.String()
}

func TestReadyzFailsClosedWhenClusterRequiredButAbsent(t *testing.T) {
	app := NewApp(validProductionConfigForService("cluster-needing-service"), WithBackingChecker(&recordingBackingChecker{}))
	app.RegisterService(clusterRequiringSpec("cluster-needing-service", true))

	code, body := readyzCode(t, app)
	if code != http.StatusServiceUnavailable {
		t.Fatalf("/readyz status = %d, want 503: %s", code, body)
	}
	if !strings.Contains(body, "cluster") {
		t.Fatalf("/readyz body = %s, want a cluster reason", body)
	}
}

func TestReadyzPassesWhenClusterRequiredAndConfigured(t *testing.T) {
	app := NewApp(validProductionConfigForService("cluster-needing-service"),
		WithBackingChecker(&recordingBackingChecker{}),
		WithCluster(cluster.New(fake.NewSimpleClientset(), "proj")),
	)
	app.RegisterService(clusterRequiringSpec("cluster-needing-service", true))

	if code, body := readyzCode(t, app); code != http.StatusOK {
		t.Fatalf("/readyz status = %d, want 200: %s", code, body)
	}
}

func TestReadyzFailsClosedForProductionStorageServiceWithoutCluster(t *testing.T) {
	app := NewApp(validProductionConfigForService("platform-io-unit"), WithBackingChecker(&recordingBackingChecker{}))
	app.RegisterService(clusterRequiringSpec("storage-service", true))

	code, body := readyzCode(t, app)
	if code != http.StatusServiceUnavailable {
		t.Fatalf("storage-service /readyz status = %d, want 503: %s", code, body)
	}
	if !strings.Contains(body, "cluster") {
		t.Fatalf("storage-service /readyz body = %s, want a cluster reason", body)
	}
}

func TestReadyzPassesForProductionStorageServiceWithCluster(t *testing.T) {
	app := NewApp(validProductionConfigForService("platform-io-unit"),
		WithBackingChecker(&recordingBackingChecker{}),
		WithCluster(cluster.New(fake.NewSimpleClientset(), "proj")),
	)
	app.RegisterService(clusterRequiringSpec("storage-service", true))

	if code, body := readyzCode(t, app); code != http.StatusOK {
		t.Fatalf("storage-service /readyz status = %d, want 200: %s", code, body)
	}
}

func TestReadyzPassesWhenNoHostedServiceRequiresCluster(t *testing.T) {
	app := NewApp(validProductionConfigForService("plain-service"), WithBackingChecker(&recordingBackingChecker{}))
	app.RegisterService(clusterRequiringSpec("plain-service", false))

	if code, body := readyzCode(t, app); code != http.StatusOK {
		t.Fatalf("/readyz status = %d, want 200: %s", code, body)
	}
}

func TestReadyzClusterGateIsProductionOnly(t *testing.T) {
	cfg := validProductionConfig()
	cfg.Production = false
	app := NewApp(cfg, WithBackingChecker(&recordingBackingChecker{}))
	app.RegisterService(clusterRequiringSpec("cluster-needing-service", true))

	if code, body := readyzCode(t, app); code != http.StatusOK {
		t.Fatalf("non-production /readyz status = %d, want 200: %s", code, body)
	}
}
