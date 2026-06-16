package platform

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"sort"
	"strings"
	"time"
)

// newInstanceID returns a random per-process identity used as the worker id when
// acquiring maintenance leases, so that across replicas only one process holds a
// given task's lease per interval.
func newInstanceID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "instance"
	}
	return hex.EncodeToString(b[:])
}

// maintenanceTask is a periodic background job (e.g. expired-credential cleanup)
// registered by a service and run by the composition root.
type maintenanceTask struct {
	name string
	run  func(context.Context) error
}

// RegisterMaintenanceTask registers a periodic background task. Tasks are only
// executed once StartMaintenance is called from the composition root, so unit
// tests that build an App never spawn background goroutines.
func (a *App) RegisterMaintenanceTask(name string, run func(context.Context) error) {
	a.registerMaintenanceTask(name, run)
}

// RegisterMaintenanceTaskForService registers a service-owned periodic task
// only when the current process hosts that service. This keeps isolated
// SERVICE_NAME processes from running unrelated workers while preserving
// co-hosted SERVICE_NAME=all behavior through Config.AllowsService.
func (a *App) RegisterMaintenanceTaskForService(owner, name string, run func(context.Context) error) {
	owner = strings.TrimSpace(owner)
	if owner == "" || !a.Config.AllowsService(owner) {
		return
	}
	a.registerMaintenanceTask(name, run)
}

func (a *App) registerMaintenanceTask(name string, run func(context.Context) error) {
	name = strings.TrimSpace(name)
	if name == "" || run == nil {
		return
	}
	a.maintenanceTasks = append(a.maintenanceTasks, maintenanceTask{name: name, run: run})
}

// MaintenanceTaskNames returns the currently registered maintenance task names
// in deterministic order for runtime-isolation tests.
func (a *App) MaintenanceTaskNames() []string {
	names := make([]string, 0, len(a.maintenanceTasks))
	for _, task := range a.maintenanceTasks {
		names = append(names, task.name)
	}
	sort.Strings(names)
	return names
}

// StartMaintenance runs the registered maintenance tasks every interval until ctx
// is cancelled. It returns immediately; the work happens in a single background
// goroutine. Call once from main after services are registered.
func (a *App) StartMaintenance(ctx context.Context, interval time.Duration) {
	if len(a.maintenanceTasks) == 0 || interval <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			a.runMaintenance(ctx, interval)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

// RunMaintenanceOnce executes every registered maintenance task a single time,
// lease-gated exactly as the periodic loop does. It exists so service tests can
// exercise their registered workers deterministically without spawning the
// background goroutine that StartMaintenance creates.
func (a *App) RunMaintenanceOnce(ctx context.Context, ttl time.Duration) {
	a.runMaintenance(ctx, ttl)
}

// runMaintenance executes each registered task once, lease-gated so only one
// replica runs a given task per interval (multi-process coordination). It is the
// directly-testable core of StartMaintenance.
func (a *App) runMaintenance(ctx context.Context, ttl time.Duration) {
	for _, task := range a.maintenanceTasks {
		// Shard per task name so distinct tasks don't contend; the per-process
		// instance id is the worker so only one replica wins each task's lease.
		acquired, err := a.Leases.Acquire(ctx, a.instanceID, "maintenance:"+task.name, ttl)
		if err != nil || !acquired {
			continue
		}
		if err := task.run(ctx); err != nil {
			slog.Warn("maintenance task failed", "task", task.name, "error", err)
		}
	}
}
