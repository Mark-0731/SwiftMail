package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

// Config holds all application configuration loaded from environment variables.
type Config struct {
	Server        ServerConfig
	Database      DatabaseConfig
	Redis         RedisConfig
	ClickHouse    ClickHouseConfig
	MinIO         MinIOConfig
	JWT           JWTConfig
	SMTP          SMTPConfig
	Worker        WorkerConfig
	RateLimit     RateLimitConfig
	Observability ObservabilityConfig
	Stripe        StripeConfig
	App           AppConfig
}

type ServerConfig struct {
	Host         string
	Port         string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

type DatabaseConfig struct {
	Host            string
	Port            string
	User            string
	Password        string
	Name            string
	SSLMode         string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

func (d DatabaseConfig) DSN() string {
	return "postgres://" + d.User + ":" + d.Password + "@" + d.Host + ":" + d.Port + "/" + d.Name + "?sslmode=" + d.SSLMode
}

type RedisConfig struct {
	Host     string
	Port     string
	Password string
	DB       int
}

func (r RedisConfig) Addr() string {
	return r.Host + ":" + r.Port
}

type ClickHouseConfig struct {
	Host     string
	Port     string
	Database string
	User     string
	Password string
}

type MinIOConfig struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
	UseSSL    bool
}

type JWTConfig struct {
	AccessSecret  string
	RefreshSecret string
	AccessExpiry  time.Duration
	RefreshExpiry time.Duration
}

type SMTPConfig struct {
	Host           string
	Port           string
	Username       string
	Password       string
	PoolSize       int
	MinPoolSize    int
	MaxPoolSize    int
	ConnectTimeout time.Duration
	SendTimeout    time.Duration
	MaxIdleTime    time.Duration
	MaxConnAge     time.Duration
	MaxConnUses    int
	EnableRetries  bool // Enable application-level retries (disable if using Postfix/relay that handles retries)
	MaxRetries     int  // Maximum retry attempts for temporary failures
}

type WorkerConfig struct {
	Concurrency     int
	QueueCritical   int
	QueueHigh       int
	QueueDefault    int
	QueueLow        int
	StrictPriority  bool
	ShutdownTimeout time.Duration
}

type RateLimitConfig struct {
	PerSec int
	PerDay int
}

type ObservabilityConfig struct {
	OTELEndpoint   string
	PrometheusPort string
}

type StripeConfig struct {
	SecretKey      string
	WebhookSecret  string
	PublishableKey string
	PriceIDStarter string
	PriceIDPro     string
	SuccessURL     string
	CancelURL      string
}

type AppConfig struct {
	Env string // development, staging, production
}

// Load reads all configuration from environment variables with sensible defaults.
func Load() *Config {
	// Try to load .env file (ignore error if it doesn't exist)
	_ = godotenv.Load()

	cfg := loadConfig()

	// Validate production secrets
	if cfg.App.Env == "production" {
		validateProductionSecrets(cfg)
	}

	return cfg
}

func loadConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Host:         getEnv("SERVER_HOST", "0.0.0.0"),
			Port:         getEnv("SERVER_PORT", "8080"),
			ReadTimeout:  getDurationEnv("SERVER_READ_TIMEOUT", 10*time.Second),
			WriteTimeout: getDurationEnv("SERVER_WRITE_TIMEOUT", 10*time.Second),
		},
		Database: DatabaseConfig{
			Host:            getEnv("DB_HOST", "localhost"),
			Port:            getEnv("DB_PORT", "5432"),
			User:            getEnv("DB_USER", "swiftmail"),
			Password:        getEnv("DB_PASSWORD", "swiftmail_dev"),
			Name:            getEnv("DB_NAME", "swiftmail"),
			SSLMode:         getEnv("DB_SSL_MODE", "disable"),
			MaxOpenConns:    getIntEnv("DB_MAX_OPEN_CONNS", 25),
			MaxIdleConns:    getIntEnv("DB_MAX_IDLE_CONNS", 10),
			ConnMaxLifetime: getDurationEnv("DB_CONN_MAX_LIFETIME", 5*time.Minute),
		},
		Redis: RedisConfig{
			Host:     getEnv("REDIS_HOST", "localhost"),
			Port:     getEnv("REDIS_PORT", "6379"),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getIntEnv("REDIS_DB", 0),
		},
		ClickHouse: ClickHouseConfig{
			Host:     getEnv("CLICKHOUSE_HOST", "localhost"),
			Port:     getEnv("CLICKHOUSE_PORT", "9000"),
			Database: getEnv("CLICKHOUSE_DATABASE", "swiftmail"),
			User:     getEnv("CLICKHOUSE_USER", "default"),
			Password: getEnv("CLICKHOUSE_PASSWORD", ""),
		},
		MinIO: MinIOConfig{
			Endpoint:  getEnv("MINIO_ENDPOINT", "localhost:9002"),
			AccessKey: getEnv("MINIO_ACCESS_KEY", "minioadmin"),
			SecretKey: getEnv("MINIO_SECRET_KEY", "minioadmin"),
			Bucket:    getEnv("MINIO_BUCKET", "swiftmail-attachments"),
			UseSSL:    getBoolEnv("MINIO_USE_SSL", false),
		},
		JWT: JWTConfig{
			AccessSecret:  getEnv("JWT_ACCESS_SECRET", "change-me-in-production-access-secret-key"),
			RefreshSecret: getEnv("JWT_REFRESH_SECRET", "change-me-in-production-refresh-secret-key"),
			AccessExpiry:  getDurationEnv("JWT_ACCESS_EXPIRY", 15*time.Minute),
			RefreshExpiry: getDurationEnv("JWT_REFRESH_EXPIRY", 168*time.Hour),
		},
		SMTP: SMTPConfig{
			Host:           getEnv("SMTP_HOST", "localhost"),
			Port:           getEnv("SMTP_PORT", "587"),
			Username:       getEnv("SMTP_USERNAME", ""),
			Password:       getEnv("SMTP_PASSWORD", ""),
			PoolSize:       getIntEnv("SMTP_POOL_SIZE", 50),
			MinPoolSize:    getIntEnv("SMTP_MIN_POOL_SIZE", 10),
			MaxPoolSize:    getIntEnv("SMTP_MAX_POOL_SIZE", 100),
			ConnectTimeout: getDurationEnv("SMTP_CONNECT_TIMEOUT", 5*time.Second),
			SendTimeout:    getDurationEnv("SMTP_SEND_TIMEOUT", 15*time.Second),
			MaxIdleTime:    getDurationEnv("SMTP_MAX_IDLE_TIME", 2*time.Minute),
			MaxConnAge:     getDurationEnv("SMTP_MAX_CONN_AGE", 10*time.Minute),
			MaxConnUses:    getIntEnv("SMTP_MAX_CONN_USES", 500),
			EnableRetries:  getBoolEnv("SMTP_ENABLE_RETRIES", false), // Default OFF for Postfix/relay
			MaxRetries:     getIntEnv("SMTP_MAX_RETRIES", 3),
		},
		Worker: WorkerConfig{
			Concurrency:     getIntEnv("WORKER_CONCURRENCY", 50),
			QueueCritical:   getIntEnv("WORKER_QUEUE_CRITICAL", 8),
			QueueHigh:       getIntEnv("WORKER_QUEUE_HIGH", 5),
			QueueDefault:    getIntEnv("WORKER_QUEUE_DEFAULT", 2),
			QueueLow:        getIntEnv("WORKER_QUEUE_LOW", 1),
			StrictPriority:  getBoolEnv("WORKER_STRICT_PRIORITY", true),
			ShutdownTimeout: getDurationEnv("WORKER_SHUTDOWN_TIMEOUT", 30*time.Second),
		},
		RateLimit: RateLimitConfig{
			PerSec: getIntEnv("RATE_LIMIT_PER_SEC", 100),
			PerDay: getIntEnv("RATE_LIMIT_PER_DAY", 50000),
		},
		Observability: ObservabilityConfig{
			OTELEndpoint:   getEnv("OTEL_ENDPOINT", "localhost:4318"),
			PrometheusPort: getEnv("PROMETHEUS_PORT", "9091"),
		},
		Stripe: StripeConfig{
			SecretKey:      getEnv("STRIPE_SECRET_KEY", ""),
			WebhookSecret:  getEnv("STRIPE_WEBHOOK_SECRET", ""),
			PublishableKey: getEnv("STRIPE_PUBLISHABLE_KEY", ""),
			PriceIDStarter: getEnv("STRIPE_PRICE_ID_STARTER", ""),
			PriceIDPro:     getEnv("STRIPE_PRICE_ID_PRO", ""),
			SuccessURL:     getEnv("STRIPE_SUCCESS_URL", "http://localhost:3000/billing/success"),
			CancelURL:      getEnv("STRIPE_CANCEL_URL", "http://localhost:3000/billing/cancel"),
		},
		App: AppConfig{
			Env: getEnv("APP_ENV", "development"),
		},
	}
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func getIntEnv(key string, fallback int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return fallback
}

func getBoolEnv(key string, fallback bool) bool {
	if val := os.Getenv(key); val != "" {
		if b, err := strconv.ParseBool(val); err == nil {
			return b
		}
	}
	return fallback
}

func getDurationEnv(key string, fallback time.Duration) time.Duration {
	if val := os.Getenv(key); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			return d
		}
	}
	return fallback
}

// validateProductionSecrets ensures critical secrets are not using default values in production
func validateProductionSecrets(cfg *Config) {
	errors := []string{}

	// JWT secrets must not be defaults
	if cfg.JWT.AccessSecret == "change-me-in-production-access-secret-key" {
		errors = append(errors, "JWT_ACCESS_SECRET is using default value")
	}
	if cfg.JWT.RefreshSecret == "change-me-in-production-refresh-secret-key" {
		errors = append(errors, "JWT_REFRESH_SECRET is using default value")
	}

	// Stripe keys must be set
	if cfg.Stripe.SecretKey == "" {
		errors = append(errors, "STRIPE_SECRET_KEY is not set")
	}
	if cfg.Stripe.WebhookSecret == "" {
		errors = append(errors, "STRIPE_WEBHOOK_SECRET is not set")
	}

	// Database password should be set
	if cfg.Database.Password == "swiftmail_dev" {
		errors = append(errors, "DB_PASSWORD is using default development value")
	}

	// MinIO credentials should not be defaults
	if cfg.MinIO.AccessKey == "minioadmin" || cfg.MinIO.SecretKey == "minioadmin" {
		errors = append(errors, "MINIO credentials are using default values")
	}

	if len(errors) > 0 {
		panic(fmt.Sprintf("Production configuration validation failed:\n- %s",
			joinStrings(errors, "\n- ")))
	}
}

func joinStrings(strs []string, sep string) string {
	result := ""
	for i, s := range strs {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}
