package platform

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"
)

func TestRunMaintenanceExecutesLeaseGatedTasks(t *testing.T) {
	app := NewApp(Config{})
	runs := 0
	app.RegisterMaintenanceTask("job", func(context.Context) error { runs++; return nil })

	app.runMaintenance(context.Background(), time.Minute)
	if runs != 1 {
		t.Fatalf("task runs = %d, want 1", runs)
	}
}

func TestRunMaintenanceSkipsWhenLeaseNotAcquired(t *testing.T) {
	app := NewApp(Config{}, WithLeases(denyLease{}))
	runs := 0
	app.RegisterMaintenanceTask("job", func(context.Context) error { runs++; return nil })

	app.runMaintenance(context.Background(), time.Minute)
	if runs != 0 {
		t.Fatalf("task runs = %d, want 0 when lease denied", runs)
	}
}

func TestRunMaintenanceContinuesAfterTaskError(t *testing.T) {
	app := NewApp(Config{})
	second := false
	app.RegisterMaintenanceTask("a", func(context.Context) error { return errors.New("boom") })
	app.RegisterMaintenanceTask("b", func(context.Context) error { second = true; return nil })

	app.runMaintenance(context.Background(), time.Minute)
	if !second {
		t.Fatal("second task should run even after the first errors")
	}
}

func TestRegisterMaintenanceTaskForServiceAllowsHostedOwner(t *testing.T) {
	app := NewApp(Config{ServiceName: "identity-service"})
	runs := 0
	app.RegisterMaintenanceTaskForService("identity-service", " identity-cleanup ", func(context.Context) error { runs++; return nil })

	if got, want := app.MaintenanceTaskNames(), []string{"identity-cleanup"}; !slices.Equal(got, want) {
		t.Fatalf("maintenance tasks = %v, want %v", got, want)
	}
	app.runMaintenance(context.Background(), time.Minute)
	if runs != 1 {
		t.Fatalf("task runs = %d, want 1", runs)
	}
}

func TestRegisterMaintenanceTaskForServiceSkipsUnhostedOwner(t *testing.T) {
	app := NewApp(Config{ServiceName: "identity-service"})
	runs := 0
	app.RegisterMaintenanceTaskForService("workload-service", "workload-dispatcher", func(context.Context) error { runs++; return nil })

	if got := app.MaintenanceTaskNames(); len(got) != 0 {
		t.Fatalf("maintenance tasks = %v, want none", got)
	}
	app.runMaintenance(context.Background(), time.Minute)
	if runs != 0 {
		t.Fatalf("task runs = %d, want 0", runs)
	}
}

func TestRegisterMaintenanceTaskForServiceAllowsCoHostedApp(t *testing.T) {
	app := NewApp(Config{ServiceName: "all"})
	app.RegisterMaintenanceTaskForService("identity-service", "identity-auth-cleanup", func(context.Context) error { return nil })
	app.RegisterMaintenanceTaskForService("workload-service", "workload-dispatcher", func(context.Context) error { return nil })

	if got, want := app.MaintenanceTaskNames(), []string{"identity-auth-cleanup", "workload-dispatcher"}; !slices.Equal(got, want) {
		t.Fatalf("maintenance tasks = %v, want %v", got, want)
	}
}

type denyLease struct{}

func (denyLease) Acquire(context.Context, string, string, time.Duration) (bool, error) {
	return false, nil
}
func (denyLease) Release(context.Context, string, string) error { return nil }
