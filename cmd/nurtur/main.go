package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"nurtur"
	"nurtur/internal/clock"
	"nurtur/internal/config"
	"nurtur/internal/server"
	"nurtur/internal/store"
)

// version is set with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "nurtur:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	localOnly := cfg.LocalOnly || cfg.TursoURL == "" || cfg.TursoToken == ""
	st, err := store.Open(ctx, store.Options{
		Path:      cfg.DBPath,
		LocalOnly: localOnly,
		RemoteURL: cfg.TursoURL,
		AuthToken: cfg.TursoToken,
		Logger:    logger,
	})
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	migrations, err := fs.Sub(nurtur.Assets, "migrations")
	if err != nil {
		return fmt.Errorf("migrations: %w", err)
	}
	if err := st.Migrate(ctx, migrations, clock.Real{}.Now()); err != nil {
		return err
	}

	var worker *store.SyncWorker
	if st.SyncDB() != nil {
		worker = store.NewSyncWorker(
			store.NewTursoSyncer(st.SyncDB()),
			store.DefaultSyncConfig(cfg.SyncInterval),
			logger,
		)
		st.SetOnChange(worker.Notify)
		worker.Start(ctx)
		// Flush any local changes left behind by a previous process.
		worker.Notify()
		defer func() {
			flushCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := worker.Close(flushCtx); err != nil {
				logger.Warn("sync worker did not close cleanly", "error", err)
			}
		}()
	}

	srv, err := server.New(server.Config{
		Store:         st,
		Clock:         clock.Real{},
		Location:      cfg.Location,
		BabyName:      cfg.BabyName,
		Version:       version,
		Assets:        nurtur.Assets,
		Migrations:    migrations,
		Logger:        logger,
		SyncWorker:    worker,
		LocalOnly:     st.SyncDB() == nil,
		NightStart:    cfg.NightStart,
		NightEnd:      cfg.NightEnd,
		Passcode:      cfg.Passcode,
		SessionSecret: cfg.SessionSecret,
		SecureCookie:  cfg.SecureCookie,
	})
	if err != nil {
		return err
	}

	httpServer := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	mode := "sync"
	if st.SyncDB() == nil {
		mode = "local"
	}
	logger.Info("nurtur starting",
		"addr", cfg.Addr,
		"db", cfg.DBPath,
		"mode", mode,
		"tz", cfg.Location.String(),
		"version", version,
	)

	errCh := make(chan error, 1)
	go func() {
		err := httpServer.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return fmt.Errorf("http server: %w", err)
	case <-ctx.Done():
		logger.Info("shutdown requested")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Warn("http shutdown failed", "error", err)
	}
	if worker != nil {
		if err := worker.Close(shutdownCtx); err != nil {
			logger.Warn("sync shutdown failed", "error", err)
		}
	}
	if err := st.Close(); err != nil {
		logger.Warn("database close failed", "error", err)
	}
	logger.Info("nurtur stopped")
	return nil
}
