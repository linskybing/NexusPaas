package platform

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// header resolves the upstream auth configuration to a concrete (name, value)
// header pair, or ("","") when no usable credential is configured.
func (c AdapterAuthConfig) header() (string, string) {
	switch strings.ToLower(strings.TrimSpace(c.Type)) {
	case "bearer":
		if c.Token == "" {
			return "", ""
		}
		return "Authorization", "Bearer " + c.Token
	case "basic":
		if c.Username == "" && c.Password == "" {
			return "", ""
		}
		return "Authorization", "Basic " + base64.StdEncoding.EncodeToString([]byte(c.Username+":"+c.Password))
	case "header":
		if c.Header == "" || c.Value == "" {
			return "", ""
		}
		return c.Header, c.Value
	default:
		return "", ""
	}
}

const (
	adapterNotConfiguredCode = "adapter_not_configured"
	adapterUnavailableCode   = "adapter_unavailable"
	adapterCircuitOpenCode   = "circuit_open"
	adapterOKCode            = "ok"
	adapterFailedMessage     = "external adapter failed"
)

type ExternalAdapter struct {
	name            string
	url             string
	client          *http.Client
	maxRetries      int
	baseBackoff     time.Duration
	threshold       int
	openInterval    time.Duration
	stripPrefix     string
	addPrefix       string
	authHeaderName  string
	authHeaderValue string
	mu              sync.Mutex
	failures        int
	circuitOpen     bool
	lastStateMove   time.Time
}

// configure applies per-adapter upstream routing: a path rewrite (strip/add
// prefix) and an injected upstream auth header (finding 8). It is called from the
// composition root after the adapter is constructed.
func (a *ExternalAdapter) configure(cfg AdapterConfig) {
	a.stripPrefix = cfg.StripPrefix
	a.addPrefix = cfg.AddPrefix
	a.authHeaderName, a.authHeaderValue = cfg.Auth.header()
}

// rewritePath maps the inbound request path onto the upstream path by stripping a
// configured prefix and/or adding one, so a platform route can target a differently
// rooted upstream API.
func (a *ExternalAdapter) rewritePath(path string) string {
	if a.stripPrefix != "" && strings.HasPrefix(path, a.stripPrefix) {
		path = strings.TrimPrefix(path, a.stripPrefix)
	}
	if a.addPrefix != "" {
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		path = strings.TrimRight(a.addPrefix, "/") + path
	}
	if path == "" {
		path = "/"
	}
	return path
}

// applyUpstreamAuth injects the configured upstream credential, overwriting any
// client-supplied value on that header so callers cannot spoof upstream auth.
func (a *ExternalAdapter) applyUpstreamAuth(req *http.Request) {
	if a.authHeaderName != "" && a.authHeaderValue != "" {
		req.Header.Set(a.authHeaderName, a.authHeaderValue)
	}
}

func NewExternalAdapter(name, url string, timeout time.Duration, maxRetries, threshold int, openInterval time.Duration) *ExternalAdapter {
	if maxRetries < 1 {
		maxRetries = 1
	}
	if threshold < 1 {
		threshold = 1
	}
	if openInterval <= 0 {
		openInterval = 30 * time.Second
	}
	return &ExternalAdapter{
		name: name,
		url:  url,
		// otelhttp.NewTransport injects W3C trace context into outbound requests
		// and records a client span, so calls to upstreams stay on the same trace.
		client:       &http.Client{Timeout: timeout, Transport: otelhttp.NewTransport(http.DefaultTransport)},
		maxRetries:   maxRetries,
		baseBackoff:  25 * time.Millisecond,
		threshold:    threshold,
		openInterval: openInterval,
	}
}

func (a *ExternalAdapter) Call(ctx context.Context, operation string, idempotent bool) (contracts.AdapterResult, error) {
	if result, blocked := a.preflightResult(operation); blocked {
		return result, nil
	}

	attempts := 1
	if idempotent {
		attempts = a.maxRetries
	}
	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.url, nil)
		if err != nil {
			lastErr = err
			break
		}
		a.applyUpstreamAuth(req)
		resp, err := a.client.Do(req)
		if err == nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		if err == nil && resp.StatusCode < 500 {
			a.recordSuccess()
			return a.successResult(operation, "adapter call succeeded"), nil
		}
		if err != nil {
			lastErr = err
		}
		if err := sleepWithContext(ctx, a.baseBackoff*(1<<attempt)); err != nil {
			lastErr = err
			break
		}
	}
	a.recordFailure()
	return a.degradedResult(operation, adapterUnavailableCode, adapterFailureMessage(lastErr), true), nil
}

func (a *ExternalAdapter) Proxy(ctx context.Context, request contracts.AdapterProxyRequest) (contracts.AdapterProxyResponse, contracts.AdapterResult, error) {
	if result, blocked := a.preflightResult(request.Operation); blocked {
		return contracts.AdapterProxyResponse{}, result, nil
	}
	upstreamURL, err := upstreamProxyURL(a.url, a.rewritePath(request.Path), request.RawQuery)
	if err != nil {
		return contracts.AdapterProxyResponse{}, a.degradedResult(request.Operation, adapterUnavailableCode, err.Error(), false), nil
	}

	attempt := a.proxyWithRetries(ctx, upstreamURL, request)
	if attempt.success {
		a.recordSuccess()
		return attempt.response, a.successResult(request.Operation, "adapter proxy succeeded"), nil
	}
	a.recordFailure()
	if attempt.response.StatusCode != 0 {
		return attempt.response, a.successResult(request.Operation, "upstream response propagated"), nil
	}
	return contracts.AdapterProxyResponse{}, a.degradedResult(request.Operation, adapterUnavailableCode, adapterFailureMessage(attempt.err), true), nil
}

type proxyAttempt struct {
	response contracts.AdapterProxyResponse
	err      error
	success  bool
}

func (a *ExternalAdapter) proxyWithRetries(ctx context.Context, upstreamURL string, request contracts.AdapterProxyRequest) proxyAttempt {
	attempts := proxyAttempts(request.Idempotent, a.maxRetries)
	result := proxyAttempt{}
	for attempt := 0; attempt < attempts; attempt++ {
		response, retryable, err := a.proxyOnce(ctx, upstreamURL, request)
		if err == nil && !retryable {
			return proxyAttempt{response: response, success: true}
		}
		result = proxyAttempt{response: response, err: err}
		if attempt == attempts-1 {
			return result
		}
		if err := sleepWithContext(ctx, a.baseBackoff*(1<<attempt)); err != nil {
			result.err = err
			return result
		}
	}
	return result
}

func proxyAttempts(idempotent bool, maxRetries int) int {
	if idempotent {
		return maxRetries
	}
	return 1
}

func (a *ExternalAdapter) proxyOnce(ctx context.Context, upstreamURL string, request contracts.AdapterProxyRequest) (contracts.AdapterProxyResponse, bool, error) {
	method := request.Method
	if method == "" {
		method = http.MethodGet
	}
	req, err := http.NewRequestWithContext(ctx, method, upstreamURL, bytes.NewReader(request.Body))
	if err != nil {
		return contracts.AdapterProxyResponse{}, false, err
	}
	req.Header = cloneProxyRequestHeader(request.Header)
	a.applyUpstreamAuth(req)
	resp, err := a.client.Do(req)
	if err != nil {
		return contracts.AdapterProxyResponse{}, true, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return contracts.AdapterProxyResponse{}, true, err
	}
	return contracts.AdapterProxyResponse{
		StatusCode: resp.StatusCode,
		Header:     cloneProxyHeader(resp.Header),
		Body:       body,
	}, resp.StatusCode >= 500, nil
}

func (a *ExternalAdapter) preflightResult(operation string) (contracts.AdapterResult, bool) {
	if a.url == "" {
		return a.degradedResult(operation, adapterNotConfiguredCode, "external adapter URL is not configured", true), true
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.circuitOpen && time.Since(a.lastStateMove) < a.openInterval {
		return contracts.AdapterResult{
			Adapter:     a.name,
			Operation:   operation,
			Degraded:    true,
			Retryable:   true,
			Code:        adapterCircuitOpenCode,
			Message:     "external adapter circuit is open",
			CircuitOpen: true,
		}, true
	}
	return contracts.AdapterResult{}, false
}

func (a *ExternalAdapter) successResult(operation, message string) contracts.AdapterResult {
	return contracts.AdapterResult{Adapter: a.name, Operation: operation, Code: adapterOKCode, Message: message}
}

func (a *ExternalAdapter) degradedResult(operation, code, message string, retryable bool) contracts.AdapterResult {
	return contracts.AdapterResult{
		Adapter:   a.name,
		Operation: operation,
		Degraded:  true,
		Retryable: retryable,
		Code:      code,
		Message:   message,
	}
}

func adapterFailureMessage(err error) string {
	if err != nil {
		return err.Error()
	}
	return adapterFailedMessage
}

func upstreamProxyURL(baseURL, requestPath, rawQuery string) (string, error) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	if !base.IsAbs() || base.Host == "" {
		return "", fmt.Errorf("external adapter URL must be absolute")
	}
	base.Path = joinProxyPath(base.Path, requestPath)
	base.RawQuery = rawQuery
	base.Fragment = ""
	return base.String(), nil
}

func joinProxyPath(basePath, requestPath string) string {
	if requestPath == "" {
		requestPath = "/"
	}
	if basePath == "" || basePath == "/" {
		return "/" + strings.TrimLeft(requestPath, "/")
	}
	return strings.TrimRight(basePath, "/") + "/" + strings.TrimLeft(requestPath, "/")
}

func cloneProxyHeader(header http.Header) http.Header {
	cloned := http.Header{}
	connectionHeaders := connectionHeaderTokens(header)
	for key, values := range header {
		if isHopByHopHeader(key) || connectionHeaders[http.CanonicalHeaderKey(key)] {
			continue
		}
		for _, value := range values {
			cloned.Add(key, value)
		}
	}
	return cloned
}

func cloneProxyRequestHeader(header http.Header) http.Header {
	cloned := http.Header{}
	connectionHeaders := connectionHeaderTokens(header)
	for key, values := range header {
		canonicalKey := http.CanonicalHeaderKey(key)
		if isHopByHopHeader(key) || connectionHeaders[canonicalKey] || isSensitiveProxyRequestHeader(key) {
			continue
		}
		for _, value := range values {
			cloned.Add(key, value)
		}
	}
	return cloned
}

func connectionHeaderTokens(header http.Header) map[string]bool {
	tokens := map[string]bool{}
	for _, value := range header.Values("Connection") {
		for _, token := range strings.Split(value, ",") {
			token = strings.TrimSpace(token)
			if token != "" {
				tokens[http.CanonicalHeaderKey(token)] = true
			}
		}
	}
	return tokens
}

func isSensitiveProxyRequestHeader(key string) bool {
	switch http.CanonicalHeaderKey(key) {
	case "Authorization", "Proxy-Authorization", "Cookie":
		return true
	}
	switch normalizedHeaderKey(key) {
	case "apikey", "xapikey", "servicekey", "xservicekey":
		return true
	default:
		return false
	}
}

func normalizedHeaderKey(key string) string {
	key = strings.ToLower(strings.TrimSpace(key))
	key = strings.ReplaceAll(key, "-", "")
	key = strings.ReplaceAll(key, "_", "")
	return key
}

func isHopByHopHeader(key string) bool {
	switch http.CanonicalHeaderKey(key) {
	case "Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization", "Te", "Trailer", "Transfer-Encoding", "Upgrade":
		return true
	default:
		return false
	}
}

// sleepWithContext waits for d or until ctx is cancelled, whichever comes first,
// so retry backoff honors request cancellation instead of blocking blindly.
func sleepWithContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (a *ExternalAdapter) recordSuccess() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.failures = 0
	a.circuitOpen = false
	a.lastStateMove = time.Now().UTC()
}

func (a *ExternalAdapter) recordFailure() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.failures++
	if a.failures >= a.threshold {
		a.circuitOpen = true
		a.lastStateMove = time.Now().UTC()
	}
}
