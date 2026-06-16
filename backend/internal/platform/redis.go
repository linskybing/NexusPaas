package platform

import (
	"fmt"

	"github.com/redis/go-redis/v9"
)

// newRedisClient parses a redis:// URL and returns a connected client. The
// caller owns the client lifecycle (Close on shutdown).
func newRedisClient(url string) (*redis.Client, error) {
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}
	return redis.NewClient(opts), nil
}
