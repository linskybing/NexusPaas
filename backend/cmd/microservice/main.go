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
	cfg := platform.ConfigFromEnv()
	platform.ConfigureLogging(cfg)
	if err := cfg.Validate(); err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}
	if task := os.Getenv("ADMIN_TASK"); task != "" {
		if err := platform.RunAdminTask(task, cfg); err != nil {
			slog.Error("admin task failed", "task", task, "error", err)
			os.Exit(1)
		}
		return
	}

	shutdownTracing, err := platform.InitTracing(context.Background(), cfg)
	if err != nil {
		slog.Error("failed to initialize tracing", "error", err)
		os.Exit(1)
	}

	backing, err := platform.NewBackingResources(context.Background(), cfg)
	if err != nil {
		slog.Error("failed to connect backing services", "error", err)
		os.Exit(1)
	}
	defer backing.Close()

	app := platform.NewApp(cfg, backing.Options...)
	services.RegisterAll(app)

	// Run registered maintenance tasks (e.g. expired-credential cleanup) on an
	// interval, lease-gated so only one replica acts each cycle (finding 1).
	maintenanceCtx, stopMaintenance := context.WithCancel(context.Background())
	defer stopMaintenance()
	app.StartMaintenance(maintenanceCtx, cfg.MaintenanceInterval)

	if !startupChecksPass(cfg,
		startupCheck{err: app.ValidateServiceIsolation(), failureMessage: "service isolation check failed", warningMessage: "service isolation gaps detected (non-production)"},
		startupCheck{err: app.ValidateAdminCoverage(), failureMessage: "admin route coverage check failed", warningMessage: "admin route coverage gaps detected (non-production)"},
		startupCheck{err: app.ValidatePolicyDecisionPoint(), failureMessage: "policy decision point check failed", warningMessage: "policy decision point is not production-ready"},
	) {
		os.Exit(1)
	}

	// otelhttp.NewHandler extracts inbound W3C trace context and opens a server
	// span per request so every microservice is observable out of the box.
	handler := otelhttp.NewHandler(app, "nexuspaas-http",
		otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
			return r.Method + " " + r.URL.Path
		}),
	)

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		slog.Info("microservice listening", "addr", cfg.HTTPAddr, "service", cfg.ServiceName)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("graceful shutdown failed", "error", err)
		os.Exit(1)
	}
	if err := shutdownTracing(ctx); err != nil {
		slog.Error("tracing shutdown failed", "error", err)
	}
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
