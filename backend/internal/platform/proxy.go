package platform

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
)

const (
	gatewayProxyAction       = "gateway_proxy"
	headerForwardedHost      = "X-Forwarded-Host"
	headerForwardedProto     = "X-Forwarded-Proto"
	headerForwardedProtoHTTP = "http"
	headerForwardedProtoTLS  = "https"
)

func (a *App) callAdapter(r *httpRequest, adapterName string, route RouteSpec) contracts.AdapterResult {
	adapter := a.Adapters[adapterName]
	if adapter == nil {
		return contracts.AdapterResult{
			Adapter:   adapterName,
			Operation: route.OperationID,
			Degraded:  true,
			Retryable: false,
			Code:      adapterNotConfiguredCode,
			Message:   "external adapter is not registered",
		}
	}
	result, err := adapter.Call(r.Context(), route.OperationID, route.Method == http.MethodGet)
	if err != nil {
		return contracts.AdapterResult{
			Adapter:   adapterName,
			Operation: route.OperationID,
			Degraded:  true,
			Retryable: true,
			Code:      adapterUnavailableCode,
			Message:   err.Error(),
		}
	}
	return result
}

func degradedFromAdapterResult(result contracts.AdapterResult) *Degraded {
	return &Degraded{
		Adapter:   result.Adapter,
		Code:      result.Code,
		Message:   result.Message,
		Retryable: result.Retryable,
	}
}

func (a *App) handleProxy(r *httpRequest, route RouteSpec) (int, any) {
	adapterName := a.proxyAdapterName(route)
	if adapterName == "" {
		result := contracts.AdapterResult{
			Adapter:   unboundProxyAdapter,
			Operation: route.OperationID,
			Degraded:  true,
			Retryable: false,
			Code:      adapterNotConfiguredCode,
			Message:   "proxy route has no external adapter",
		}
		a.Metrics.Inc(unboundProxyAdapter + "_degraded")
		return http.StatusBadGateway, actionDegradedResponse{data: result, degraded: *degradedFromAdapterResult(result)}
	}
	adapter := a.Adapters[adapterName]
	proxyAdapter, ok := adapter.(contracts.ProxyAdapter)
	if adapter == nil || !ok {
		result := contracts.AdapterResult{
			Adapter:   adapterName,
			Operation: route.OperationID,
			Degraded:  true,
			Retryable: false,
			Code:      adapterNotConfiguredCode,
			Message:   "external adapter proxy is not registered",
		}
		a.Metrics.Inc(adapterName + "_degraded")
		return http.StatusOK, actionDegradedResponse{data: result, degraded: *degradedFromAdapterResult(result)}
	}
	request, err := adapterProxyRequest(r, route)
	if err != nil {
		return http.StatusBadRequest, map[string]any{"error": errInvalidRequestBody, "message": err.Error()}
	}
	upstream, result, err := proxyAdapter.Proxy(r.Context(), request)
	if err != nil {
		result = contracts.AdapterResult{
			Adapter:   adapterName,
			Operation: route.OperationID,
			Degraded:  true,
			Retryable: true,
			Code:      adapterUnavailableCode,
			Message:   err.Error(),
		}
	}
	if result.Degraded {
		a.Metrics.Inc(adapterName + "_degraded")
		return http.StatusOK, actionDegradedResponse{data: result, degraded: *degradedFromAdapterResult(result)}
	}
	status := upstream.StatusCode
	if status == 0 {
		status = http.StatusOK
	}
	return status, rawResponseFromAdapter(upstream)
}

func (a *App) proxyAdapterName(route RouteSpec) string {
	return route.ExternalAdapter
}

func adapterProxyRequest(r *httpRequest, route RouteSpec) (contracts.AdapterProxyRequest, error) {
	var body []byte
	var err error
	if r.Body != nil {
		body, err = io.ReadAll(r.Body)
		if err != nil {
			return contracts.AdapterProxyRequest{}, err
		}
	}
	return contracts.AdapterProxyRequest{
		Operation:  route.OperationID,
		Method:     r.Method,
		Path:       r.URL.Path,
		RawQuery:   r.URL.RawQuery,
		Header:     r.Header.Clone(),
		Body:       body,
		Idempotent: r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions,
	}, nil
}

func rawResponseFromAdapter(response contracts.AdapterProxyResponse) RawResponse {
	headers := http.Header{}
	for key, values := range response.Header {
		if isHopByHopHeader(key) || strings.EqualFold(key, headerContentType) {
			continue
		}
		for _, value := range values {
			headers.Add(key, value)
		}
	}
	return RawResponse{
		ContentType:  response.Header.Get(headerContentType),
		HeaderValues: headers,
		Body:         append([]byte(nil), response.Body...),
	}
}

func (a *App) handleGatewayProxy(r *httpRequest, route RouteSpec) (int, any) {
	owner := routeService(route)
	baseURL := strings.TrimSpace(a.Config.ServiceURLs[owner])
	if baseURL == "" {
		return http.StatusBadGateway, map[string]any{"error": "bad_gateway", "message": "downstream service URL is not configured"}
	}
	endpoint, err := serviceEndpoint(baseURL, r.URL.Path, r.URL.RawQuery)
	if err != nil {
		return http.StatusBadGateway, map[string]any{"error": "bad_gateway", "message": "downstream service URL is invalid"}
	}
	var body []byte
	if r.Body != nil {
		body, err = io.ReadAll(r.Body)
		if err != nil {
			return http.StatusBadRequest, map[string]any{"error": errInvalidRequestBody, "message": err.Error()}
		}
	}
	req, err := http.NewRequestWithContext(r.Context(), r.Method, endpoint, bytes.NewReader(body))
	if err != nil {
		return http.StatusBadGateway, map[string]any{"error": "bad_gateway", "message": "downstream request could not be created"}
	}
	req.Header = gatewayProxyRequestHeader(r.Header)
	if gatewayProxyOIDCBrowserRoute(route, r.URL.Path) {
		setGatewayProxyOIDCForwardedOrigin(req.Header, r.Request)
	}
	timeout := a.Config.AdapterTimeout
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	resp, err := gatewayProxyHTTPClient(timeout, gatewayProxyBrowserRedirectRoute(route, r.URL.Path)).Do(req)
	if err != nil {
		return http.StatusBadGateway, map[string]any{"error": "bad_gateway", "message": "downstream service request failed"}
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return http.StatusBadGateway, map[string]any{"error": "bad_gateway", "message": "downstream response could not be read"}
	}
	return resp.StatusCode, RawResponse{
		ContentType:  resp.Header.Get(headerContentType),
		HeaderValues: cloneProxyHeader(resp.Header),
		Body:         raw,
	}
}

func gatewayProxyRequestHeader(header http.Header) http.Header {
	allowed := map[string]bool{
		"Accept":          true,
		"Authorization":   true,
		"Content-Type":    true,
		"Cookie":          true,
		"Idempotency-Key": true,
		"Traceparent":     true,
		"X-Api-Key":       true,
		"X-Request-Id":    true,
		"X-Trace-Id":      true,
	}
	cloned := http.Header{}
	connectionHeaders := connectionHeaderTokens(header)
	for key, values := range header {
		canonicalKey := http.CanonicalHeaderKey(key)
		if !allowed[canonicalKey] || isHopByHopHeader(key) || connectionHeaders[canonicalKey] {
			continue
		}
		for _, value := range values {
			cloned.Add(canonicalKey, value)
		}
	}
	return cloned
}

func gatewayProxyOIDCBrowserRoute(route RouteSpec, path string) bool {
	return routeService(route) == identityServiceName && strings.HasPrefix(path, "/api/v1/oidc/")
}

func gatewayProxyBrowserRedirectRoute(route RouteSpec, path string) bool {
	if routeService(route) != identityServiceName {
		return false
	}
	return strings.HasPrefix(path, "/api/v1/oidc/") || strings.HasPrefix(path, "/dex/")
}

func gatewayProxyHTTPClient(timeout time.Duration, preserveRedirects bool) *http.Client {
	client := &http.Client{Timeout: timeout}
	if preserveRedirects {
		client.CheckRedirect = func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}
	return client
}

func setGatewayProxyOIDCForwardedOrigin(header http.Header, r *http.Request) {
	if host, ok := gatewayValidHostAuthority(r.Host); ok {
		header.Set(headerForwardedHost, host)
	}
	if proto, ok := gatewayValidForwardedProto(r.Header.Values(headerForwardedProto)); ok {
		header.Set(headerForwardedProto, proto)
		return
	}
	if r.TLS != nil {
		header.Set(headerForwardedProto, headerForwardedProtoTLS)
		return
	}
	header.Set(headerForwardedProto, headerForwardedProtoHTTP)
}

func gatewayValidForwardedProto(values []string) (string, bool) {
	if len(values) != 1 {
		return "", false
	}
	value := strings.ToLower(strings.TrimSpace(values[0]))
	if strings.Contains(value, ",") || (value != headerForwardedProtoHTTP && value != headerForwardedProtoTLS) {
		return "", false
	}
	return value, true
}

func gatewayValidHostAuthority(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" || strings.Contains(value, ",") || strings.ContainsAny(value, " \t\r\n/\\@") {
		return "", false
	}
	host := value
	if splitHost, port, err := net.SplitHostPort(value); err == nil {
		if !gatewayValidPort(port) {
			return "", false
		}
		host = splitHost
	} else if strings.Contains(value, ":") {
		if !strings.HasPrefix(value, "[") || !strings.HasSuffix(value, "]") {
			return "", false
		}
		host = strings.TrimSuffix(strings.TrimPrefix(value, "["), "]")
	}
	host = strings.Trim(host, "[]")
	if host == "" || strings.Contains(host, ",") || strings.ContainsAny(host, " \t\r\n/\\@") {
		return "", false
	}
	return value, true
}

func gatewayValidPort(port string) bool {
	if port == "" {
		return false
	}
	value, err := strconv.Atoi(port)
	return err == nil && value > 0 && value <= 65535
}

func shouldPublishRouteAudit(route RouteSpec, status int) bool {
	return route.StateChanging && !(route.Action == "event_ingest" && status >= 400)
}
