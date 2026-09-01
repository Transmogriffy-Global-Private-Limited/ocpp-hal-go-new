package config

import (
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	defaultEnvironment    = "development"
	defaultRESTHost       = "127.0.0.1"
	defaultRESTPort       = "18080"
	defaultOCPPListenPort = 18081
	// ocpp-go passes only the final path segment to protocol handlers. The
	// catch-all lets HAL validate both supported physical URL forms itself.
	defaultOCPPListenPath                       = "/{ws:.*}"
	defaultHeartbeatIntervalSeconds             = 300
	defaultMeterSampleIntervalSeconds           = 15
	defaultConfigurationReconcileTimeoutSeconds = 20
	defaultDBHost                               = "127.0.0.1"
	defaultDBPort                               = 5432
	defaultDBSSLMode                            = "disable"
)

type Config struct {
	Environment                              string
	RESTHost                                 string
	RESTPort                                 string
	OCPPListenPort                           int
	OCPPListenPath                           string
	OCPPHeartbeatIntervalSeconds             int
	OCPPMeterSampleIntervalSeconds           int
	OCPPConfigurationReconcileTimeoutSeconds int
	OCPPVendorConfigurationProfile           string
	OCPPVendorConfigurationVendor            string
	LogLevel                                 slog.Level
	DatabaseURL                              string
	DBName                                   string
	DBUser                                   string
	DBPassword                               string
	DBHost                                   string
	DBPort                                   int
	DBSSLMode                                string
	V1CMSBearerToken                         string
	V1FactDeliveryEnabled                    bool
	V1CMSFactsURL                            string
	V1CMSFactsBearerToken                    string
	V1TraceRetentionDays                     int
	V1TraceRetentionIntervalSeconds          int
	MigrationApplicationRole                 string
	APIDocsEnabled                           bool
}

// Load returns only a fully validated runtime configuration. Defaults apply
// when a setting is absent; an explicitly supplied invalid setting prevents
// startup rather than changing the operator's intent.
func Load() (Config, error) {
	values, err := loadValues()
	if err != nil {
		return Config{}, err
	}
	return parse(values)
}

// loadValues gives process configuration precedence over local development
// defaults without mutating process state. Explicit production never consumes
// a repository-local .env file.
func loadValues() (map[string]string, error) {
	values := make(map[string]string)
	for _, pair := range os.Environ() {
		key, raw, ok := strings.Cut(pair, "=")
		if ok {
			values[key] = raw
		}
	}
	if raw, present := values["HAL_ENVIRONMENT"]; present {
		environment, err := parseEnvironment(raw)
		if err != nil {
			return nil, err
		}
		if environment == "production" {
			return values, nil
		}
	}
	local, err := readLocalEnv(filepath.Join(".", ".env"))
	if err != nil {
		return nil, err
	}
	for key, raw := range local {
		// Process environment alone chooses the bootstrap environment. A local
		// file may provide development defaults but may not silently turn an
		// unclassified process into production.
		if key == "HAL_ENVIRONMENT" {
			continue
		}
		if _, supplied := values[key]; !supplied {
			values[key] = raw
		}
	}
	return values, nil
}

func readLocalEnv(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read local .env: %w", err)
	}
	values := make(map[string]string)
	for lineNumber, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, raw, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			return nil, fmt.Errorf("invalid local .env line %d", lineNumber+1)
		}
		values[key] = strings.Trim(strings.TrimSpace(raw), "\"'")
	}
	return values, nil
}

func parse(values map[string]string) (Config, error) {
	environment, err := parseEnvironment(value(values, "HAL_ENVIRONMENT", defaultEnvironment))
	if err != nil {
		return Config{}, err
	}
	restPort, err := parsePort("F_SERVER_PORT", value(values, "F_SERVER_PORT", defaultRESTPort))
	if err != nil {
		return Config{}, err
	}
	ocppPortString, err := parsePort("OCPP_LISTEN_PORT", value(values, "OCPP_LISTEN_PORT", strconv.Itoa(defaultOCPPListenPort)))
	if err != nil {
		return Config{}, err
	}
	ocppPort, _ := strconv.Atoi(ocppPortString)
	heartbeat, err := parseBoundedInt("OCPP_HEARTBEAT_INTERVAL_SECONDS", value(values, "OCPP_HEARTBEAT_INTERVAL_SECONDS", strconv.Itoa(defaultHeartbeatIntervalSeconds)), 1, 86400)
	if err != nil {
		return Config{}, err
	}
	meterSampleInterval, err := parseBoundedInt("OCPP_METER_VALUE_SAMPLE_INTERVAL_SECONDS", value(values, "OCPP_METER_VALUE_SAMPLE_INTERVAL_SECONDS", strconv.Itoa(defaultMeterSampleIntervalSeconds)), 1, 3600)
	if err != nil {
		return Config{}, err
	}
	reconcileTimeout, err := parseBoundedInt("OCPP_CONFIGURATION_RECONCILE_TIMEOUT_SECONDS", value(values, "OCPP_CONFIGURATION_RECONCILE_TIMEOUT_SECONDS", strconv.Itoa(defaultConfigurationReconcileTimeoutSeconds)), 1, 120)
	if err != nil {
		return Config{}, err
	}
	traceRetentionDays, err := parseBoundedInt("HAL_V1_TRACE_RETENTION_DAYS", value(values, "HAL_V1_TRACE_RETENTION_DAYS", "30"), 1, 3650)
	if err != nil {
		return Config{}, err
	}
	traceRetentionInterval, err := parseBoundedInt("HAL_V1_TRACE_RETENTION_INTERVAL_SECONDS", value(values, "HAL_V1_TRACE_RETENTION_INTERVAL_SECONDS", "3600"), 60, 86400)
	if err != nil {
		return Config{}, err
	}
	vendorProfile := strings.TrimSpace(value(values, "OCPP_VENDOR_CONFIGURATION_PROFILE", ""))
	if vendorProfile != "" && vendorProfile != "legacy-remote-only" {
		return Config{}, fmt.Errorf("invalid OCPP_VENDOR_CONFIGURATION_PROFILE: %q", vendorProfile)
	}
	vendorProfileVendor := strings.TrimSpace(value(values, "OCPP_VENDOR_CONFIGURATION_VENDOR", ""))
	if vendorProfile != "" && vendorProfileVendor == "" {
		return Config{}, fmt.Errorf("OCPP_VENDOR_CONFIGURATION_VENDOR is required when OCPP_VENDOR_CONFIGURATION_PROFILE is set")
	}
	dbPortString, err := parsePort("DB_PORT", value(values, "DB_PORT", strconv.Itoa(defaultDBPort)))
	if err != nil {
		return Config{}, err
	}
	dbPort, _ := strconv.Atoi(dbPortString)
	factDelivery, err := parseBool("HAL_V1_FACT_DELIVERY_ENABLED", value(values, "HAL_V1_FACT_DELIVERY_ENABLED", "false"))
	if err != nil {
		return Config{}, err
	}
	docsEnabled, err := parseBool("API_DOCS_ENABLED", value(values, "API_DOCS_ENABLED", "false"))
	if err != nil {
		return Config{}, err
	}
	level, err := parseLogLevel(value(values, "LOG_LEVEL", "info"))
	if err != nil {
		return Config{}, err
	}
	cfg := Config{
		Environment: environment, RESTHost: value(values, "F_SERVER_HOST", defaultRESTHost), RESTPort: restPort,
		OCPPListenPort: ocppPort, OCPPListenPath: value(values, "OCPP_LISTEN_PATH", defaultOCPPListenPath), OCPPHeartbeatIntervalSeconds: heartbeat, OCPPMeterSampleIntervalSeconds: meterSampleInterval, OCPPConfigurationReconcileTimeoutSeconds: reconcileTimeout, OCPPVendorConfigurationProfile: vendorProfile, OCPPVendorConfigurationVendor: vendorProfileVendor,
		LogLevel:    level,
		DatabaseURL: value(values, "DATABASE_URL", ""), DBName: value(values, "DB_NAME", ""), DBUser: value(values, "DB_USER", ""), DBPassword: value(values, "DB_PASSWORD", ""), DBHost: value(values, "DB_HOST", defaultDBHost), DBPort: dbPort, DBSSLMode: value(values, "DB_SSLMODE", defaultDBSSLMode),
		V1CMSBearerToken: value(values, "HAL_V1_CMS_BEARER_TOKEN", ""), V1FactDeliveryEnabled: factDelivery, V1CMSFactsURL: value(values, "HAL_V1_CMS_FACTS_URL", ""), V1CMSFactsBearerToken: value(values, "HAL_V1_CMS_FACT_BEARER_TOKEN", ""), V1TraceRetentionDays: traceRetentionDays, V1TraceRetentionIntervalSeconds: traceRetentionInterval, MigrationApplicationRole: strings.TrimSpace(value(values, "HAL_MIGRATION_APPLICATION_ROLE", "")), APIDocsEnabled: docsEnabled,
	}
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) validate() error {
	if strings.TrimSpace(c.RESTHost) == "" {
		return fmt.Errorf("invalid F_SERVER_HOST: must not be empty")
	}
	if !strings.HasPrefix(c.OCPPListenPath, "/") {
		return fmt.Errorf("invalid OCPP_LISTEN_PATH: must start with /")
	}
	if strings.TrimSpace(c.V1CMSBearerToken) == "" {
		return fmt.Errorf("HAL_V1_CMS_BEARER_TOKEN is required")
	}
	if c.V1FactDeliveryEnabled {
		if err := validateHTTPURL("HAL_V1_CMS_FACTS_URL", c.V1CMSFactsURL); err != nil {
			return err
		}
		if strings.TrimSpace(c.V1CMSFactsBearerToken) == "" {
			return fmt.Errorf("HAL_V1_CMS_FACT_BEARER_TOKEN is required when HAL_V1_FACT_DELIVERY_ENABLED=true")
		}
	}
	if strings.TrimSpace(c.DatabaseURL) != "" {
		if err := validatePostgresURL(c.DatabaseURL); err != nil {
			return err
		}
	} else if strings.TrimSpace(c.DBName) == "" || strings.TrimSpace(c.DBUser) == "" || strings.TrimSpace(c.DBPassword) == "" || strings.TrimSpace(c.DBHost) == "" {
		return fmt.Errorf("DATABASE_URL or DB_NAME, DB_USER, DB_PASSWORD, and DB_HOST are required")
	}
	switch c.DBSSLMode {
	case "disable", "allow", "prefer", "require", "verify-ca", "verify-full":
	default:
		return fmt.Errorf("invalid DB_SSLMODE: %q", c.DBSSLMode)
	}
	return nil
}

func parseEnvironment(raw string) (string, error) {
	parsed := strings.ToLower(strings.TrimSpace(raw))
	switch parsed {
	case "development", "test", "production":
		return parsed, nil
	default:
		return "", fmt.Errorf("invalid HAL_ENVIRONMENT: %q (supported: development, test, production)", raw)
	}
}

func parsePort(key, raw string) (string, error) {
	value, err := parseBoundedInt(key, raw, 1, 65535)
	if err != nil {
		return "", err
	}
	return strconv.Itoa(value), nil
}
func parseBoundedInt(key, raw string, minimum, maximum int) (int, error) {
	parsed, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || parsed < minimum || parsed > maximum {
		return 0, fmt.Errorf("invalid %s: must be an integer between %d and %d", key, minimum, maximum)
	}
	return parsed, nil
}
func parseBool(key, raw string) (bool, error) {
	parsed, err := strconv.ParseBool(strings.TrimSpace(raw))
	if err != nil {
		return false, fmt.Errorf("invalid %s: must be true or false", key)
	}
	return parsed, nil
}

// PostgresURL returns the configured privileged migration/runtime connection
// target without logging credentials. Callers remain responsible for their
// distinct role and lifecycle policy.
func (c Config) PostgresURL() string {
	if strings.TrimSpace(c.DatabaseURL) != "" {
		return strings.TrimSpace(c.DatabaseURL)
	}
	u := url.URL{Scheme: "postgres", User: url.UserPassword(c.DBUser, c.DBPassword), Host: net.JoinHostPort(c.DBHost, strconv.Itoa(c.DBPort)), Path: c.DBName}
	query := u.Query()
	query.Set("sslmode", c.DBSSLMode)
	u.RawQuery = query.Encode()
	return u.String()
}
func parseLogLevel(raw string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("invalid LOG_LEVEL: %q (supported: debug, info, warn, error)", raw)
	}
}
func validateHTTPURL(key, raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil || parsed.Fragment != "" {
		return fmt.Errorf("invalid %s: must be an absolute http or https URL without credentials or fragment", key)
	}
	return nil
}
func validatePostgresURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") || parsed.Host == "" || strings.Trim(parsed.Path, "/") == "" {
		return fmt.Errorf("invalid DATABASE_URL: must be an absolute postgres URL")
	}
	return nil
}
func value(values map[string]string, key, fallback string) string {
	if raw, ok := values[key]; ok {
		return strings.TrimSpace(raw)
	}
	return fallback
}
func (c Config) RESTListenAddr() string { return net.JoinHostPort(c.RESTHost, c.RESTPort) }
func (c Config) HasDatabase() bool {
	return strings.TrimSpace(c.DatabaseURL) != "" || (strings.TrimSpace(c.DBName) != "" && strings.TrimSpace(c.DBUser) != "" && strings.TrimSpace(c.DBHost) != "" && c.DBPort > 0)
}
