package platform

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	sigyaml "sigs.k8s.io/yaml"
)

// TestPrometheusAlertRulesParity pins the plain-Prometheus rules ConfigMap
// (prometheus.yaml, operator-less live-evidence path) to the PrometheusRule
// object (prometheus-rules.yaml, prometheus-operator path): the same alert
// groups must exist in both, byte-for-semantic-byte, so the two deployment
// paths cannot drift apart.
func TestPrometheusAlertRulesParity(t *testing.T) {
	dir := filepath.Join("..", "..", "deploy", "observability", "production-beta")

	operatorGroups := prometheusRuleGroups(t, filepath.Join(dir, "prometheus-rules.yaml"))
	plainGroups := plainPrometheusRuleGroups(t, filepath.Join(dir, "prometheus.yaml"))

	if !reflect.DeepEqual(operatorGroups, plainGroups) {
		t.Fatalf("alert rule groups drifted between PrometheusRule and the plain-Prometheus ConfigMap:\noperator: %#v\nplain:    %#v", operatorGroups, plainGroups)
	}
	if groups, _ := operatorGroups.([]any); len(groups) == 0 {
		t.Fatal("no alert groups found — parity test is vacuous")
	}
}

func prometheusRuleGroups(t *testing.T, path string) any {
	t.Helper()
	for _, doc := range yamlDocuments(t, path) {
		if doc["kind"] != "PrometheusRule" {
			continue
		}
		spec, _ := doc["spec"].(map[string]any)
		if spec["groups"] == nil {
			t.Fatalf("%s: PrometheusRule has no spec.groups", path)
		}
		return spec["groups"]
	}
	t.Fatalf("%s: no PrometheusRule document found", path)
	return nil
}

func plainPrometheusRuleGroups(t *testing.T, path string) any {
	t.Helper()
	for _, doc := range yamlDocuments(t, path) {
		if doc["kind"] != "ConfigMap" {
			continue
		}
		meta, _ := doc["metadata"].(map[string]any)
		if meta["name"] != "prometheus-alert-rules" {
			continue
		}
		data, _ := doc["data"].(map[string]any)
		raw, _ := data["nexuspaas-alerts.yaml"].(string)
		if raw == "" {
			t.Fatalf("%s: prometheus-alert-rules ConfigMap has no nexuspaas-alerts.yaml key", path)
		}
		var rules map[string]any
		if err := sigyaml.Unmarshal([]byte(raw), &rules); err != nil {
			t.Fatalf("%s: parse embedded rules: %v", path, err)
		}
		if rules["groups"] == nil {
			t.Fatalf("%s: embedded rules have no groups", path)
		}
		return rules["groups"]
	}
	t.Fatalf("%s: no prometheus-alert-rules ConfigMap found", path)
	return nil
}

func yamlDocuments(t *testing.T, path string) []map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var docs []map[string]any
	for _, chunk := range strings.Split(string(raw), "\n---\n") {
		if strings.TrimSpace(chunk) == "" {
			continue
		}
		var doc map[string]any
		if err := sigyaml.Unmarshal([]byte(chunk), &doc); err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		if len(doc) > 0 {
			docs = append(docs, doc)
		}
	}
	return docs
}
