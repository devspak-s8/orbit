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

	"github.com/orbit-org/orbit/server/internal/automation"
	"github.com/orbit-org/orbit/server/internal/config"
	"github.com/orbit-org/orbit/server/internal/db"
	"github.com/orbit-org/orbit/server/internal/demo"
	"github.com/orbit-org/orbit/server/internal/events"
	"github.com/orbit-org/orbit/server/internal/httpapi"
	"github.com/orbit-org/orbit/server/internal/identity"
	"github.com/orbit-org/orbit/server/internal/modules"
	"github.com/orbit-org/orbit/server/internal/work"
	"github.com/orbit-org/orbit/server/internal/workspaces"
)

func main() {
	cfg := config.Load()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("connecting to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := db.Migrate(ctx, cfg.DatabaseURL); err != nil {
		logger.Error("running migrations", "error", err)
		os.Exit(1)
	}
	logger.Info("database ready")

	// Module catalog: any invalid embedded manifest fails the boot.
	catalog, err := modules.LoadCatalog()
	if err != nil {
		logger.Error("loading module catalog", "error", err)
		os.Exit(1)
	}

	ident := identity.NewService(pool, cfg.SessionTTL)
	ws := workspaces.NewService(pool)
	mods := modules.NewService(pool, catalog)
	eventsSvc := events.NewService(pool)
	demoSvc := demo.NewService(pool, eventsSvc, mods)
	mods.RegisterUninstallHook("orbit.demo", demoSvc.Cleanup)
	automationSvc := automation.NewService(pool)
	runner := automation.NewRunner(pool)
	workSvc := work.NewService(pool, eventsSvc, mods)

	if err := mods.Seed(ctx); err != nil {
		logger.Error("seeding module registry", "error", err)
		os.Exit(1)
	}
	mods.RegisterUninstallHook(work.ModuleID, workSvc.Cleanup)

	// The automation worker runs in-process for the alpha (ADR-0004);
	// its queue is durable so a separate process can take over later.
	go runner.Run(ctx)

	svcs := httpapi.Services{
		Identity:   ident,
		Workspaces: ws,
		Modules:    mods,
		Demo:       demoSvc,
		Events:     eventsSvc,
		Automation: automationSvc,
		Work:       workSvc,
	}

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           httpapi.NewRouter(pool, logger, cfg.CORSOrigins, svcs),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logger.Info("orbit api listening", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server error", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown", "error", err)
	}
}
