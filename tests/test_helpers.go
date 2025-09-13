//go:build integration

package tests

import (
	"context"
	"os/exec"
	"time"
)

// isDockerAvailable checks if Docker is available and running
func isDockerAvailable() bool {
	// Check if docker command is available
	_, err := exec.LookPath("docker")
	if err != nil {
		return false
	}

	// Check if Docker daemon is running with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", "info")
	err = cmd.Run()
	return err == nil
}
