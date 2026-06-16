package platform

import "sync"

type RollbackMetrics struct {
	OutboxLag        int
	ErrorRatePercent int
	DegradedAdapters int
}

type RollbackGate struct {
	MaxOutboxLag        int
	MaxErrorRatePercent int
	MaxDegradedAdapters int
}

func DefaultRollbackGate() RollbackGate {
	return RollbackGate{MaxOutboxLag: 100, MaxErrorRatePercent: 5, MaxDegradedAdapters: 0}
}

func (g RollbackGate) Allows(metrics RollbackMetrics) bool {
	return metrics.OutboxLag <= g.MaxOutboxLag &&
		metrics.ErrorRatePercent <= g.MaxErrorRatePercent &&
		metrics.DegradedAdapters <= g.MaxDegradedAdapters
}

func (a *App) RollbackTargetFor(route RouteSpec) string {
	return a.Switches.TargetFor(route)
}

func (a *App) RollbackMetrics() RollbackMetrics {
	return RollbackMetrics{
		OutboxLag:        a.Events.Lag("rollback-gate"),
		ErrorRatePercent: a.Metrics.ErrorRatePercent(),
		DegradedAdapters: a.Metrics.CounterSuffix("_degraded"),
	}
}

func (a *App) CanRollback(gate RollbackGate) bool {
	return gate.Allows(a.RollbackMetrics())
}

type RouteSwitches struct {
	mu      sync.RWMutex
	targets map[string]string
}

func NewRouteSwitches() *RouteSwitches {
	return &RouteSwitches{targets: map[string]string{}}
}

func (s *RouteSwitches) Enable(pattern, service string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.targets[pattern] = service
}

func (s *RouteSwitches) Rollback(pattern string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.targets[pattern] = "monolith"
}

func (s *RouteSwitches) Target(pattern string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if target := s.targets[pattern]; target != "" {
		return target
	}
	return "service"
}

func (s *RouteSwitches) TargetFor(route RouteSpec) string {
	if target := s.Target(route.Pattern); target != "service" {
		return target
	}
	if route.ExternalAdapter == "monolith" {
		return "monolith"
	}
	if route.Pattern != "" {
		return "service"
	}
	return "unknown"
}
