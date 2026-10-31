package config

import (
	"fmt"
	"os"
	"strconv"
)

// getEnv retrieves an environment variable or returns a fallback value
func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}

// getEnvInt64 retrieves an environment variable as an int64 or returns a fallback value
func getEnvInt32(key string, fallback int32) (int32, error) {
	if value, exists := os.LookupEnv(key); exists {
		intValue, err := strconv.ParseInt(value, 10, 32)
		if err != nil {
			return 0, fmt.Errorf("%s must be an integer: %w", key, err)
		}
		return int32(intValue), nil
	}
	return fallback, nil
}

func getEnvInt(key string, fallback int) (int, error) {
	if value, exists := os.LookupEnv(key); exists {
		intValue, err := strconv.Atoi(value)
		if err != nil {
			return 0, fmt.Errorf("%s must be an integer: %w", key, err)
		}
		return intValue, nil
	}
	return fallback, nil
}
