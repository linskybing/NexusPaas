package platform

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Limiter is the rate-limiting port the guard chain depends on. The in-memory
// *RateLimiter is the default; a Redis-backed limiter can be injected via
// WithRateLimiter so counters are shared across replicas (finding 4). Allow uses
// the limiter's configured default quota; AllowWithin lets the caller supply a
// per-route-class quota so build/transfer/workload/auth get tighter caps. (P1-7)
type Limiter interface {
	Allow(key string) bool
	AllowWithin(key string, limit int, window time.Duration) bool
}

var _ Limiter = (*RateLimiter)(nil)

type RateLimiter struct {
	mu       sync.Mutex
	limit    int
	window   time.Duration
	counters map[string]rateCounter
}

type rateCounter struct {
	count     int
	expiresAt time.Time
}

func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{limit: limit, window: window, counters: map[string]rateCounter{}}
}

func (r *RateLimiter) Allow(key string) bool {
	return r.AllowWithin(key, r.limit, r.window)
}

func (r *RateLimiter) AllowWithin(key string, limit int, window time.Duration) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	r.deleteExpiredLocked(now)
	counter := r.counters[key]
	if counter.expiresAt.Before(now) {
		counter = rateCounter{expiresAt: now.Add(window)}
	}
	counter.count++
	r.counters[key] = counter
	return counter.count <= limit
}

func (r *RateLimiter) deleteExpiredLocked(now time.Time) {
	for key, counter := range r.counters {
		if counter.expiresAt.Before(now) {
			delete(r.counters, key)
		}
	}
}

// Route classes with quotas distinct from ordinary reads. Resource-heavy and
// security-sensitive routes get tighter caps so one principal cannot flood image
// builds / data transfers / workload submits or brute-force login/token endpoints.
const (
	rateClassDefault  = "default"
	rateClassBuild    = "build"
	rateClassTransfer = "transfer"
	rateClassWorkload = "workload"
	rateClassAuth     = "auth"
)

type rateLimitRule struct {
	limit  int
	window time.Duration
}

// specialRateLimit returns a tighter per-class quota, or false for the default
// class (which uses the limiter's configured default so a single global cap still
// applies to everything else). All windows are one minute to match Retry-After.
func specialRateLimit(class string) (rateLimitRule, bool) {
	switch class {
	case rateClassBuild:
		return rateLimitRule{limit: 30, window: time.Minute}, true
	case rateClassTransfer:
		return rateLimitRule{limit: 60, window: time.Minute}, true
	case rateClassWorkload:
		return rateLimitRule{limit: 60, window: time.Minute}, true
	case rateClassAuth:
		return rateLimitRule{limit: 20, window: time.Minute}, true
	default:
		return rateLimitRule{}, false
	}
}

func rateLimitClass(route RouteSpec) string {
	p := route.Pattern
	switch {
	case strings.Contains(p, "/images/build"):
		return rateClassBuild
	case strings.Contains(p, "/storage/transfers"):
		return rateClassTransfer
	case route.Method == http.MethodPost && (p == "/api/v1/jobs" || strings.HasPrefix(p, "/api/v1/jobs/")):
		return rateClassWorkload
	case strings.Contains(p, "/login") || strings.Contains(p, "/api-tokens"):
		return rateClassAuth
	default:
		return rateClassDefault
	}
}

// rateLimitKey scopes the counter to the route class and the acting principal
// (verified user on authed routes, else client IP), so each class has its own
// per-principal budget and abuse in one class cannot exhaust another.
func rateLimitKey(r *http.Request, route RouteSpec, class string, trustedProxies []*net.IPNet) string {
	if route.AuthRequired {
		if user, ok := verifiedUser(r); ok {
			if id := asString(user["id"]); id != "" {
				return class + "|user:" + id
			}
		}
	}
	return class + "|ip:" + clientKey(r, trustedProxies)
}

func clientKey(r *http.Request, trustedProxies []*net.IPNet) string {
	return ClientIPFromRequest(r, trustedProxies)
}

func ClientIPFromRequest(r *http.Request, trustedProxies []*net.IPNet) string {
	remoteIP, remoteKey := remoteIPAndKey(r.RemoteAddr)
	if len(trustedProxies) == 0 || remoteIP == nil || !isTrustedProxy(remoteIP, trustedProxies) {
		return remoteKey
	}
	forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-For"))
	if forwarded == "" {
		return remoteKey
	}
	hops := strings.Split(forwarded, ",")
	for i := len(hops) - 1; i >= 0; i-- {
		ip := parseForwardedHop(hops[i])
		if ip == nil || isTrustedProxy(ip, trustedProxies) {
			continue
		}
		return ip.String()
	}
	return remoteKey
}

func remoteIPAndKey(remoteAddr string) (net.IP, string) {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil || host == "" {
		host = strings.Trim(remoteAddr, "[]")
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return nil, remoteAddr
	}
	return ip, ip.String()
}

func parseForwardedHop(value string) net.IP {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if host, port, err := net.SplitHostPort(value); err == nil {
		if _, err := strconv.Atoi(port); err != nil {
			return nil
		}
		value = host
	} else if strings.Count(value, ":") == 1 {
		return nil
	}
	value = strings.Trim(value, "[]")
	return net.ParseIP(value)
}

func isTrustedProxy(ip net.IP, trustedProxies []*net.IPNet) bool {
	for _, cidr := range trustedProxies {
		if cidr != nil && cidr.Contains(ip) {
			return true
		}
	}
	return false
}
