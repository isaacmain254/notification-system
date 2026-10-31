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

	"github.com/isaacmain254/notification-system/internal/config"
	"github.com/isaacmain254/notification-system/internal/database"
	apphttp "github.com/isaacmain254/notification-system/internal/http"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg, err := config.LoadConfig()

	if err != nil {
		logger.Error("load configuration", "error", err)
		os.Exit(1)
	}
	startupCtx, startupCancel := context.WithTimeout(context.Background(), 5*time.Second)
	pool, err := database.OpenPool(startupCtx, &cfg.Database)
	startupCancel()
	if err != nil {
		logger.Error("connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	server := &http.Server{
		Addr:              ":" + cfg.Service.Port,
		Handler:           apphttp.RouterHandler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Setup signal context
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Run server in background go routine
	serverErr := make(chan error, 1)
	go func() {
		logger.Info("HTTP server starting", "addr", server.Addr, "service", cfg.Service.Name, "environment", cfg.Service.Environment)
		serverErr <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	case err := <-serverErr:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("HTTP server failed", "error", err)
			os.Exit(1)
		}
	}

	// Graceful shutdown with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
		os.Exit(1)
	}
	logger.Info("HTTP server stopped")
}
