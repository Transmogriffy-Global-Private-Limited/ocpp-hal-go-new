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

	OCPPListenPort int
	OCPPListenPath string

	APIKey     string
	APIAuthKey string
	LogLevel   slog.Level

	DatabaseURL string
	DBName      string
	DBUser      string
	DBPassword  string
	DBHost      string
	DBPort      int
	DBSSLMode   string

	MainCMSStartTxnHookURL       string
	SingleSessionStartTxnHookURL string

	MainCMSCompletedTxnURL       string
	SingleSessionCompletedTxnURL string

	ChargerDataURL             string
	ChargerDataCacheTTLSeconds int

	V1Enabled        bool
	V1CMSBearerToken string
	APIDocsEnabled   bool
}

func Load() Config {
	loadLocalEnv()

	return Config{
		Environment: env("HAL_ENVIRONMENT", "development"),

		RESTHost: env("F_SERVER_HOST", "127.0.0.1"),
		RESTPort: env("F_SERVER_PORT", "18080"),

		OCPPListenPort: envInt("OCPP_LISTEN_PORT", 18081),
		OCPPListenPath: env("OCPP_LISTEN_PATH", "/{ws}"),

		APIKey:     os.Getenv("API_KEY"),
		APIAuthKey: os.Getenv("APIAUTHKEY"),
		LogLevel:   parseLogLevel(env("LOG_LEVEL", "info")),

		DatabaseURL: os.Getenv("DATABASE_URL"),
		DBName:      os.Getenv("DB_NAME"),
		DBUser:      os.Getenv("DB_USER"),
		DBPassword:  os.Getenv("DB_PASSWORD"),
		DBHost:      env("DB_HOST", "127.0.0.1"),
		DBPort:      envInt("DB_PORT", 5432),
		DBSSLMode:   env("DB_SSLMODE", "disable"),

		MainCMSStartTxnHookURL:       env("MAIN_CMS_START_TXN_HOOK_URL", "https://be.cms.ocpp.transev.site/users/checkstartresponse"),
		SingleSessionStartTxnHookURL: os.Getenv("SINGLE_SESSION_START_TXN_HOOK_URL"),

		MainCMSCompletedTxnURL:       env("MAIN_CMS_COMPLETED_TXN_URL", "https://be.cms.ocpp.transev.site/users/deductcalculate"),
		SingleSessionCompletedTxnURL: os.Getenv("SINGLE_SESSION_COMPLETED_TXN_URL"),

		ChargerDataURL:             os.Getenv("APICHARGERDATA"),
		ChargerDataCacheTTLSeconds: envInt("CHARGER_DATA_CACHE_TTL_SECONDS", 7200),

		V1Enabled:        envBool("HAL_V1_ENABLED", false),
		V1CMSBearerToken: os.Getenv("HAL_V1_CMS_BEARER_TOKEN"),
		APIDocsEnabled:   envBool("API_DOCS_ENABLED", false),
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

func (c Config) RequiresDatabase() bool {
	return strings.EqualFold(strings.TrimSpace(c.Environment), "production")
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
