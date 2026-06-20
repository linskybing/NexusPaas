package platform

import (
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

type Metrics struct {
	mu             sync.RWMutex
	requests       map[string]int
	latency        map[string]time.Duration
	latencyBuckets map[string][]int
	latencyCount   map[string]int
	counters       map[string]int
	labeled        map[string]metricSample
	total          int
	errors         int
}

type metricSample struct {
	name   string
	kind   string
	labels string
	value  int64
}

var httpDurationBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.3, 0.5, 1, 2, 5, 10}
var prometheusLabelNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func NewMetrics() *Metrics {
	return &Metrics{
		requests:       map[string]int{},
		latency:        map[string]time.Duration{},
		latencyBuckets: map[string][]int{},
		latencyCount:   map[string]int{},
		counters:       map[string]int{},
		labeled:        map[string]metricSample{},
	}
}

func (m *Metrics) Observe(route, method string, status int, duration time.Duration) {
	key := fmt.Sprintf(`route="%s",method="%s",status="%d"`, route, method, status)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests[key]++
	m.latency[key] += duration
	buckets := m.latencyBuckets[key]
	if buckets == nil {
		buckets = make([]int, len(httpDurationBuckets))
		m.latencyBuckets[key] = buckets
	}
	seconds := duration.Seconds()
	for i, boundary := range httpDurationBuckets {
		if seconds <= boundary {
			buckets[i]++
		}
	}
	m.latencyCount[key]++
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

func (m *Metrics) SetGauge(name string, labels map[string]string, value int64) {
	m.setLabeledMetric(name, "gauge", labels, value)
}

func (m *Metrics) SetCounter(name string, labels map[string]string, value int64) {
	m.setLabeledMetric(name, "counter", labels, value)
}

func (m *Metrics) setLabeledMetric(name, kind string, labels map[string]string, value int64) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	labelText, ok := prometheusLabels(labels)
	if !ok {
		return
	}
	key := name + "\xff" + labelText
	m.mu.Lock()
	defer m.mu.Unlock()
	m.labeled[key] = metricSample{name: name, kind: kind, labels: labelText, value: value}
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
	writeMetricHeader(&out, "nexuspaas_http_request_duration_seconds", "histogram")
	for _, key := range sortedKeys(m.latency) {
		for i, boundary := range httpDurationBuckets {
			fmt.Fprintf(&out, "nexuspaas_http_request_duration_seconds_bucket{%s,le=\"%s\"} %d\n", key, formatBucketBoundary(boundary), m.latencyBuckets[key][i])
		}
		fmt.Fprintf(&out, "nexuspaas_http_request_duration_seconds_bucket{%s,le=\"+Inf\"} %d\n", key, m.latencyCount[key])
		fmt.Fprintf(&out, "nexuspaas_http_request_duration_seconds_sum{%s} %.6f\n", key, m.latency[key].Seconds())
		fmt.Fprintf(&out, "nexuspaas_http_request_duration_seconds_count{%s} %d\n", key, m.latencyCount[key])
	}
	for _, key := range sortedKeys(m.counters) {
		fmt.Fprintf(&out, "nexuspaas_%s_total %d\n", sanitizeMetricName(key), m.counters[key])
	}
	lastLabeledMetric := ""
	for _, sample := range sortedMetricSamples(m.labeled) {
		if sample.name != lastLabeledMetric {
			writeMetricHeader(&out, sample.name, sample.kind)
			lastLabeledMetric = sample.name
		}
		if sample.labels == "" {
			fmt.Fprintf(&out, "%s %d\n", sample.name, sample.value)
			continue
		}
		fmt.Fprintf(&out, "%s{%s} %d\n", sample.name, sample.labels, sample.value)
	}
	_, _ = w.Write([]byte(out.String()))
}

func writeMetricHeader(out *strings.Builder, name, kind string) {
	fmt.Fprintf(out, "# TYPE %s %s\n", name, kind)
}

func formatBucketBoundary(boundary float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.3f", boundary), "0"), ".")
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedMetricSamples(samples map[string]metricSample) []metricSample {
	out := make([]metricSample, 0, len(samples))
	for _, sample := range samples {
		out = append(out, sample)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].name != out[j].name {
			return out[i].name < out[j].name
		}
		return out[i].labels < out[j].labels
	})
	return out
}

func sanitizeMetricName(name string) string {
	name = strings.ReplaceAll(name, "-", "_")
	name = strings.ReplaceAll(name, ".", "_")
	name = strings.ReplaceAll(name, "/", "_")
	return name
}

func prometheusLabels(labels map[string]string) (string, bool) {
	if len(labels) == 0 {
		return "", true
	}
	parts := make([]string, 0, len(labels))
	for _, name := range sortedKeys(labels) {
		if !validPrometheusLabelName(name) {
			return "", false
		}
		parts = append(parts, fmt.Sprintf(`%s="%s"`, name, escapePrometheusLabelValue(labels[name])))
	}
	return strings.Join(parts, ","), true
}

func validPrometheusLabelName(name string) bool {
	return prometheusLabelNamePattern.MatchString(name)
}

func escapePrometheusLabelValue(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "\n", "\\n")
	value = strings.ReplaceAll(value, "\"", "\\\"")
	return value
}
