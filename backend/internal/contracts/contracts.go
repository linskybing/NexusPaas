package contracts

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Event struct {
	EventID        string         `json:"event_id"`
	Name           string         `json:"name"`
	Source         string         `json:"source"`
	OccurredAt     time.Time      `json:"occurred_at"`
	TraceID        string         `json:"trace_id"`
	SchemaVersion  int            `json:"schema_version"`
	IdempotencyKey string         `json:"idempotency_key"`
	Data           map[string]any `json:"data,omitempty"`
}

type EventBus interface {
	Publish(ctx context.Context, event Event) error
	Consume(ctx context.Context, consumer string, event Event) (bool, error)
	Outbox() []Event
}

type EventEnvelope struct {
	EventID       string         `json:"event_id"`
	SchemaVersion int            `json:"schema_version"`
	EventType     string         `json:"event_type"`
	Producer      string         `json:"producer"`
	OccurredAt    time.Time      `json:"occurred_at"`
	TraceID       string         `json:"trace_id"`
	RequestID     string         `json:"request_id,omitempty"`
	AggregateID   string         `json:"aggregate_id"`
	Payload       map[string]any `json:"payload"`
}

func DecodeEventEnvelope(data []byte) (EventEnvelope, error) {
	var event EventEnvelope
	if err := json.Unmarshal(data, &event); err != nil {
		return EventEnvelope{}, err
	}
	if err := event.Validate(); err != nil {
		return EventEnvelope{}, err
	}
	return event, nil
}

func (e EventEnvelope) Validate() error {
	missing := make([]string, 0, 8)
	if e.EventID == "" {
		missing = append(missing, "event_id")
	}
	if e.SchemaVersion == 0 {
		missing = append(missing, "schema_version")
	}
	if e.EventType == "" {
		missing = append(missing, "event_type")
	}
	if e.Producer == "" {
		missing = append(missing, "producer")
	}
	if e.OccurredAt.IsZero() {
		missing = append(missing, "occurred_at")
	}
	if e.TraceID == "" {
		missing = append(missing, "trace_id")
	}
	if e.AggregateID == "" {
		missing = append(missing, "aggregate_id")
	}
	if e.Payload == nil {
		missing = append(missing, "payload")
	}
	if len(missing) > 0 {
		return fmt.Errorf("event envelope missing required fields: %s", strings.Join(missing, ", "))
	}
	if e.SchemaVersion < 1 {
		return fmt.Errorf("event envelope schema_version = %d, want positive version", e.SchemaVersion)
	}
	if err := validateEventEnvelopePayload("payload", e.Payload); err != nil {
		return err
	}
	return nil
}

func validateEventEnvelopePayload(path string, value any) error {
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			fieldPath := path + "." + key
			if forbiddenEventEnvelopePayloadKey(key) {
				return fmt.Errorf("event envelope payload key %q is forbidden", fieldPath)
			}
			if err := validateEventEnvelopePayload(fieldPath, item); err != nil {
				return err
			}
		}
	case []any:
		for i, item := range typed {
			if err := validateEventEnvelopePayload(fmt.Sprintf("%s[%d]", path, i), item); err != nil {
				return err
			}
		}
	}
	return nil
}

func forbiddenEventEnvelopePayloadKey(key string) bool {
	normalized := strings.NewReplacer("-", "", "_", "", " ", "").Replace(strings.ToLower(key))
	if forbiddenEventEnvelopePayloadExactKeys[normalized] {
		return true
	}
	for _, token := range forbiddenEventEnvelopePayloadTokens {
		if strings.Contains(normalized, token) {
			return true
		}
	}
	return false
}

var forbiddenEventEnvelopePayloadExactKeys = map[string]bool{
	"databaseid": true,
	"dbid":       true,
	"gormid":     true,
	"internalid": true,
	"primarykey": true,
	"rowid":      true,
}

var forbiddenEventEnvelopePayloadTokens = []string{
	"accesstoken",
	"apikey",
	"authorization",
	"cookie",
	"credential",
	"jwt",
	"password",
	"privatekey",
	"refreshtoken",
	"secret",
	"sessiontoken",
	"tokenhash",
}

type Record[T any] struct {
	ID        string    `json:"id"`
	Data      T         `json:"data"`
	Version   int       `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type PolicyEnforcer interface {
	Enforce(ctx context.Context, subject, domain, object, action string) (Decision, error)
}

type Decision struct {
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason,omitempty"`
	Version int    `json:"version"`
}

type ExternalAdapter interface {
	Call(ctx context.Context, operation string, idempotent bool) (AdapterResult, error)
}

type ProxyAdapter interface {
	Proxy(ctx context.Context, request AdapterProxyRequest) (AdapterProxyResponse, AdapterResult, error)
}

type AdapterProxyRequest struct {
	Operation  string
	Method     string
	Path       string
	RawQuery   string
	Header     http.Header
	Body       []byte
	Idempotent bool
}

type AdapterProxyResponse struct {
	StatusCode int
	Header     http.Header
	Body       []byte
}

type AdapterResult struct {
	Adapter     string `json:"adapter"`
	Operation   string `json:"operation"`
	Degraded    bool   `json:"degraded"`
	Retryable   bool   `json:"retryable"`
	Code        string `json:"code"`
	Message     string `json:"message"`
	CircuitOpen bool   `json:"circuit_open"`
}

type WorkerLease interface {
	Acquire(ctx context.Context, worker, shard string, ttl time.Duration) (bool, error)
	Release(ctx context.Context, worker, shard string) error
}
