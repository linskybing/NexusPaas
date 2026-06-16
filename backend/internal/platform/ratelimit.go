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
// WithRateLimiter so counters are shared across replicas (finding 4).
type Limiter interface {
	Allow(key string) bool
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
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	r.deleteExpiredLocked(now)
	counter := r.counters[key]
	if counter.expiresAt.Before(now) {
		counter = rateCounter{expiresAt: now.Add(r.window)}
	}
	counter.count++
	r.counters[key] = counter
	return counter.count <= r.limit
}

func (r *RateLimiter) deleteExpiredLocked(now time.Time) {
	for key, counter := range r.counters {
		if counter.expiresAt.Before(now) {
			delete(r.counters, key)
		}
	}
}

func rateLimitKey(r *http.Request, route RouteSpec, trustedProxies []*net.IPNet) string {
	if route.AuthRequired {
		if user, ok := verifiedUser(r); ok {
			if id := asString(user["id"]); id != "" {
				return "user:" + id
			}
		}
	}
	return clientKey(r, trustedProxies)
}

func clientKey(r *http.Request, trustedProxies []*net.IPNet) string {
	return clientIPFromRequest(r, trustedProxies)
}

func clientIPFromRequest(r *http.Request, trustedProxies []*net.IPNet) string {
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
