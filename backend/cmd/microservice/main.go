package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

func main() {
	if code := runMicroservice(context.Background(), defaultMicroserviceDeps()); code != 0 {
		os.Exit(code)
	}
}

type microserviceBacking struct {
	options []platform.Option
	close   func()
}

type microserviceServer interface {
	ListenAndServe() error
	Shutdown(context.Context) error
}

type microserviceDeps struct {
	configFromEnv       func() platform.Config
	configureLogging    func(platform.Config)
	adminTaskName       func() string
	runAdminTask        func(string, platform.Config) error
	initTracing         func(context.Context, platform.Config) (func(context.Context) error, error)
	newBackingResources func(context.Context, platform.Config) (microserviceBacking, error)
	newApp              func(platform.Config, ...platform.Option) *platform.App
	registerServices    func(*platform.App)
	newServer           func(platform.Config, http.Handler) microserviceServer
	notifySignals       func() chan os.Signal
	stopSignals         func(chan os.Signal)
}

func defaultMicroserviceDeps() microserviceDeps {
	return microserviceDeps{
		configFromEnv:    platform.ConfigFromEnv,
		configureLogging: platform.ConfigureLogging,
		adminTaskName:    func() string { return os.Getenv("ADMIN_TASK") },
		runAdminTask:     platform.RunAdminTask,
		initTracing:      platform.InitTracing,
		newBackingResources: func(ctx context.Context, cfg platform.Config) (microserviceBacking, error) {
			backing, err := platform.NewBackingResources(ctx, cfg)
			if err != nil {
				return microserviceBacking{}, err
			}
			return microserviceBacking{options: backing.Options, close: backing.Close}, nil
		},
		newApp:           platform.NewApp,
		registerServices: services.RegisterAll,
		newServer: func(cfg platform.Config, handler http.Handler) microserviceServer {
			return &http.Server{
				Addr:              cfg.HTTPAddr,
				Handler:           handler,
				ReadHeaderTimeout: 5 * time.Second,
			}
		},
		notifySignals: func() chan os.Signal {
			stop := make(chan os.Signal, 1)
			signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
			return stop
		},
		stopSignals: func(stop chan os.Signal) { signal.Stop(stop) },
	}
}

func runMicroservice(ctx context.Context, deps microserviceDeps) int {
	deps = normalizeMicroserviceDeps(deps)
	cfg := deps.configFromEnv()
	deps.configureLogging(cfg)
	if err := cfg.Validate(); err != nil {
		slog.Error("invalid configuration", "error", err)
		return 1
	}
	if task := deps.adminTaskName(); task != "" {
		if err := deps.runAdminTask(task, cfg); err != nil {
			slog.Error("admin task failed", "task", task, "error", err)
			return 1
		}
		return 0
	}

	shutdownTracing, err := deps.initTracing(ctx, cfg)
	if err != nil {
		slog.Error("failed to initialize tracing", "error", err)
		return 1
	}

	backing, err := deps.newBackingResources(ctx, cfg)
	if err != nil {
		slog.Error("failed to connect backing services", "error", err)
		return 1
	}
	if backing.close != nil {
		defer backing.close()
	}

	app := deps.newApp(cfg, backing.options...)
	deps.registerServices(app)

	// Run registered maintenance tasks (e.g. expired-credential cleanup) on an
	// interval, lease-gated so only one replica acts each cycle (finding 1).
	maintenanceCtx, stopMaintenance := context.WithCancel(ctx)
	defer stopMaintenance()
	app.StartMaintenance(maintenanceCtx, cfg.MaintenanceInterval)

	if !startupChecksPass(cfg,
		startupCheck{err: app.ValidateServiceIsolation(), failureMessage: "service isolation check failed", warningMessage: "service isolation gaps detected (non-production)"},
		startupCheck{err: app.ValidateAdminCoverage(), failureMessage: "admin route coverage check failed", warningMessage: "admin route coverage gaps detected (non-production)"},
		startupCheck{err: app.ValidatePolicyDecisionPoint(), failureMessage: "policy decision point check failed", warningMessage: "policy decision point is not production-ready"},
		startupCheck{err: app.ValidateRouteCollisions(), failureMessage: "route collision check failed", warningMessage: "route collision gaps detected (non-production)"},
		startupCheck{err: app.ValidateInternalRouteAuth(), failureMessage: "internal route auth check failed", warningMessage: "internal route auth gaps detected (non-production)"},
	) {
		return 1
	}

	// otelhttp.NewHandler extracts inbound W3C trace context and opens a server
	// span per request so every microservice is observable out of the box.
	handler := otelhttp.NewHandler(app, "nexuspaas-http",
		otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
			return r.Method + " " + r.URL.Path
		}),
	)

	srv := deps.newServer(cfg, handler)

	serveErr := make(chan error, 1)
	go func() {
		slog.Info("microservice listening", "addr", cfg.HTTPAddr, "service", cfg.ServiceName)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
	}()

	signals := deps.notifySignals()
	defer deps.stopSignals(signals)

	select {
	case err := <-serveErr:
		slog.Error("server failed", "error", err)
		return 1
	case <-signals:
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("graceful shutdown failed", "error", err)
		return 1
	}
	if shutdownTracing != nil {
		if err := shutdownTracing(shutdownCtx); err != nil {
			slog.Error("tracing shutdown failed", "error", err)
		}
	}
	return 0
}

func normalizeMicroserviceDeps(deps microserviceDeps) microserviceDeps {
	defaults := defaultMicroserviceDeps()
	if deps.configFromEnv == nil {
		deps.configFromEnv = defaults.configFromEnv
	}
	if deps.configureLogging == nil {
		deps.configureLogging = defaults.configureLogging
	}
	if deps.adminTaskName == nil {
		deps.adminTaskName = defaults.adminTaskName
	}
	if deps.runAdminTask == nil {
		deps.runAdminTask = defaults.runAdminTask
	}
	if deps.initTracing == nil {
		deps.initTracing = defaults.initTracing
	}
	if deps.newBackingResources == nil {
		deps.newBackingResources = defaults.newBackingResources
	}
	if deps.newApp == nil {
		deps.newApp = defaults.newApp
	}
	if deps.registerServices == nil {
		deps.registerServices = defaults.registerServices
	}
	if deps.newServer == nil {
		deps.newServer = defaults.newServer
	}
	if deps.notifySignals == nil {
		deps.notifySignals = defaults.notifySignals
	}
	if deps.stopSignals == nil {
		deps.stopSignals = defaults.stopSignals
	}
	return deps
}

type startupCheck struct {
	err            error
	failureMessage string
	warningMessage string
}

func startupChecksPass(cfg platform.Config, checks ...startupCheck) bool {
	for _, check := range checks {
		if check.err == nil {
			continue
		}
		if cfg.Production {
			slog.Error(check.failureMessage, "error", check.err)
			return false
		}
		slog.Warn(check.warningMessage, "error", check.err)
	}
	return true
}
