package database

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/isaacmain254/notification-system/internal/config"
)

func connectionString(cfg *config.DatabaseConfig) string {
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(cfg.User, cfg.Password),
		Host:   net.JoinHostPort(cfg.Host, fmt.Sprintf("%d", cfg.Port)),
		Path:   "/" + cfg.Name,
	}
	query := u.Query()
	query.Set("sslmode", cfg.SSLMode)
	u.RawQuery = query.Encode()
	return u.String()
}

// OpenPool creates and verifies the application's pgx connection pool.
func OpenPool(ctx context.Context, cfg *config.DatabaseConfig) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(connectionString(cfg))
	if err != nil {
		return nil, fmt.Errorf("parse database configuration: %w", err)
	}
	poolCfg.MaxConns = cfg.MaxConns
	poolCfg.MinConns = cfg.MinConns
	poolCfg.MaxConnLifetime = time.Duration(cfg.MaxConnLifeTime) * time.Minute
	poolCfg.MaxConnIdleTime = time.Duration(cfg.MaxConnIdleTime) * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("create database pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return pool, nil
}

// OpenMigrationDB provides a database/sql handle for golang-migrate's Postgres driver.
func OpenMigrationDB(ctx context.Context, cfg *config.DatabaseConfig) (*sql.DB, error) {
	connCfg, err := pgx.ParseConfig(connectionString(cfg))
	if err != nil {
		return nil, fmt.Errorf("parse migration database configuration: %w", err)
	}
	db := stdlib.OpenDB(*connCfg)
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(time.Duration(cfg.MaxConnLifeTime) * time.Minute)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping migration database: %w", err)
	}
	return db, nil
}
