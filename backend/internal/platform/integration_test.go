//go:build integration

package platform

import (
	"os"
	"testing"
)

// Integration tests run only with `-tags integration` against the local
// docker-compose stack (backend/deploy/local/docker-compose.yml). Each helper
// skips the test when its backing URL is not exported, so a partial stack still
// lets the rest of the suite run.

func requireTestDatabaseURL(t *testing.T) string {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping Postgres integration test")
	}
	return url
}

func requireTestRedisURL(t *testing.T) string {
	t.Helper()
	url := os.Getenv("TEST_REDIS_URL")
	if url == "" {
		t.Skip("TEST_REDIS_URL not set; skipping Redis integration test")
	}
	return url
}
