package config

import (
	"fmt"
	"strconv"
)

// ServiceConfig represents service configuration
type ServiceConfig struct {
	Name        string
	Environment string
	Port        string
}

type Config struct {
	Service ServiceConfig
}

// LoadConfig loads configuration from environment variables and validates it.
func LoadConfig() (*Config, error) {
	cfg := &Config{
		Service: ServiceConfig{
			Name:        getEnv("SERVICE_NAME", DefaultServiceName),
			Environment: getEnv("SERVICE_ENVIRONMENT", DefaultServiceEnvironment),
			Port:        getEnv("SERVICE_PORT", DefaultServicePort),
		},
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Validate checks that the loaded settings are usable by the service.
func (c *Config) Validate() error {
	if c == nil {
		return fmt.Errorf("configuration is nil")
	}
	if c.Service.Name == "" {
		return fmt.Errorf("SERVICE_NAME must not be empty")
	}
	if c.Service.Environment == "" {
		return fmt.Errorf("SERVICE_ENVIRONMENT must not be empty")
	}
	port, err := strconv.Atoi(c.Service.Port)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("SERVICE_PORT must be a number between 1 and 65535; got %q", c.Service.Port)
	}
	return nil
}
