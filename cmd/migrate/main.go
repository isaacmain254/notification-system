package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"

	"github.com/isaacmain254/notification-system/internal/config"
	"github.com/isaacmain254/notification-system/internal/database"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.LoadConfig()
	if err != nil {
		logger.Error("load configuration", "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	db, err := database.OpenMigrationDB(ctx, &cfg.Database)
	if err != nil {
		logger.Error("connect to database", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	conn, err := db.Conn(ctx)
	if err != nil {
		logger.Error("reserve migration connection", "error", err)
		os.Exit(1)
	}
	defer conn.Close()

	driver, err := postgres.WithConnection(ctx, conn, &postgres.Config{})
	if err != nil {
		logger.Error("initialize migration driver", "error", err)
		os.Exit(1)
	}
	m, err := migrate.NewWithDatabaseInstance("file://db/migrations", "postgres", driver)
	if err != nil {
		logger.Error("initialize migrations", "error", err)
		os.Exit(1)
	}
	defer func() {
		if _, closeErr := m.Close(); closeErr != nil {
			logger.Error("close migration resources", "error", closeErr)
		}
	}()

	var migrationErr error
	if len(os.Args) > 1 && os.Args[1] == "down" {
		migrationErr = m.Down()
	} else {
		migrationErr = m.Up()
	}
	if migrationErr != nil && !errors.Is(migrationErr, migrate.ErrNoChange) {
		logger.Error("run migrations", "error", migrationErr)
		os.Exit(1)
	}
	logger.Info("database migrations are up to date")
}
