package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadLocalEnvPreservesProcessEnvironment(t *testing.T) {
	temporary := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(previous) }()
	if err := os.Chdir(temporary); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(temporary, ".env"), []byte("F_SERVER_PORT=19999\nHAL_V1_CMS_BEARER_TOKEN=local-token\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("F_SERVER_PORT", "18888")
	t.Setenv("HAL_V1_CMS_BEARER_TOKEN", "")
	loaded := Load()
	if loaded.RESTPort != "18888" {
		t.Fatalf("RESTPort=%q, want process value", loaded.RESTPort)
	}
	if loaded.V1CMSBearerToken != "local-token" {
		t.Fatalf("V1CMSBearerToken=%q", loaded.V1CMSBearerToken)
	}
}

func TestProductionDoesNotDependOnLocalEnv(t *testing.T) {
	temporary := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(previous) }()
	if err := os.Chdir(temporary); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(temporary, ".env"), []byte("DATABASE_URL=postgres://local-only\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HAL_ENVIRONMENT", "production")
	t.Setenv("DATABASE_URL", "")
	if Load().DatabaseURL != "" {
		t.Fatal("production must not load repository .env")
	}
}
