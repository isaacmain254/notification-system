package config

const (
	// Service
	DefaultServiceName        = "notification-system"
	DefaultServiceEnvironment = "development"
	DefaultServicePort        = "5000"
	// Database
	DefaultDBHost            = "localhost"
	DefaultDBPort            = 5432
	DefaultDBName            = "notification-system"
	DefaultDBUser            = "postgres"
	DefaultDBPassword        = "password"
	DefaultDBSSLMode         = "disable"
	DefaultDBMaxConns        = 25
	DefaultDBMinConns        = 2
	DefaultDBMaxConnIdleTime = 5
	DefaultDBMaxConnLifeTime = 30
	// Rabbitmq
	DefaultRabbitmqHost     = "localhost"
	DefaultRabbitmqPort     = 5672
	DefaultRabbitmqUser     = "notification"
	DefaultRabbitmqPassword = "notification-dev-password"
	DefaultRabbitmqPrefetch = 10
	DefaultOutboxBatchSize  = 50
)
