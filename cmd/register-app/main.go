package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/isaacmain254/notification-system/internal/applications"
	"github.com/isaacmain254/notification-system/internal/config"
	"github.com/isaacmain254/notification-system/internal/database"
)

func main() {
	name := flag.String("name", "", "display name for the integrating application")
	flag.Parse()
	if strings.TrimSpace(*name) == "" {
		fmt.Fprintln(os.Stderr, "-name is required")
		os.Exit(2)
	}
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	cfg, err := config.LoadConfig()
	if err != nil {
		logger.Error("load configuration", "error", err)
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := database.OpenPool(ctx, &cfg.Database)
	if err != nil {
		logger.Error("connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	key, err := applications.GenerateAPIKey()
	if err != nil {
		logger.Error("generate API key", "error", err)
		os.Exit(1)
	}
	app, err := database.NewRepository(pool).CreateApplication(ctx, strings.TrimSpace(*name), applications.HashAPIKey(key))
	if err != nil {
		logger.Error("register application", "error", err)
		os.Exit(1)
	}
	fmt.Printf("Application ID: %s\nAPI key (shown once): %s\n", app.ID.String(), key)
}
