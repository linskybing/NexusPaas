package shared

import (
	"context"
	"net/http"
)

// MaintenanceRequest adapts a maintenance-task context to the *http.Request
// shape that request-scoped projection sync/drift helpers take. Those helpers
// only read the request's context; there is no HTTP exchange behind it.
func MaintenanceRequest(ctx context.Context) *http.Request {
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, "/", nil)
	if err != nil {
		// static method/URL cannot fail; keep the helper total anyway
		return (&http.Request{}).WithContext(ctx)
	}
	return r
}
