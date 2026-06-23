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

func TestValidateServiceIsolationAllowsCoHostedDeployableUnitDependencies(t *testing.T) {
	app := NewApp(Config{ServiceName: "iam-unit"})
	app.RegisterStoreDependencies("authorization-policy-service", testIdentityUsersResource)

	if err := app.ValidateServiceIsolation(); err != nil {
		t.Fatalf("iam-unit should allow co-hosted identity/authz dependency: %v", err)
	}
}

func TestValidateServiceIsolationRequiresRemoteForCrossUnitDependency(t *testing.T) {
	app := NewApp(Config{ServiceName: "compute-api"})
	app.RegisterOwnerReadDependencies("workload-service", "org-project-service:projects")

	if err := app.ValidateServiceIsolation(); err == nil {
		t.Fatal("expected compute-api cross-unit owner read without SERVICE_URLS to fail")
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

func TestValidateServiceIsolationStrictProfileRejectsLegacyOnlyRemoteReads(t *testing.T) {
	app := NewApp(Config{
		ServiceName:        "widget-service",
		EnvironmentProfile: runtimeProfileStaging,
		ServiceURLs:        map[string]string{"identity-service": "http://identity-service"},
		ServiceAPIKey:      "legacy-key",
	})
	app.RegisterStoreDependencies("widget-service", testIdentityUsersResource)

	if err := app.ValidateServiceIsolation(); err == nil {
		t.Fatal("expected strict remote read with only SERVICE_API_KEY to fail isolation validation")
	}
}

func TestValidateServiceIsolationStrictProfileAllowsScopedRemoteReads(t *testing.T) {
	app := NewApp(Config{
		ServiceName:         "widget-service",
		EnvironmentProfile:  runtimeProfileStaging,
		ServiceURLs:         map[string]string{"identity-service": "http://identity-service"},
		ServiceIdentityName: "widget-service",
		ServiceIdentityKey:  "scoped-key",
	})
	app.RegisterStoreDependencies("widget-service", testIdentityUsersResource)

	if err := app.ValidateServiceIsolation(); err != nil {
		t.Fatalf("strict scoped remote read should pass isolation validation: %v", err)
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

func TestValidateServiceIsolationOwnerReadRequiresServiceKey(t *testing.T) {
	app := NewApp(Config{
		ServiceName: "widget-service",
		ServiceURLs: map[string]string{"identity-service": "http://identity-service"},
	})
	app.RegisterOwnerReadDependencies("widget-service", testIdentityUsersResource)

	err := app.ValidateServiceIsolation()
	if err == nil {
		t.Fatal("expected owner read without SERVICE_API_KEY to fail isolation validation")
	}
	if !strings.Contains(err.Error(), "(owner-read)") {
		t.Fatalf("error %q does not classify owner-read dependency", err.Error())
	}
}

func TestValidateServiceIsolationAllowsOwnerReadWithServiceKey(t *testing.T) {
	app := NewApp(Config{
		ServiceName:   "widget-service",
		ServiceURLs:   map[string]string{"identity-service": "http://identity-service"},
		ServiceAPIKey: "service-key",
	})
	app.RegisterOwnerReadDependencies("widget-service", testIdentityUsersResource)

	if err := app.ValidateServiceIsolation(); err != nil {
		t.Fatalf("configured owner read should pass isolation validation: %v", err)
	}
}

func TestValidateServiceIsolationRejectsOwnerReadWithoutDomainContract(t *testing.T) {
	app := NewApp(Config{
		ServiceName:   "widget-service",
		ServiceURLs:   map[string]string{"identity-service": "http://identity-service"},
		ServiceAPIKey: "service-key",
	})
	app.RegisterOwnerReadDependencies("widget-service", testIdentitySessions)

	if err := app.ValidateServiceIsolation(); err == nil {
		t.Fatal("expected uncontracted owner read to remain an isolation gap")
	}
}
