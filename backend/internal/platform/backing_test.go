package platform

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDependencyAddressDefaultsPorts(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{name: "postgres", raw: "postgres://db/app", want: net.JoinHostPort("db", "5432")},
		{name: "redis", raw: "redis://cache/0", want: net.JoinHostPort("cache", "6379")},
		{name: "nats", raw: "nats://events", want: net.JoinHostPort("events", "4222")},
		{name: "explicit port", raw: "kafka://broker:19092", want: net.JoinHostPort("broker", "19092")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := dependencyAddress(BackingDependency{Name: envEventBusURL, URL: tc.raw})
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("dependencyAddress(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

func TestDependencyAddressRejectsInvalidURLs(t *testing.T) {
	for _, raw := range []string{"", "db:5432", "file:///tmp/db", "unknown://host"} {
		t.Run(raw, func(t *testing.T) {
			if _, err := dependencyAddress(BackingDependency{Name: envDatabaseURL, URL: raw}); err == nil {
				t.Fatalf("dependencyAddress(%q) error = nil, want error", raw)
			}
		})
	}
}

func TestReadyzChecksProductionBackingDependencies(t *testing.T) {
	checker := &recordingBackingChecker{}
	app := NewApp(validProductionConfig(), WithBackingChecker(checker))

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("/readyz status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if len(checker.checked) != 4 {
		t.Fatalf("checked dependencies = %#v, want database/redis/event bus/object store", checker.checked)
	}
}

func TestReadyzSkipsObjectStoreForNonBlobService(t *testing.T) {
	checker := &recordingBackingChecker{}
	cfg := validProductionConfig()
	cfg.ServiceName = "identity-service"
	cfg.AuthorizationPolicyURL = testPolicyURL
	cfg.AuthorizationPolicyAPIKey = testPolicyKey
	cfg.ObjectStoreURL = "http://127.0.0.1:1"
	cfg.ObjectStoreAccessKey = ""
	cfg.ObjectStoreSecretKey = ""
	app := NewApp(cfg, WithBackingChecker(checker))

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("/readyz status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if len(checker.checked) != 3 {
		t.Fatalf("checked dependencies = %#v, want database/redis/event bus", checker.checked)
	}
	for _, checked := range checker.checked {
		if checked == envObjectStoreURL {
			t.Fatalf("checked object store for non-blob service: %#v", checker.checked)
		}
	}
}

func TestReadyzFailsWhenProductionBackingDependencyUnavailable(t *testing.T) {
	checker := &recordingBackingChecker{failName: envRedisURL}
	app := NewApp(validProductionConfig(), WithBackingChecker(checker))

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("/readyz status = %d, want 503: %s", rec.Code, rec.Body.String())
	}
	if len(checker.checked) != 2 {
		t.Fatalf("checked dependencies = %#v, want stop after failed redis", checker.checked)
	}
}

func TestNewBackingResourcesSkipsObjectStoreForNonBlobService(t *testing.T) {
	backing, err := NewBackingResources(context.Background(), Config{
		ServiceName:       "identity-service",
		ObjectStoreURL:    "http://127.0.0.1:1",
		ObjectStoreBucket: "missing",
	})
	if err != nil {
		t.Fatalf("NewBackingResources non-blob error = %v, want nil", err)
	}
	defer backing.Close()

	app := NewApp(Config{ServiceName: "identity-service"}, backing.Options...)
	if app.ObjectStore != nil {
		t.Fatalf("object store = %T, want nil for non-blob service", app.ObjectStore)
	}
}

func TestNewBackingResourcesConnectsObjectStoreForBlobService(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	backing, err := NewBackingResources(ctx, Config{
		ServiceName:            mediaUploadServiceName,
		ObjectStoreURL:         "http://127.0.0.1:1",
		ObjectStoreAccessKey:   "access",
		ObjectStoreSecretKey:   "secret",
		ObjectStoreBucket:      "missing",
		AdapterTimeout:         100 * time.Millisecond,
		ExternalURLs:           map[string]string{},
		AuthorizationPolicyURL: "",
	})
	if err == nil {
		backing.Close()
		t.Fatal("NewBackingResources blob service error = nil, want object-store connection error")
	}
	if !strings.Contains(err.Error(), "connect object store") {
		t.Fatalf("NewBackingResources blob service error = %v, want connect object store", err)
	}
}

type recordingBackingChecker struct {
	failName string
	checked  []string
}

func (c *recordingBackingChecker) Check(_ context.Context, dependency BackingDependency) error {
	c.checked = append(c.checked, dependency.Name)
	if dependency.Name == c.failName {
		return errors.New("dial failed")
	}
	return nil
}
