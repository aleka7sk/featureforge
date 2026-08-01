// Package main is FeatureForge's sole composition root (FF-018 §5.2, §14).
// It reads configuration, selects one persistence adapter, builds the HTTP
// handler, and owns the server's lifecycle. It contains no application,
// transport, domain, or engineering logic of its own -- only wiring.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aleka7sk/featureforge/internal/application"
	"github.com/aleka7sk/featureforge/internal/engineering/peos"
	"github.com/aleka7sk/featureforge/internal/infrastructure/memory"
	"github.com/aleka7sk/featureforge/internal/infrastructure/postgres"
	"github.com/aleka7sk/featureforge/internal/proposal"
	transporthttp "github.com/aleka7sk/featureforge/internal/transport/http"
	"github.com/aleka7sk/featureforge/internal/ui"
)

func main() {
	logger := slog.Default()
	if err := run(logger); err != nil {
		logger.Error("featureforge: fatal", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	addr := getenv("FEATUREFORGE_ADDR", "127.0.0.1:8080")
	adapter := getenv("FEATUREFORGE_ADAPTER", "memory")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	uow, closeAdapter, err := buildAdapter(ctx, adapter, logger)
	if err != nil {
		return err
	}
	defer closeAdapter()

	recorder := peos.NewRecorder()
	generator := proposal.NewDeterministicGenerator()
	if err := application.EnsureLifecycleConfiguration(ctx, uow, recorder, recorder); err != nil {
		return fmt.Errorf("initialize lifecycle configuration: %w", err)
	}
	deps := transporthttp.Dependencies{
		UOW:       uow,
		Recorder:  recorder,
		Inspector: recorder,
		Projector: recorder,
		Generator: generator,
		Clock:     application.SystemClock{},
		Logger:    logger,
	}
	apiHandler := transporthttp.NewHandler(deps)
	uiHandler := ui.NewHandler(ui.Dependencies{API: apiHandler, Logger: logger})

	// One root mux composes the two independently-built handlers under
	// disjoint prefixes (FF-021 §15): /api/v1/ keeps serving exactly what
	// FF-018 built and tested, unmodified and independently servable;
	// everything else reaches the UI, which reaches the API only in-process
	// through apiHandler (AD-028) -- never a second network hop.
	rootMux := http.NewServeMux()
	rootMux.Handle("/api/v1/", apiHandler)
	rootMux.Handle("/", uiHandler)

	server := &http.Server{
		Addr:              addr,
		Handler:           rootMux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	serveErr := make(chan error, 1)
	go func() {
		logger.Info("featureforge: listening", "addr", addr, "adapter", adapter)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
			return
		}
		serveErr <- nil
	}()

	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return err
	}
	return <-serveErr
}

// buildAdapter selects and constructs the one persistence adapter §14.1
// names, returning its UnitOfWork and a cleanup func releasing whatever
// resources it holds. PostgreSQL migration runs on every start -- Migrate is
// idempotent, built exactly for this (§14.2 step 2) -- so the binary is
// always runnable against a fresh database with no separate migration step.
// The returned pool's type is never named here: postgres.Connect's return
// is carried only through type inference and postgres.Migrate/NewUnitOfWork,
// so this file needs no direct pgx import (open question N1, resolved).
func buildAdapter(ctx context.Context, adapter string, logger *slog.Logger) (application.UnitOfWork, func(), error) {
	switch adapter {
	case "memory":
		return memory.NewUnitOfWork(memory.NewStore()), func() {}, nil
	case "postgres":
		dsn := os.Getenv("FEATUREFORGE_POSTGRES_DSN")
		if dsn == "" {
			return nil, nil, errors.New("featureforge: FEATUREFORGE_POSTGRES_DSN is required when FEATUREFORGE_ADAPTER=postgres")
		}
		pool, err := postgres.Connect(ctx, dsn)
		if err != nil {
			return nil, nil, err
		}
		if err := postgres.Migrate(ctx, pool); err != nil {
			pool.Close()
			return nil, nil, err
		}
		logger.Info("featureforge: migrations applied")
		return postgres.NewUnitOfWork(pool), pool.Close, nil
	default:
		return nil, nil, errors.New("featureforge: unknown FEATUREFORGE_ADAPTER " + adapter + " (want memory or postgres)")
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
