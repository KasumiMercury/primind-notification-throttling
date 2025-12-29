package testutil

import (
	"context"
	"testing"

	"github.com/redis/go-redis/v9"
	redismodule "github.com/testcontainers/testcontainers-go/modules/redis"
)

func SetupRedisContainer(ctx context.Context, t *testing.T) (*redis.Client, func()) {
	t.Helper()

	defer func() {
		if r := recover(); r != nil {
			t.Skipf("failed to start redis container: %v", r)
		}
	}()

	container, err := redismodule.Run(ctx, "redis:8-alpine")
	if err != nil {
		t.Skipf("failed to start redis container: %v", err)
	}

	endpoint, err := container.Endpoint(ctx, "")
	if err != nil {
		t.Skipf("failed to get redis endpoint: %v", err)
	}

	client := redis.NewClient(&redis.Options{
		Addr: endpoint,
	})

	cleanup := func() {
		if err := client.Close(); err != nil {
			t.Logf("failed to close redis client: %v", err)
		}

		if err := container.Terminate(ctx); err != nil {
			t.Logf("failed to terminate redis container: %v", err)
		}
	}

	return client, cleanup
}
