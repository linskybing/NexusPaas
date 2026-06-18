package contracts

import (
	"context"
	"net/http"
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

type Record[T any] struct {
	ID        string    `json:"id"`
	Data      T         `json:"data"`
	Version   int       `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type PolicyDecisionPoint interface {
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
