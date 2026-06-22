package platform

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientIPFromRequestUsesSameTrustedProxyResolver(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.10:1234"
	req.Header.Set("X-Forwarded-For", "198.51.100.5, 203.0.113.20")

	if got := ClientIPFromRequest(req, parseTrustedProxyCIDRs("203.0.113.0/24")); got != "198.51.100.5" {
		t.Fatalf("client IP = %q, want rightmost untrusted hop", got)
	}
}
