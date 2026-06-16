package platform

import (
	"strings"
	"testing"
)

const (
	testIdentityUsersResource = "identity-service:users"
	testIdentitySessions      = "identity-service:sessions"
	testWidgetResource        = "widget-service:widgets"
)

func TestValidateServiceIsolationAllowsAllServices(t *testing.T) {
	app := NewApp(Config{ServiceName: "all"})
	app.RegisterStoreDependencies("widget-service", testIdentityUsersResource)

	if err := app.ValidateServiceIsolation(); err != nil {
		t.Fatalf("SERVICE_NAME=all should allow co-hosted dependencies: %v", err)
	}
}

func TestValidateServiceIsolationAllowsOwnedResources(t *testing.T) {
	app := NewApp(Config{ServiceName: "widget-service"})
	app.RegisterStoreDependencies("widget-service", testWidgetResource)

	if err := app.ValidateServiceIsolation(); err != nil {
		t.Fatalf("owned store dependency should pass: %v", err)
	}
}

func TestValidateServiceIsolationFlagsExternalResources(t *testing.T) {
	app := NewApp(Config{ServiceName: "widget-service"})
	app.RegisterStoreDependencies("widget-service", testIdentityUsersResource)

	err := app.ValidateServiceIsolation()
	if err == nil {
		t.Fatal("expected external store dependency to fail isolation validation")
	}
	for _, want := range []string{"widget-service", testIdentityUsersResource} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not include %q", err.Error(), want)
		}
	}
}

func TestValidateServiceIsolationRequiresServiceKeyForRemoteReads(t *testing.T) {
	app := NewApp(Config{
		ServiceName: "widget-service",
		ServiceURLs: map[string]string{"identity-service": "http://identity-service"},
	})
	app.RegisterStoreDependencies("widget-service", testIdentityUsersResource)

	if err := app.ValidateServiceIsolation(); err == nil {
		t.Fatal("expected SERVICE_URLS without SERVICE_API_KEY to remain an isolation gap")
	}
}

func TestValidateServiceIsolationAllowsRemoteReadsWithServiceKey(t *testing.T) {
	app := NewApp(Config{
		ServiceName:   "widget-service",
		ServiceURLs:   map[string]string{"identity-service": "http://identity-service"},
		ServiceAPIKey: "service-key",
	})
	app.RegisterStoreDependencies("widget-service", testIdentityUsersResource)

	if err := app.ValidateServiceIsolation(); err != nil {
		t.Fatalf("configured remote read should pass isolation validation: %v", err)
	}
}

func TestValidateServiceIsolationRejectsRemoteReadsWithoutDomainContract(t *testing.T) {
	app := NewApp(Config{
		ServiceName:   "widget-service",
		ServiceURLs:   map[string]string{"identity-service": "http://identity-service"},
		ServiceAPIKey: "service-key",
	})
	app.RegisterStoreDependencies("widget-service", testIdentitySessions)

	if err := app.ValidateServiceIsolation(); err == nil {
		t.Fatal("expected uncontracted remote resource to remain an isolation gap")
	}
}
