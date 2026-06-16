package platform

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

type Metrics struct {
	mu       sync.RWMutex
	requests map[string]int
	latency  map[string]time.Duration
	counters map[string]int
	total    int
	errors   int
}

func NewMetrics() *Metrics {
	return &Metrics{
		requests: map[string]int{},
		latency:  map[string]time.Duration{},
		counters: map[string]int{},
	}
}

func (m *Metrics) Observe(route, method string, status int, duration time.Duration) {
	key := fmt.Sprintf(`route="%s",method="%s",status="%d"`, route, method, status)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests[key]++
	m.latency[key] += duration
	m.total++
	if status >= 500 {
		m.errors++
	}
}

func (m *Metrics) Inc(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.counters[name]++
}

func (m *Metrics) Counter(name string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.counters[name]
}

func (m *Metrics) CounterSuffix(suffix string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	total := 0
	for name, value := range m.counters {
		if strings.HasSuffix(name, suffix) {
			total += value
		}
	}
	return total
}

func (m *Metrics) ErrorRatePercent() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.total == 0 {
		return 0
	}
	return int(float64(m.errors) / float64(m.total) * 100)
}

func (m *Metrics) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	var out strings.Builder
	writeMetricHeader(&out, "nexuspaas_http_requests_total", "counter")
	for _, key := range sortedKeys(m.requests) {
		fmt.Fprintf(&out, "nexuspaas_http_requests_total{%s} %d\n", key, m.requests[key])
	}
	writeMetricHeader(&out, "nexuspaas_http_request_duration_seconds_sum", "counter")
	for _, key := range sortedKeys(m.latency) {
		fmt.Fprintf(&out, "nexuspaas_http_request_duration_seconds_sum{%s} %.6f\n", key, m.latency[key].Seconds())
	}
	for _, key := range sortedKeys(m.counters) {
		fmt.Fprintf(&out, "nexuspaas_%s_total %d\n", sanitizeMetricName(key), m.counters[key])
	}
	_, _ = w.Write([]byte(out.String()))
}

func writeMetricHeader(out *strings.Builder, name, kind string) {
	fmt.Fprintf(out, "# TYPE %s %s\n", name, kind)
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sanitizeMetricName(name string) string {
	name = strings.ReplaceAll(name, "-", "_")
	name = strings.ReplaceAll(name, ".", "_")
	name = strings.ReplaceAll(name, "/", "_")
	return name
}
