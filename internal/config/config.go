package config

import (
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	Environment string

	RESTHost string
	RESTPort string

	OCPPListenPort               int
	OCPPListenPath               string
	OCPPHeartbeatIntervalSeconds int

	LogLevel slog.Level

	DatabaseURL string
	DBName      string
	DBUser      string
	DBPassword  string
	DBHost      string
	DBPort      int
	DBSSLMode   string

	V1CMSBearerToken      string
	V1FactDeliveryEnabled bool
	V1CMSFactsURL         string
	V1CMSFactsBearerToken string
	APIDocsEnabled        bool
}

func Load() Config {
	loadLocalEnv()

	return Config{
		Environment: env("HAL_ENVIRONMENT", "development"),

		RESTHost: env("F_SERVER_HOST", "127.0.0.1"),
		RESTPort: env("F_SERVER_PORT", "18080"),

		OCPPListenPort:               envInt("OCPP_LISTEN_PORT", 18081),
		OCPPListenPath:               env("OCPP_LISTEN_PATH", "/{ws}"),
		OCPPHeartbeatIntervalSeconds: envInt("OCPP_HEARTBEAT_INTERVAL_SECONDS", 300),

		LogLevel: parseLogLevel(env("LOG_LEVEL", "info")),

		DatabaseURL: os.Getenv("DATABASE_URL"),
		DBName:      os.Getenv("DB_NAME"),
		DBUser:      os.Getenv("DB_USER"),
		DBPassword:  os.Getenv("DB_PASSWORD"),
		DBHost:      env("DB_HOST", "127.0.0.1"),
		DBPort:      envInt("DB_PORT", 5432),
		DBSSLMode:   env("DB_SSLMODE", "disable"),

		V1CMSBearerToken:      os.Getenv("HAL_V1_CMS_BEARER_TOKEN"),
		V1FactDeliveryEnabled: envBool("HAL_V1_FACT_DELIVERY_ENABLED", false),
		V1CMSFactsURL:         os.Getenv("HAL_V1_CMS_FACTS_URL"),
		V1CMSFactsBearerToken: os.Getenv("HAL_V1_CMS_FACT_BEARER_TOKEN"),
		APIDocsEnabled:        envBool("API_DOCS_ENABLED", false),
	}
}

// loadLocalEnv is intentionally small: it provides local development defaults
// without changing already-supplied process configuration. Production deploys
// provide their environment directly and do not depend on this file.
func loadLocalEnv() {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("HAL_ENVIRONMENT")), "production") {
		return
	}
	path := filepath.Join(".", ".env")
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), "\"'")
		if key == "" || os.Getenv(key) != "" {
			continue
		}
		_ = os.Setenv(key, value)
	}
}

func (c Config) RESTListenAddr() string {
	return net.JoinHostPort(c.RESTHost, c.RESTPort)
}

func (c Config) HasDatabase() bool {
	if strings.TrimSpace(c.DatabaseURL) != "" {
		return true
	}

	return strings.TrimSpace(c.DBName) != "" &&
		strings.TrimSpace(c.DBUser) != "" &&
		strings.TrimSpace(c.DBHost) != "" &&
		c.DBPort > 0
}

func env(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func envInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}

func envBool(key string, fallback bool) bool {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return fallback
	}
	return value
}

func parseLogLevel(value string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
