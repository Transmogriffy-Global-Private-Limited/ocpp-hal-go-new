package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadDefaultsOnlyAbsentValues(t *testing.T) {
	withTempWorkingDirectory(t)
	configureValidRuntime(t)
	for _, key := range []string{"OCPP_LISTEN_PORT", "OCPP_HEARTBEAT_INTERVAL_SECONDS", "HAL_V1_FACT_DELIVERY_ENABLED", "API_DOCS_ENABLED", "LOG_LEVEL"} {
		unsetEnvironment(t, key)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.OCPPListenPort != defaultOCPPListenPort || cfg.OCPPHeartbeatIntervalSeconds != defaultHeartbeatIntervalSeconds || cfg.V1FactDeliveryEnabled || cfg.APIDocsEnabled || cfg.LogLevel.String() != "INFO" {
		t.Fatalf("unexpected defaults: %#v", cfg)
	}
}

func TestLoadRejectsExplicitEmptyValues(t *testing.T) {
	for _, key := range []string{"HAL_ENVIRONMENT", "OCPP_LISTEN_PORT", "HAL_V1_FACT_DELIVERY_ENABLED", "HAL_V1_CMS_BEARER_TOKEN"} {
		t.Run(key, func(t *testing.T) {
			withTempWorkingDirectory(t)
			configureValidRuntime(t)
			t.Setenv(key, "")
			if _, err := Load(); err == nil || !strings.Contains(err.Error(), key) {
				t.Fatalf("error=%v, want explicit %s validation", err, key)
			}
		})
	}
}

func TestProcessEmptyValueBlocksLocalFallback(t *testing.T) {
	temporary := withTempWorkingDirectory(t)
	if err := os.WriteFile(filepath.Join(temporary, ".env"), []byte("F_SERVER_PORT=19999\n"), 0600); err != nil {
		t.Fatal(err)
	}
	configureValidRuntime(t)
	t.Setenv("F_SERVER_PORT", "")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "F_SERVER_PORT") {
		t.Fatalf("error=%v, want explicit process empty port failure", err)
	}
}

func TestLoadRejectsMalformedExplicitValues(t *testing.T) {
	for _, test := range []struct{ key, value string }{{"HAL_V1_FACT_DELIVERY_ENABLED", "treu"}, {"OCPP_LISTEN_PORT", "not-a-number"}, {"OCPP_LISTEN_PORT", "65536"}, {"OCPP_HEARTBEAT_INTERVAL_SECONDS", "0"}, {"HAL_ENVIRONMENT", "prodution"}, {"LOG_LEVEL", "verbose"}} {
		t.Run(test.key+"="+test.value, func(t *testing.T) {
			withTempWorkingDirectory(t)
			configureValidRuntime(t)
			t.Setenv(test.key, test.value)
			_, err := Load()
			if err == nil || !strings.Contains(err.Error(), test.key) {
				t.Fatalf("error=%v, want %s validation", err, test.key)
			}
		})
	}
}

func TestLoadRequiresFactReceiverConfigurationWhenEnabled(t *testing.T) {
	withTempWorkingDirectory(t)
	configureValidRuntime(t)
	t.Setenv("HAL_V1_FACT_DELIVERY_ENABLED", "true")
	t.Setenv("HAL_V1_CMS_FACTS_URL", "")
	t.Setenv("HAL_V1_CMS_FACT_BEARER_TOKEN", "")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "HAL_V1_CMS_FACTS_URL") {
		t.Fatalf("error=%v", err)
	}
}

func TestLoadRequiresIndependentTraceReceiverConfigurationWhenEnabled(t *testing.T) {
	withTempWorkingDirectory(t)
	configureValidRuntime(t)
	t.Setenv("HAL_V1_TRACE_DELIVERY_ENABLED", "true")
	t.Setenv("HAL_V1_CMS_TRACE_URL", "")
	t.Setenv("HAL_V1_CMS_TRACE_BEARER_TOKEN", "")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "HAL_V1_CMS_TRACE_URL") {
		t.Fatalf("error=%v", err)
	}
}

func TestProductionDoesNotConsumeLocalEnvAndProcessValuesWin(t *testing.T) {
	temporary := withTempWorkingDirectory(t)
	if err := os.WriteFile(filepath.Join(temporary, ".env"), []byte("HAL_V1_CMS_BEARER_TOKEN=local-token\nDATABASE_URL=postgres://local:secret@localhost/local\nF_SERVER_PORT=19999\n"), 0600); err != nil {
		t.Fatal(err)
	}
	configureValidRuntime(t)
	t.Setenv("HAL_ENVIRONMENT", "production")
	t.Setenv("F_SERVER_PORT", "18888")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RESTPort != "18888" || cfg.V1CMSBearerToken != "runtime-token" || cfg.DatabaseURL != "postgres://runtime:secret@localhost/runtime" {
		t.Fatalf("production loaded local state: %#v", cfg)
	}
}

func TestDevelopmentLoadsLocalDefaultsWithoutOverridingProcess(t *testing.T) {
	temporary := withTempWorkingDirectory(t)
	if err := os.WriteFile(filepath.Join(temporary, ".env"), []byte("F_SERVER_PORT=19999\nOCPP_HEARTBEAT_INTERVAL_SECONDS=120\nHAL_V1_CMS_BEARER_TOKEN=local-token\nDATABASE_URL=postgres://local:secret@localhost/local\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HAL_ENVIRONMENT", "development")
	t.Setenv("F_SERVER_PORT", "18888")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RESTPort != "18888" || cfg.OCPPHeartbeatIntervalSeconds != 120 || cfg.V1CMSBearerToken != "local-token" {
		t.Fatalf("unexpected precedence: %#v", cfg)
	}
}

func configureValidRuntime(t *testing.T) {
	t.Helper()
	t.Setenv("HAL_ENVIRONMENT", "test")
	t.Setenv("HAL_V1_CMS_BEARER_TOKEN", "runtime-token")
	t.Setenv("DATABASE_URL", "postgres://runtime:secret@localhost/runtime")
	t.Setenv("DB_NAME", "")
	t.Setenv("DB_USER", "")
	t.Setenv("DB_PASSWORD", "")
	t.Setenv("DB_HOST", "")
	t.Setenv("HAL_V1_FACT_DELIVERY_ENABLED", "false")
	t.Setenv("HAL_V1_CMS_FACTS_URL", "")
	t.Setenv("HAL_V1_CMS_FACT_BEARER_TOKEN", "")
	t.Setenv("HAL_V1_TRACE_DELIVERY_ENABLED", "false")
	t.Setenv("HAL_V1_CMS_TRACE_URL", "")
	t.Setenv("HAL_V1_CMS_TRACE_BEARER_TOKEN", "")
}

func unsetEnvironment(t *testing.T, key string) {
	t.Helper()
	previous, present := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if present {
			_ = os.Setenv(key, previous)
			return
		}
		_ = os.Unsetenv(key)
	})
}

func withTempWorkingDirectory(t *testing.T) string {
	t.Helper()
	temporary := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
	if err := os.Chdir(temporary); err != nil {
		t.Fatal(err)
	}
	return temporary
}
