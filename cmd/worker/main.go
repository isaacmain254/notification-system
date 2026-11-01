package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/isaacmain254/notification-system/internal/config"
	"github.com/isaacmain254/notification-system/internal/database"
	"github.com/isaacmain254/notification-system/internal/messaging"
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
	repository := database.NewRepository(pool)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	workerCtx, cancelWorkers := context.WithCancel(ctx)
	defer cancelWorkers()

	results := make(chan error, 2)
	var workers sync.WaitGroup
	workers.Add(2)
	go func() {
		defer workers.Done()
		results <- messaging.RunOutboxPublisher(workerCtx, cfg.Rabbitmq, repository, int32(cfg.Rabbitmq.BatchSize), logger)
	}()
	go func() {
		defer workers.Done()
		results <- messaging.RunEventConsumer(workerCtx, cfg.Rabbitmq, repository, cfg.Rabbitmq.Prefetch, logger)
	}()

	var workerErr error
	select {
	case <-ctx.Done():
		logger.Info("worker shutdown signal received")
	case workerErr = <-results:
		if workerErr == nil && ctx.Err() == nil {
			workerErr = errors.New("messaging worker stopped unexpectedly")
		}
		cancelWorkers()
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()
	done := make(chan struct{})
	go func() {
		workers.Wait()
		close(done)
	}()
	select {
	case <-done:
		logger.Info("messaging workers stopped")
	case <-shutdownCtx.Done():
		logger.Error("messaging workers did not stop before timeout")
		os.Exit(1)
	}
	if workerErr != nil {
		logger.Error("messaging worker stopped", "error", workerErr)
		os.Exit(1)
	}
}
