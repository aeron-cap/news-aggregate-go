package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aeron-cap/news-aggregator/internal/config"
	"github.com/aeron-cap/news-aggregator/internal/database"
)

func unset(t *testing.T, key string) {
	t.Helper()
	t.Setenv(key, "")
	if err := os.Unsetenv(key); err != nil {
		t.Fatal(err)
	}
}

func TestEnvFileConfiguresDatabaseAndPreservesProcessOverrides(t *testing.T) {
	unset(t, "DATABASE_URL")
	unset(t, "PGPASSWORD")
	t.Setenv("PYTHON_BIN", "service-python")
	path := filepath.Join(t.TempDir(), ".env")
	contents := "DATABASE_URL='postgresql://reader@localhost:5433/custom?sslmode=disable'\nPGPASSWORD='literal $password # with spaces'\nPYTHON_BIN=file-python\n"
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := config.LoadEnv(path); err != nil {
		t.Fatal(err)
	}
	if database.ConnectionURL() != "postgresql://reader@localhost:5433/custom?sslmode=disable" {
		t.Fatal("database did not use .env configuration")
	}
	if os.Getenv("PGPASSWORD") != "literal $password # with spaces" {
		t.Fatal("quoted password was altered")
	}
	if os.Getenv("PYTHON_BIN") != "service-python" {
		t.Fatal(".env replaced explicit process configuration")
	}
}

func TestMissingEnvFileKeepsDefaults(t *testing.T) {
	unset(t, "DATABASE_URL")
	if err := config.LoadEnv(filepath.Join(t.TempDir(), "missing.env")); err != nil {
		t.Fatal(err)
	}
	if database.ConnectionURL() != database.DefaultURL {
		t.Fatal("missing .env did not retain the local database default")
	}
}

func TestMalformedEnvDoesNotExposeSecretsOrPartiallyApplyValues(t *testing.T) {
	unset(t, "DATABASE_URL")
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("DATABASE_URL=changed\nsecret-password@invalid\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := config.LoadEnv(path)
	if err == nil || strings.Contains(err.Error(), "secret-password") {
		t.Fatalf("unsafe malformed-file result: %v", err)
	}
	if _, exists := os.LookupEnv("DATABASE_URL"); exists {
		t.Fatal("malformed file partially changed configuration")
	}
}
