package scripts

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/google/uuid"
)

func pythonBin() string {
	if bin := os.Getenv("PYTHON_BIN"); bin != "" {
		return bin
	}
	bin := filepath.Join(".venv", "bin", "python")
	if runtime.GOOS == "windows" {
		bin = filepath.Join(".venv", "Scripts", "python.exe")
	}
	if info, err := os.Stat(bin); err == nil && !info.IsDir() {
		return bin
	}
	return "python"
}

func RunResnik(ctx context.Context, scriptPath string, userID uuid.UUID, databaseURL string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, pythonBin(), scriptPath, "--user-id", userID.String())
	// Keep credentials out of process arguments and captured error output.
	cmd.Env = append(os.Environ(), "DATABASE_URL="+databaseURL)
	err := cmd.Run()

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("interest scoring timed out: %w", ctx.Err())
		}
		return fmt.Errorf("interest scoring failed: %w", err)
	}

	return nil
}
