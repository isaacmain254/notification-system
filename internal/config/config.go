package config

import (
	"fmt"
	"strconv"
)

type ServiceConfig struct {
	Name        string
	Environment string
	Port        string
}

type DatabaseConfig struct {
	Host            string
	Port            int
	Name            string
	User            string
	Password        string
	SSLMode         string
	MaxConns        int32
	MinConns        int32
	MaxConnIdleTime int32 // minutes
	MaxConnLifeTime int32 // minutes
}

type Config struct {
	Service  ServiceConfig
	Database DatabaseConfig
}

func LoadConfig() (*Config, error) {
	dbPort, err := getEnvInt("DB_PORT", DefaultDBPort)
	if err != nil {
		return nil, err
	}
	maxConns, err := getEnvInt32("DB_MAX_CONNS", DefaultDBMaxConns)
	if err != nil {
		return nil, err
	}
	minConns, err := getEnvInt32("DB_MIN_CONNS", DefaultDBMinConns)
	if err != nil {
		return nil, err
	}
	idleMinutes, err := getEnvInt32("DB_MAX_CONN_IDLE_TIME", DefaultDBMaxConnIdleTime)
	if err != nil {
		return nil, err
	}
	lifetimeMinutes, err := getEnvInt32("DB_MAX_CONN_LIFETIME", DefaultDBMaxConnLifeTime)
	if err != nil {
		return nil, err
	}
	cfg := &Config{
		Service: ServiceConfig{
			Name:        getEnv("SERVICE_NAME", DefaultServiceName),
			Environment: getEnv("SERVICE_ENVIRONMENT", DefaultServiceEnvironment),
			Port:        getEnv("SERVICE_PORT", DefaultServicePort),
		},
		Database: DatabaseConfig{
			Host:            getEnv("DB_HOST", DefaultDBHost),
			Port:            dbPort,
			Name:            getEnv("DB_NAME", DefaultDBName),
			User:            getEnv("DB_USER", DefaultDBUser),
			Password:        getEnv("DB_PASSWORD", DefaultDBPassword),
			SSLMode:         getEnv("DB_SSL_MODE", DefaultDBSSLMode),
			MaxConns:        maxConns,
			MinConns:        minConns,
			MaxConnIdleTime: idleMinutes,
			MaxConnLifeTime: lifetimeMinutes,
		},
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) Validate() error {
	if c == nil {
		return fmt.Errorf("configuration is nil")
	}
	if c.Service.Name == "" || c.Service.Environment == "" {
		return fmt.Errorf("SERVICE_NAME and SERVICE_ENVIRONMENT must not be empty")
	}
	servicePort, err := strconv.Atoi(c.Service.Port)
	if err != nil || servicePort < 1 || servicePort > 65535 {
		return fmt.Errorf("SERVICE_PORT must be a number between 1 and 65535; got %q", c.Service.Port)
	}
	db := c.Database
	if db.Host == "" || db.Name == "" || db.User == "" || db.Password == "" {
		return fmt.Errorf("DB_HOST, DB_NAME, DB_USER, and DB_PASSWORD must not be empty")
	}
	if db.Port < 1 || db.Port > 65535 {
		return fmt.Errorf("DB_PORT must be between 1 and 65535; got %d", db.Port)
	}
	if db.SSLMode == "" {
		return fmt.Errorf("DB_SSL_MODE must not be empty")
	}
	if db.MaxConns < 1 || db.MinConns < 0 || db.MinConns > db.MaxConns {
		return fmt.Errorf("database pool limits must satisfy 0 <= DB_MIN_CONNS <= DB_MAX_CONNS")
	}
	if db.MaxConnIdleTime < 1 || db.MaxConnLifeTime < 1 {
		return fmt.Errorf("DB_MAX_CONN_IDLE_TIME and DB_MAX_CONN_LIFETIME must be positive minutes")
	}
	return nil
}
