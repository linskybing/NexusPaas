package services

import (
	"net/http"
	"testing"
)

// TestDashboardReflectsRealFormAndAuditFlows drives real handlers — register a
// user, create a form through the request-notification handler — and asserts the
// dashboard sees the form (write-through read model) and the audit event emitted
// by that state-changing request (shared outbox+store reader). This exercises
// the actual flows rather than seeding the store directly, which is what masked
// the split-brain in finding 32.
func TestDashboardReflectsRealFormAndAuditFlows(t *testing.T) {
	app := newTestApp()

	// Register a user via the real identity handler; first user id is US2600001.
	assertNoData(t, requestJSON(t, app, http.MethodPost, "/api/v1/register",
		`{"username":"erin","password":"correct-password"}`, nil, http.StatusOK))
	userID := "US2600001"
	userHdr := map[string]string{"X-User-ID": userID, "X-Username": "erin"}

	// Create a real form (state-changing → mirrored to store + emits an AuditEvent).
	requestJSON(t, app, http.MethodPost, "/api/v1/forms",
		`{"title":"Need GPU","description":"please allocate"}`, userHdr, http.StatusCreated)

	// Dashboard overview must reflect the form via the store-backed read model.
	overview := responseMap(t, requestJSON(t, app, http.MethodGet, "/api/v1/dashboard/overview", "", userHdr, http.StatusOK))
	if got := len(overview["activities"].([]any)); got != 1 {
		t.Fatalf("overview activities = %d, want 1 (form should appear via write-through)", got)
	}

	// Admin summary must count the pending form and surface the emitted audit event.
	adminHdr := map[string]string{"X-User-ID": userID, "X-Username": "erin", "X-User-Role": "admin"}
	summary := responseMap(t, requestJSON(t, app, http.MethodGet, "/api/v1/admin/dashboard-summary", "", adminHdr, http.StatusOK))
	if summary["pendingRequestsCount"] != float64(1) {
		t.Fatalf("pendingRequestsCount = %#v, want 1", summary["pendingRequestsCount"])
	}
	if got := len(summary["recentLogs"].([]any)); got < 1 {
		t.Fatalf("recentLogs = %d, want >=1 (audit event from form creation should appear)", got)
	}
}
