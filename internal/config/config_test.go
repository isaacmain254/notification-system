package config

import "testing"

func TestLoadConfigReadsEnvironment(t *testing.T) {
	t.Setenv("SERVICE_NAME", "test-service")
	t.Setenv("SERVICE_ENVIRONMENT", "test")
	t.Setenv("SERVICE_PORT", "8081")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if cfg.Service.Name != "test-service" || cfg.Service.Environment != "test" || cfg.Service.Port != "8081" {
		t.Fatalf("LoadConfig() = %+v, want environment values", cfg.Service)
	}
}

func TestLoadConfigRejectsInvalidPort(t *testing.T) {
	t.Setenv("SERVICE_NAME", "test-service")
	t.Setenv("SERVICE_ENVIRONMENT", "test")
	t.Setenv("SERVICE_PORT", "70000")

	if _, err := LoadConfig(); err == nil {
		t.Fatal("LoadConfig() error = nil, want invalid port error")
	}
}
