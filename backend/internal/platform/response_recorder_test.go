package platform

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeMapFallbacksAndErrors(t *testing.T) {
	emptyReq := httptest.NewRequest(http.MethodPost, "/", nil)
	if got := DecodeMap(emptyReq); len(got) != 0 {
		t.Fatalf("DecodeMap empty = %#v, want empty map", got)
	}

	invalidReq := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{"))
	if got := DecodeMap(invalidReq); len(got) != 0 {
		t.Fatalf("DecodeMap invalid = %#v, want empty map", got)
	}

	nullReq := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("null"))
	payload, err := DecodeMapWithError(nullReq)
	if err != nil || len(payload) != 0 {
		t.Fatalf("DecodeMapWithError null = %#v err=%v, want empty nil error", payload, err)
	}

	limitedReq := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"too-large"}`))
	limitedReq.Body = http.MaxBytesReader(httptest.NewRecorder(), limitedReq.Body, 4)
	_, err = DecodeMapWithError(limitedReq)
	if InputLimitStatus(err, 0) != http.StatusRequestEntityTooLarge {
		t.Fatalf("DecodeMapWithError limited status = %d, want 413: %v", InputLimitStatus(err, 0), err)
	}
	if !strings.Contains(InputLimitMessage(err, ""), "4 bytes") {
		t.Fatalf("DecodeMapWithError limited message = %q, want byte limit", InputLimitMessage(err, ""))
	}
}

func TestRawResponseRecorderCapturesRawPayload(t *testing.T) {
	rec := newRawResponseRecorder()
	rec.Header().Set(headerContentType, "text/plain")
	rec.Header().Set("X-One", "first")
	rec.Header().Add("X-One", "second")
	if _, err := rec.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	if rec.statusCode() != http.StatusOK {
		t.Fatalf("default status = %d, want 200", rec.statusCode())
	}

	raw := rec.rawResponse()
	if raw.ContentType != "text/plain" || string(raw.Body) != "hello" {
		t.Fatalf("raw response = %#v, want text/plain hello", raw)
	}
	if raw.Headers["X-One"] != "first" {
		t.Fatalf("raw header X-One = %q, want first", raw.Headers["X-One"])
	}
	if _, found := raw.Headers[headerContentType]; found {
		t.Fatalf("raw response should keep content type separate: %#v", raw.Headers)
	}

	rec.WriteHeader(http.StatusAccepted)
	if rec.statusCode() != http.StatusAccepted {
		t.Fatalf("explicit status = %d, want 202", rec.statusCode())
	}
}

func TestStatusRecorderUnwrapsWrappedWriter(t *testing.T) {
	base := httptest.NewRecorder()
	rec := &statusRecorder{ResponseWriter: base}
	if rec.Unwrap() != base {
		t.Fatal("statusRecorder did not unwrap original writer")
	}
}
