package scripts

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/aeron-cap/news-aggregator/internal/config"
)

func TestPythonUsesEnvLoadedAfterPackageInitialization(t *testing.T) {
	t.Setenv("PYTHON_BIN", "")
	if err := os.Unsetenv("PYTHON_BIN"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("PYTHON_BIN='configured python'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := config.LoadEnv(path); err != nil {
		t.Fatal(err)
	}
	if pythonBin() != "configured python" {
		t.Fatal("scorer ignored Python configured after package initialization")
	}
}

func TestPythonDefaultsWithoutEnvFile(t *testing.T) {
	t.Setenv("PYTHON_BIN", "")
	t.Chdir(t.TempDir())
	if pythonBin() != "python" {
		t.Fatal("missing configuration did not use PATH fallback")
	}
	venv := filepath.Join(".venv", "bin", "python")
	if runtime.GOOS == "windows" {
		venv = filepath.Join(".venv", "Scripts", "python.exe")
	}
	if err := os.MkdirAll(filepath.Dir(venv), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(venv, nil, 0o700); err != nil {
		t.Fatal(err)
	}
	if pythonBin() != venv {
		t.Fatal("project virtual environment was not selected")
	}
}
