package scripts

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"time"
)

var PythonBin = func() string {
	if bin := os.Getenv("PYTHON_BIN"); bin != "" {
		return bin
	}
	return "python"
}()

func RunResnik(ctx context.Context, scriptPath string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, PythonBin, scriptPath)
	out, err := cmd.CombinedOutput()

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			log.Printf("Script %s timed out in %s\n", scriptPath, out)
		}
		return fmt.Errorf("Failed to run script %s: %v\nOutput: %s", scriptPath, err, out)
	} 

	return nil
}