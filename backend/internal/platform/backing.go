package platform

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"
)

type BackingDependency struct {
	Name string
	URL  string
}

type BackingChecker interface {
	Check(ctx context.Context, dependency BackingDependency) error
}

// compositeBackingChecker dispatches each backing dependency to a protocol-level
// health probe (Postgres ping, Redis PING, object-store HeadBucket) built from the
// live clients, falling back to a TCP dial for any dependency without a registered
// probe. This makes readiness reflect real dependency health rather than mere TCP
// reachability (finding 13).
type compositeBackingChecker struct {
	checks   map[string]func(context.Context) error
	fallback BackingChecker
}

func (c compositeBackingChecker) Check(ctx context.Context, dependency BackingDependency) error {
	if check, ok := c.checks[dependency.Name]; ok {
		return check(ctx)
	}
	return c.fallback.Check(ctx, dependency)
}

type TCPBackingChecker struct {
	Timeout time.Duration
}

func (c TCPBackingChecker) Check(ctx context.Context, dependency BackingDependency) error {
	address, err := dependencyAddress(dependency)
	if err != nil {
		return err
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return err
	}
	return conn.Close()
}

func (c Config) BackingDependencies() []BackingDependency {
	deps := []BackingDependency{
		{Name: envDatabaseURL, URL: c.DatabaseURL},
		{Name: envRedisURL, URL: c.RedisURL},
		{Name: envEventBusURL, URL: c.EventBusURL},
	}
	if c.RequiresObjectStore() && strings.TrimSpace(c.ObjectStoreURL) != "" {
		deps = append(deps, BackingDependency{Name: envObjectStoreURL, URL: c.ObjectStoreURL})
	}
	return deps
}

func dependencyAddress(dependency BackingDependency) (string, error) {
	raw := strings.TrimSpace(dependency.URL)
	if raw == "" {
		return "", fmt.Errorf("%s is not configured", dependency.Name)
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("%s must be an absolute URL", dependency.Name)
	}
	host := parsed.Hostname()
	port := parsed.Port()
	if host == "" {
		return "", fmt.Errorf("%s host is required", dependency.Name)
	}
	if port == "" {
		port = defaultBackingPort(parsed.Scheme)
	}
	if port == "" {
		return "", fmt.Errorf("%s port is required", dependency.Name)
	}
	return net.JoinHostPort(host, port), nil
}

func defaultBackingPort(scheme string) string {
	switch strings.ToLower(scheme) {
	case "postgres", "postgresql":
		return "5432"
	case "redis", "rediss":
		return "6379"
	case "http":
		return "80"
	case "https":
		return "443"
	case "nats":
		return "4222"
	case "kafka":
		return "9092"
	case "amqp", "amqps":
		return "5672"
	default:
		return ""
	}
}
