package main

import (
	"errors"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services"
)

func TestStartupChecksPassesWhenAllChecksPass(t *testing.T) {
	if !startupChecksPass(platform.Config{Production: true}, startupCheck{}) {
		t.Fatal("expected passing checks to pass startup")
	}
}

func TestStartupChecksWarnsButPassesOutsideProduction(t *testing.T) {
	err := errors.New("gap")
	if !startupChecksPass(platform.Config{}, startupCheck{err: err, warningMessage: "warn"}) {
		t.Fatal("expected non-production startup check gap to warn and pass")
	}
}

func TestStartupChecksFailProduction(t *testing.T) {
	err := errors.New("gap")
	if startupChecksPass(platform.Config{Production: true}, startupCheck{err: err, failureMessage: "fail"}) {
		t.Fatal("expected production startup check gap to fail")
	}
}

func TestStartupChecksFailProductionForRegisteredServiceIsolationGap(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: "scheduler-quota-service", Production: true})
	services.RegisterAll(app)

	if startupChecksPass(app.Config, startupCheck{err: app.ValidateServiceIsolation(), failureMessage: "fail"}) {
		t.Fatal("expected registered service isolation gap to fail production startup")
	}
}

func TestStartupChecksWarnOutsideProductionForRegisteredServiceIsolationGap(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: "scheduler-quota-service"})
	services.RegisterAll(app)

	if !startupChecksPass(app.Config, startupCheck{err: app.ValidateServiceIsolation(), warningMessage: "warn"}) {
		t.Fatal("expected registered service isolation gap to warn and pass outside production")
	}
}
