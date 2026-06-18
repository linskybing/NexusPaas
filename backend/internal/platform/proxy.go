package platform

import (
	"io"
	"net/http"
	"strings"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
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

func shouldPublishRouteAudit(route RouteSpec, status int) bool {
	return route.StateChanging && !(route.Action == "event_ingest" && status >= 400)
}
