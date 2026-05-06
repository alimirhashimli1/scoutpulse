package integration

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/testcontainers/testcontainers-go/modules/compose"
)

func TestDockerComposeStack(t *testing.T) {
	composeFile := filepath.Join("..", "..", "docker-compose.yml")
	
	stack, err := compose.NewDockerCompose(composeFile)
	assert.NoError(t, err, "NewDockerCompose should not return an error")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Start the stack
	err = stack.Up(ctx)
	
	assert.NoError(t, err, "Docker Compose stack should start without error")

	// Cleanup
	defer func() {
		err := stack.Down(ctx, compose.RemoveOrphans(true), compose.RemoveVolumes(true))
		assert.NoError(t, err, "Docker Compose stack should shut down without error")
	}()

	// Verify identity-db is running (basic check)
	services := []string{"identity-db", "football-db"}
	for _, svc := range services {
		// In a real test, we would check port availability or connection strings
		t.Logf("Service %s is part of the stack", svc)
	}
}
