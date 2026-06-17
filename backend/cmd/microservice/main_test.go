package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"sync"
	"syscall"
	"testing"
	"time"

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

func TestRunMicroserviceReturnsOneForInvalidConfig(t *testing.T) {
	calledTracing := false
	deps, _ := newTestRunDeps(validRunConfig())
	deps.configFromEnv = func() platform.Config {
		cfg := validRunConfig()
		cfg.DevHeaderAuth = false
		return cfg
	}
	deps.initTracing = func(context.Context, platform.Config) (func(context.Context) error, error) {
		calledTracing = true
		return nil, nil
	}

	if code := runMicroservice(context.Background(), deps); code != 1 {
		t.Fatalf("runMicroservice invalid config code = %d, want 1", code)
	}
	if calledTracing {
		t.Fatal("tracing should not start after config validation failure")
	}
}

func TestRunMicroserviceRunsAdminTaskAndReturns(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want int
	}{
		{name: "success", want: 0},
		{name: "failure", err: errors.New("denied"), want: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calledTask := ""
			calledTracing := false
			deps, _ := newTestRunDeps(validRunConfig())
			deps.adminTaskName = func() string { return "ensure-object-store-bucket" }
			deps.runAdminTask = func(task string, _ platform.Config) error {
				calledTask = task
				return tc.err
			}
			deps.initTracing = func(context.Context, platform.Config) (func(context.Context) error, error) {
				calledTracing = true
				return nil, nil
			}

			if code := runMicroservice(context.Background(), deps); code != tc.want {
				t.Fatalf("runMicroservice admin code = %d, want %d", code, tc.want)
			}
			if calledTask != "ensure-object-store-bucket" {
				t.Fatalf("admin task = %q, want ensure-object-store-bucket", calledTask)
			}
			if calledTracing {
				t.Fatal("admin task path should return before tracing starts")
			}
		})
	}
}

func TestRunMicroserviceReturnsOneForTracingAndBackingFailures(t *testing.T) {
	t.Run("tracing", func(t *testing.T) {
		deps, _ := newTestRunDeps(validRunConfig())
		deps.initTracing = func(context.Context, platform.Config) (func(context.Context) error, error) {
			return nil, errors.New("trace unavailable")
		}

		if code := runMicroservice(context.Background(), deps); code != 1 {
			t.Fatalf("runMicroservice tracing failure code = %d, want 1", code)
		}
	})

	t.Run("backing", func(t *testing.T) {
		deps, harness := newTestRunDeps(validRunConfig())
		deps.newBackingResources = func(context.Context, platform.Config) (microserviceBacking, error) {
			harness.backingRequested = true
			return microserviceBacking{}, errors.New("database unavailable")
		}

		if code := runMicroservice(context.Background(), deps); code != 1 {
			t.Fatalf("runMicroservice backing failure code = %d, want 1", code)
		}
		if !harness.backingRequested {
			t.Fatal("expected backing resources to be requested")
		}
	})
}

func TestRunMicroserviceReturnsOneWhenServerFails(t *testing.T) {
	deps, harness := newTestRunDeps(validRunConfig())
	harness.server.listenErr = errors.New("bind failed")

	if code := runMicroservice(context.Background(), deps); code != 1 {
		t.Fatalf("runMicroservice listen failure code = %d, want 1", code)
	}
	if !harness.server.listenStarted() {
		t.Fatal("expected server to start listening")
	}
	if harness.server.shutdownCalled() {
		t.Fatal("server shutdown should not be called after listen failure")
	}
}

func TestRunMicroserviceSignalShutdownClosesResourcesAndTracing(t *testing.T) {
	deps, harness := newTestRunDeps(validRunConfig())
	harness.signals <- syscall.SIGTERM

	if code := runMicroservice(context.Background(), deps); code != 0 {
		t.Fatalf("runMicroservice shutdown code = %d, want 0", code)
	}
	if !harness.server.listenStarted() {
		t.Fatal("expected server to start")
	}
	if !harness.server.shutdownCalled() {
		t.Fatal("expected graceful server shutdown")
	}
	if !harness.tracingShutdown {
		t.Fatal("expected tracing shutdown")
	}
	if !harness.backingClosed {
		t.Fatal("expected backing resources to close")
	}
	if !harness.signalsStopped {
		t.Fatal("expected signal notifications to stop")
	}
}

func TestRunMicroserviceReturnsOneWhenShutdownFails(t *testing.T) {
	deps, harness := newTestRunDeps(validRunConfig())
	harness.server.shutdownErr = errors.New("shutdown timeout")
	harness.signals <- syscall.SIGTERM

	if code := runMicroservice(context.Background(), deps); code != 1 {
		t.Fatalf("runMicroservice shutdown failure code = %d, want 1", code)
	}
	if !harness.backingClosed {
		t.Fatal("backing resources should close on shutdown failure")
	}
}

func TestRunMicroserviceContextCancellationShutsDown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	deps, harness := newTestRunDeps(validRunConfig())
	cancel()

	if code := runMicroservice(ctx, deps); code != 0 {
		t.Fatalf("runMicroservice canceled context code = %d, want 0", code)
	}
	if !harness.server.shutdownCalled() {
		t.Fatal("expected context cancellation to trigger shutdown")
	}
}

func TestNormalizeMicroserviceDepsFillsProductionDefaults(t *testing.T) {
	t.Setenv("ADMIN_TASK", "configured-task")

	deps := normalizeMicroserviceDeps(microserviceDeps{})
	if deps.configFromEnv == nil ||
		deps.configureLogging == nil ||
		deps.adminTaskName == nil ||
		deps.runAdminTask == nil ||
		deps.initTracing == nil ||
		deps.newBackingResources == nil ||
		deps.newApp == nil ||
		deps.registerServices == nil ||
		deps.newServer == nil ||
		deps.notifySignals == nil ||
		deps.stopSignals == nil {
		t.Fatal("normalizeMicroserviceDeps left a dependency unset")
	}
	if task := deps.adminTaskName(); task != "configured-task" {
		t.Fatalf("admin task default = %q, want configured-task", task)
	}

	server := deps.newServer(validRunConfig(), http.NotFoundHandler())
	if server == nil {
		t.Fatal("default server factory returned nil")
	}
	signals := deps.notifySignals()
	if signals == nil {
		t.Fatal("default signal notifier returned nil")
	}
	deps.stopSignals(signals)
}

type runHarness struct {
	server           *fakeMicroserviceServer
	signals          chan os.Signal
	backingRequested bool
	backingClosed    bool
	tracingShutdown  bool
	signalsStopped   bool
}

func newTestRunDeps(cfg platform.Config) (microserviceDeps, *runHarness) {
	harness := &runHarness{
		server:  newFakeMicroserviceServer(),
		signals: make(chan os.Signal, 1),
	}
	deps := microserviceDeps{
		configFromEnv:    func() platform.Config { return cfg },
		configureLogging: func(platform.Config) {},
		adminTaskName:    func() string { return "" },
		runAdminTask:     func(string, platform.Config) error { return nil },
		initTracing: func(context.Context, platform.Config) (func(context.Context) error, error) {
			return func(context.Context) error {
				harness.tracingShutdown = true
				return nil
			}, nil
		},
		newBackingResources: func(context.Context, platform.Config) (microserviceBacking, error) {
			harness.backingRequested = true
			return microserviceBacking{
				close: func() { harness.backingClosed = true },
			}, nil
		},
		newApp: func(cfg platform.Config, opts ...platform.Option) *platform.App {
			return platform.NewApp(cfg, opts...)
		},
		registerServices: func(*platform.App) {},
		newServer: func(platform.Config, http.Handler) microserviceServer {
			return harness.server
		},
		notifySignals: func() chan os.Signal { return harness.signals },
		stopSignals: func(chan os.Signal) {
			harness.signalsStopped = true
		},
	}
	return deps, harness
}

func validRunConfig() platform.Config {
	return platform.Config{
		HTTPAddr:                  "127.0.0.1:0",
		RequireAuth:               false,
		DevHeaderAuth:             true,
		APIKeys:                   map[string]bool{},
		APIKeyPrincipals:          map[string]platform.APIKeyPrincipal{},
		AllowedOrigins:            map[string]bool{},
		JWTAudiences:              map[string]bool{},
		ServiceURLs:               map[string]string{},
		ShutdownTimeout:           50 * time.Millisecond,
		MaintenanceInterval:       time.Hour,
		AdapterTimeout:            50 * time.Millisecond,
		LonghornRWXHealthInterval: time.Hour,
		LonghornRWXRepairCooldown: time.Hour,
		PriorityClassSyncInterval: time.Hour,
	}
}

type fakeMicroserviceServer struct {
	listenErr   error
	shutdownErr error

	started     chan struct{}
	shutdown    chan struct{}
	startOnce   sync.Once
	shutdownMux sync.Mutex
	didShutdown bool
	closeOnce   sync.Once
}

func newFakeMicroserviceServer() *fakeMicroserviceServer {
	return &fakeMicroserviceServer{
		started:  make(chan struct{}),
		shutdown: make(chan struct{}),
	}
}

func (s *fakeMicroserviceServer) ListenAndServe() error {
	s.startOnce.Do(func() { close(s.started) })
	if s.listenErr != nil {
		return s.listenErr
	}
	<-s.shutdown
	return http.ErrServerClosed
}

func (s *fakeMicroserviceServer) Shutdown(context.Context) error {
	s.shutdownMux.Lock()
	s.didShutdown = true
	s.shutdownMux.Unlock()
	s.closeOnce.Do(func() { close(s.shutdown) })
	return s.shutdownErr
}

func (s *fakeMicroserviceServer) listenStarted() bool {
	select {
	case <-s.started:
		return true
	case <-time.After(time.Second):
		return false
	}
}

func (s *fakeMicroserviceServer) shutdownCalled() bool {
	s.shutdownMux.Lock()
	defer s.shutdownMux.Unlock()
	return s.didShutdown
}
