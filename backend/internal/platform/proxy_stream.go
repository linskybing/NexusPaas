package platform

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
)

type adapterProxyStreamRequest struct {
	Operation string
	Path      string
	RawQuery  string
}

type streamingProxyAdapter interface {
	ProxyStream(http.ResponseWriter, *http.Request, adapterProxyStreamRequest) (int, contracts.AdapterResult, bool, error)
}

func (a *App) maybeHandleStreamingProxy(w http.ResponseWriter, r *httpRequest, route RouteSpec) (int, bool) {
	if !shouldStreamProxy(r.Request, route) {
		return 0, false
	}
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
		WriteDegraded(w, r.Request, http.StatusBadGateway, result, *degradedFromAdapterResult(result))
		return http.StatusBadGateway, true
	}
	adapter := a.Adapters[adapterName]
	streamAdapter, ok := adapter.(streamingProxyAdapter)
	if adapter == nil || !ok {
		result := contracts.AdapterResult{
			Adapter:   adapterName,
			Operation: route.OperationID,
			Degraded:  true,
			Retryable: false,
			Code:      adapterNotConfiguredCode,
			Message:   "external adapter stream proxy is not registered",
		}
		a.Metrics.Inc(adapterName + "_degraded")
		WriteDegraded(w, r.Request, http.StatusOK, result, *degradedFromAdapterResult(result))
		return http.StatusOK, true
	}
	status, result, written, err := streamAdapter.ProxyStream(w, r.Request, adapterProxyStreamRequest{
		Operation: route.OperationID,
		Path:      r.URL.Path,
		RawQuery:  r.URL.RawQuery,
	})
	if status == 0 {
		status = http.StatusBadGateway
	}
	if err != nil && !written {
		result = contracts.AdapterResult{
			Adapter:   adapterName,
			Operation: route.OperationID,
			Degraded:  true,
			Retryable: true,
			Code:      adapterUnavailableCode,
			Message:   err.Error(),
		}
	}
	if result.Degraded && !written {
		a.Metrics.Inc(adapterName + "_degraded")
		WriteDegraded(w, r.Request, status, result, *degradedFromAdapterResult(result))
	}
	return status, true
}

func shouldStreamProxy(r *http.Request, route RouteSpec) bool {
	if route.Action != "proxy" {
		return false
	}
	if isUpgradeRequest(r) {
		return true
	}
	if strings.Contains(strings.ToLower(r.Header.Get("Accept")), "text/event-stream") {
		return true
	}
	query := r.URL.Query()
	for _, key := range []string{"follow", "stream", "watch"} {
		if truthyQuery(query.Get(key)) {
			return true
		}
	}
	operation := strings.ToLower(route.OperationID)
	return strings.HasPrefix(strings.ToLower(r.URL.Path), "/api/v1/ws/") || strings.HasPrefix(operation, "ws_")
}

func isUpgradeRequest(r *http.Request) bool {
	return strings.TrimSpace(r.Header.Get("Upgrade")) != "" &&
		strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade")
}

func truthyQuery(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func (a *ExternalAdapter) ProxyStream(w http.ResponseWriter, r *http.Request, request adapterProxyStreamRequest) (int, contracts.AdapterResult, bool, error) {
	if result, blocked := a.preflightResult(request.Operation); blocked {
		return http.StatusBadGateway, result, false, nil
	}
	upstreamURL, err := upstreamProxyURL(a.url, a.rewritePath(request.Path), request.RawQuery)
	if err != nil {
		return http.StatusBadGateway, a.degradedResult(request.Operation, adapterUnavailableCode, err.Error(), false), false, nil
	}
	target, err := url.Parse(upstreamURL)
	if err != nil {
		return http.StatusBadGateway, a.degradedResult(request.Operation, adapterUnavailableCode, err.Error(), false), false, nil
	}

	status := http.StatusOK
	result := a.successResult(request.Operation, "adapter stream proxy succeeded")
	written := false
	proxy := &httputil.ReverseProxy{
		Director: func(out *http.Request) {
			out.URL.Scheme = target.Scheme
			out.URL.Host = target.Host
			out.URL.Path = target.Path
			out.URL.RawPath = target.RawPath
			out.URL.RawQuery = target.RawQuery
			out.Host = target.Host
			out.Header = cloneStreamProxyHeader(r.Header)
			a.applyUpstreamAuth(out)
		},
		Transport:     a.streamTransport(),
		FlushInterval: -1,
		ModifyResponse: func(resp *http.Response) error {
			status = resp.StatusCode
			written = true
			if resp.StatusCode >= 500 {
				a.recordFailure()
				result = a.successResult(request.Operation, "upstream response propagated")
				return nil
			}
			a.recordSuccess()
			result = a.successResult(request.Operation, "adapter stream proxy succeeded")
			return nil
		},
		ErrorHandler: func(rw http.ResponseWriter, req *http.Request, proxyErr error) {
			status = http.StatusBadGateway
			written = true
			a.recordFailure()
			result = a.degradedResult(request.Operation, adapterUnavailableCode, adapterFailureMessage(proxyErr), true)
			WriteDegraded(rw, req, status, result, *degradedFromAdapterResult(result))
		},
	}
	proxy.ServeHTTP(w, r)
	if !written {
		return status, result, false, fmt.Errorf("stream proxy completed without upstream response")
	}
	return status, result, true, nil
}

func (a *ExternalAdapter) streamTransport() http.RoundTripper {
	if a.client != nil && a.client.Transport != nil {
		return a.client.Transport
	}
	return http.DefaultTransport
}

func cloneStreamProxyHeader(header http.Header) http.Header {
	cloned := cloneProxyRequestHeader(header)
	if upgrade := strings.TrimSpace(header.Get("Upgrade")); upgrade != "" {
		cloned.Set("Upgrade", upgrade)
		cloned.Set("Connection", "Upgrade")
	}
	return cloned
}
