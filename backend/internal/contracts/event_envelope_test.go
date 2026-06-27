package contracts

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestEventEnvelopeFixturesAreValidV1(t *testing.T) {
	fixtures := eventFixtureFiles(t)
	want := []string{
		"accelerator-profile-changed.json",
		"audit-event.json",
		"cache-binding-changed.json",
		"data-plane-plan-built.json",
		"fast-transfer-completed.json",
		"fast-transfer-failed.json",
		"fast-transfer-progressed.json",
		"fast-transfer-queued.json",
		"job-submitted.json",
		"network-profile-changed.json",
		"placement-profile-changed.json",
		"project-updated.json",
		"quota-reserved.json",
		"storage-benchmark-recorded.json",
		"storage-profile-changed.json",
		"user-updated.json",
	}
	if !reflect.DeepEqual(fixtures, want) {
		t.Fatalf("fixture files = %v, want %v", fixtures, want)
	}

	seenTypes := make(map[string]string, len(fixtures))
	for _, name := range fixtures {
		event := readEventEnvelopeFixture(t, name)
		if event.SchemaVersion != 1 {
			t.Fatalf("%s schema_version = %d, want 1", name, event.SchemaVersion)
		}
		if event.EventType == "" || event.Producer == "" || event.AggregateID == "" {
			t.Fatalf("%s has incomplete routing metadata: %#v", name, event)
		}
		if event.TraceID == "" {
			t.Fatalf("%s missing trace_id", name)
		}
		if event.OccurredAt.IsZero() {
			t.Fatalf("%s missing occurred_at", name)
		}
		if len(event.Payload) == 0 {
			t.Fatalf("%s has empty payload", name)
		}
		seenTypes[event.EventType] = event.Producer
	}

	wantTypes := map[string]string{
		"AcceleratorProfileChanged": "scheduler-quota-service",
		"AuditEvent":               "scheduler-quota-service",
		"CacheBindingChanged":      "storage-service",
		"DataPlanePlanBuilt":       "storage-service",
		"FastTransferCompleted":    "storage-service",
		"FastTransferFailed":       "storage-service",
		"FastTransferProgressed":   "storage-service",
		"FastTransferQueued":       "storage-service",
		"JobSubmitted":             "workload-service",
		"NetworkProfileChanged":    "scheduler-quota-service",
		"PlacementProfileChanged":  "scheduler-quota-service",
		"ProjectUpdated":           "org-project-service",
		"QuotaReserved":            "scheduler-quota-service",
		"StorageBenchmarkRecorded": "storage-service",
		"StorageProfileChanged":    "storage-service",
		"UserUpdated":              "identity-service",
	}
	if !reflect.DeepEqual(seenTypes, wantTypes) {
		t.Fatalf("fixture event type/producers = %v, want %v", seenTypes, wantTypes)
	}
}

func TestEventEnvelopeCompatibilityAllowsAdditiveFieldsAndMissingRequestID(t *testing.T) {
	raw := readEventEnvelopeFixtureDocument(t, "user-updated.json")
	delete(raw, "request_id")
	raw["new_top_level_field"] = "ignored by tolerant readers"

	payload, ok := raw["payload"].(map[string]any)
	if !ok {
		t.Fatalf("payload has type %T, want object", raw["payload"])
	}
	payload["new_optional_snapshot"] = map[string]any{
		"safe_label": "ga-contract-fixture",
	}

	encoded, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal mutated fixture: %v", err)
	}
	event, err := DecodeEventEnvelope(encoded)
	if err != nil {
		t.Fatalf("DecodeEventEnvelope with additive fields and no request_id: %v", err)
	}
	if event.RequestID != "" {
		t.Fatalf("request_id = %q, want omitted", event.RequestID)
	}
	if _, ok := event.Payload["new_optional_snapshot"]; !ok {
		t.Fatalf("payload additive field was not preserved")
	}
}

func TestEventEnvelopeRejectsMissingRequiredFields(t *testing.T) {
	base := validEventEnvelope()
	cases := map[string]struct {
		mutate func(*EventEnvelope)
		field  string
	}{
		"event_id":       {func(e *EventEnvelope) { e.EventID = "" }, "event_id"},
		"schema_version": {func(e *EventEnvelope) { e.SchemaVersion = 0 }, "schema_version"},
		"event_type":     {func(e *EventEnvelope) { e.EventType = "" }, "event_type"},
		"producer":       {func(e *EventEnvelope) { e.Producer = "" }, "producer"},
		"occurred_at":    {func(e *EventEnvelope) { e.OccurredAt = time.Time{} }, "occurred_at"},
		"trace_id":       {func(e *EventEnvelope) { e.TraceID = "" }, "trace_id"},
		"aggregate_id":   {func(e *EventEnvelope) { e.AggregateID = "" }, "aggregate_id"},
		"payload":        {func(e *EventEnvelope) { e.Payload = nil }, "payload"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			event := base
			tc.mutate(&event)
			err := event.Validate()
			if err == nil {
				t.Fatalf("Validate() error = nil, want missing %s", tc.field)
			}
			if !strings.Contains(err.Error(), tc.field) {
				t.Fatalf("Validate() error = %q, want mention %s", err.Error(), tc.field)
			}
		})
	}
}

func TestEventEnvelopeRejectsForbiddenPayloadKeys(t *testing.T) {
	cases := map[string]map[string]any{
		"internal_id": {
			"internal_id": "42",
		},
		"db_id_nested": {
			"resource": map[string]any{"db_id": 42},
		},
		"access_token_array": {
			"credentials": []any{map[string]any{"access_token": "redacted"}},
		},
		"password": {
			"password": "redacted",
		},
	}

	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			event := validEventEnvelope()
			event.Payload = payload
			err := event.Validate()
			if err == nil {
				t.Fatalf("Validate() error = nil, want forbidden payload key")
			}
			if !strings.Contains(err.Error(), "forbidden") {
				t.Fatalf("Validate() error = %q, want forbidden key message", err.Error())
			}
		})
	}
}

func eventFixtureFiles(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join("fixtures", "events", "v1"))
	if err != nil {
		t.Fatalf("read event fixtures: %v", err)
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		files = append(files, entry.Name())
	}
	sort.Strings(files)
	return files
}

func readEventEnvelopeFixture(t *testing.T, name string) EventEnvelope {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("fixtures", "events", "v1", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	event, err := DecodeEventEnvelope(raw)
	if err != nil {
		t.Fatalf("DecodeEventEnvelope(%s): %v", name, err)
	}
	return event
}

func readEventEnvelopeFixtureDocument(t *testing.T, name string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("fixtures", "events", "v1", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("unmarshal fixture %s: %v", name, err)
	}
	return document
}

func validEventEnvelope() EventEnvelope {
	return EventEnvelope{
		EventID:       "ffffffff-ffff-4fff-8fff-ffffffffffff",
		SchemaVersion: 1,
		EventType:     "UserUpdated",
		Producer:      "identity-service",
		OccurredAt:    time.Date(2026, 6, 19, 0, 0, 0, 0, time.UTC),
		TraceID:       "trace-valid-event-envelope",
		RequestID:     "req-valid-event-envelope",
		AggregateID:   "22222222-2222-4222-8222-222222222222",
		Payload: map[string]any{
			"user_id":      "22222222-2222-4222-8222-222222222222",
			"display_name": "Platform Operator",
		},
	}
}
