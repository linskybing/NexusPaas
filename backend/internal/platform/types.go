package platform

import "net/http"

type ServiceSpec struct {
	Name        string      `json:"name"`
	Category    string      `json:"category"`
	Phase       string      `json:"phase"`
	Description string      `json:"description"`
	Routes      []RouteSpec `json:"routes"`
	Events      []string    `json:"events"`
	Tables      []string    `json:"tables"`
	// RequiresCluster marks services whose runtime workers/handlers depend on the
	// Kubernetes cluster facade (App.Cluster). When a hosted service requires the
	// cluster, readiness fails closed if the cluster client is absent or unreachable
	// (see App.ready) so Kubernetes stops routing to a silently degraded pod.
	RequiresCluster bool `json:"requires_cluster,omitempty"`
}

type RouteSpec struct {
	Method          string `json:"method"`
	Pattern         string `json:"pattern"`
	OperationID     string `json:"operation_id"`
	Resource        string `json:"resource"`
	Action          string `json:"action"`
	IDParam         string `json:"id_param,omitempty"`
	AuthRequired    bool   `json:"auth_required"`
	Admin           bool   `json:"admin"`
	StateChanging   bool   `json:"state_changing"`
	PolicyBypass    bool   `json:"policy_bypass,omitempty"`
	ExternalAdapter string `json:"external_adapter,omitempty"`
}

type HandlerFunc func(app *App, r *http.Request, route RouteSpec) (int, any, *Degraded)

// ActionHandler resolves a route's RouteSpec.Action to a status and response
// body. Actions are registered in a data-driven registry so new behaviors can
// be added via RegisterAction without editing the platform dispatch core (OCP).
type ActionHandler func(r *httpRequest, route RouteSpec) (int, any)

type actionDegradedResponse struct {
	data     any
	degraded Degraded
}

const unboundProxyAdapter = "unbound_proxy"

type httpRequest struct {
	*http.Request
	Service        string
	TraceID        string
	IdempotencyKey string
}

const (
	headerAPITokenID  = "X-API-Token-ID"
	headerRequestID   = "X-Request-ID"
	headerTraceID     = "X-Trace-ID"
	headerUserID      = "X-User-ID"
	headerUsername    = "X-Username"
	headerUserRole    = "X-User-Role"
	headerAdmin       = "X-Admin"
	headerContentType = "Content-Type"

	errInvalidRequestBody = "invalid request body"
)
