package platform

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type observedStreamRequest struct {
	path string
	auth string
}

func TestProxyActionStreamsWebSocketUpgrade(t *testing.T) {
	upstream, observed := startUpgradeEchoUpstream()
	defer upstream.Close()

	app := newStreamingProxyTestApp(upstream.URL)
	server := httptest.NewServer(app)
	defer server.Close()

	conn, reader, resp := openUpgradeProxyConnection(t, server.Listener.Addr().String(), "/api/v1/ws/exec")
	defer conn.Close()

	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("status = %d, want 101", resp.StatusCode)
	}

	assertTunneledPingPong(t, conn, reader)
	got := receiveObservedStreamRequest(t, observed)
	if got.path != "/upstream/ws/exec" || got.auth != "Bearer upstream-secret" {
		t.Fatalf("upstream saw path=%q auth=%q", got.path, got.auth)
	}
}

func TestProxyActionStreamsHTTPResponse(t *testing.T) {
	var seenHeader http.Header
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenHeader = r.Header.Clone()
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("X-Upstream", "stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("chunk-1\n"))
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		_, _ = w.Write([]byte("chunk-2\n"))
	}))
	defer upstream.Close()

	app := NewApp(Config{ServiceName: "all", ExternalURLs: map[string]string{"k8s": upstream.URL}})
	app.RegisterService(ServiceSpec{
		Name: "platform-gateway",
		Routes: []RouteSpec{{
			Method:          http.MethodGet,
			Pattern:         "/api/v1/ws/pod-logs",
			Resource:        "ws_pod_logs",
			Action:          "proxy",
			ExternalAdapter: "k8s",
		}},
	})
	server := httptest.NewServer(app)
	defer server.Close()

	req, err := http.NewRequest(http.MethodGet, server.URL+"/api/v1/ws/pod-logs?stream=true", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Authorization", "Bearer caller-jwt")
	req.Header.Set("Proxy-Authorization", "Bearer proxy-jwt")
	req.Header.Set("X-API-Key", "platform-api-key")
	req.Header.Set("API-Key", "legacy-platform-api-key")
	req.Header.Set("X-Service-Key", "internal-service-key")
	req.Header.Set(testProxyHeader, "stream-ok")
	req.AddCookie(&http.Cookie{Name: "token", Value: "platform"})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK || string(body) != "chunk-1\nchunk-2\n" {
		t.Fatalf("stream response status=%d body=%q", resp.StatusCode, body)
	}
	if resp.Header.Get("X-Upstream") != "stream" || resp.Header.Get("Content-Type") != "text/plain" {
		t.Fatalf("stream headers = %#v", resp.Header)
	}
	assertSensitiveProxyHeadersAbsent(t, seenHeader)
	if seenHeader.Get(testProxyHeader) != "stream-ok" {
		t.Fatalf("upstream %s = %q, want stream-ok", testProxyHeader, seenHeader.Get(testProxyHeader))
	}
	if seenHeader.Get("Accept") != "text/event-stream" {
		t.Fatalf("upstream Accept = %q, want text/event-stream", seenHeader.Get("Accept"))
	}
}

func hijackTestResponse(w http.ResponseWriter) (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijack unsupported", http.StatusInternalServerError)
		return nil, nil, fmt.Errorf("hijack unsupported")
	}
	return hijacker.Hijack()
}

func startUpgradeEchoUpstream() (*httptest.Server, <-chan observedStreamRequest) {
	observed := make(chan observedStreamRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		observed <- observedStreamRequest{path: r.URL.Path, auth: r.Header.Get("Authorization")}
		if !isUpgradeRequest(r) {
			http.Error(w, "upgrade required", http.StatusBadRequest)
			return
		}
		echoHijackedPing(w)
	}))
	return server, observed
}

func echoHijackedPing(w http.ResponseWriter) {
	conn, rw, err := hijackTestResponse(w)
	if err != nil {
		return
	}
	defer conn.Close()
	_, _ = rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: websocket\r\nSec-WebSocket-Accept: test\r\n\r\n")
	if err := rw.Flush(); err != nil {
		return
	}
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	in := make([]byte, 4)
	if _, err := io.ReadFull(rw, in); err == nil && string(in) == "ping" {
		_, _ = conn.Write([]byte("pong"))
	}
}

func newStreamingProxyTestApp(upstreamURL string) *App {
	app := NewApp(Config{
		ServiceName:  "all",
		ExternalURLs: map[string]string{"k8s": upstreamURL},
		AdapterConfigs: map[string]AdapterConfig{
			"k8s": {StripPrefix: "/api/v1", AddPrefix: "/upstream", Auth: AdapterAuthConfig{Type: "bearer", Token: "upstream-secret"}},
		},
	})
	app.RegisterService(ServiceSpec{
		Name: "platform-gateway",
		Routes: []RouteSpec{{
			Method:          http.MethodGet,
			Pattern:         "/api/v1/ws/exec",
			Resource:        "ws_exec",
			Action:          "proxy",
			ExternalAdapter: "k8s",
		}},
	})
	return app
}

func openUpgradeProxyConnection(t *testing.T, addr, path string) (net.Conn, *bufio.Reader, *http.Response) {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	_, _ = fmt.Fprintf(conn, "GET %s HTTP/1.1\r\nHost: %s\r\nConnection: Upgrade\r\nUpgrade: websocket\r\nSec-WebSocket-Key: test\r\nSec-WebSocket-Version: 13\r\n\r\n", path, addr)
	reader := bufio.NewReader(conn)
	resp, err := http.ReadResponse(reader, nil)
	if err != nil {
		_ = conn.Close()
		t.Fatal(err)
	}
	return conn, reader, resp
}

func assertTunneledPingPong(t *testing.T, conn net.Conn, reader *bufio.Reader) {
	t.Helper()
	_, _ = conn.Write([]byte("ping"))
	reply := make([]byte, 4)
	if _, err := io.ReadFull(reader, reply); err != nil {
		t.Fatalf("read tunneled reply: %v", err)
	}
	if string(reply) != "pong" {
		t.Fatalf("tunneled reply = %q, want pong", reply)
	}
}

func receiveObservedStreamRequest(t *testing.T, observed <-chan observedStreamRequest) observedStreamRequest {
	t.Helper()
	select {
	case got := <-observed:
		return got
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for upstream request")
		return observedStreamRequest{}
	}
}
