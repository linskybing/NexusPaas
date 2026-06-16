package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

const (
	remotePDPEnforcePath             = "/api/v1/permissions/enforce"
	msgRemotePolicyDecisionFailed    = "remote policy decision failed"
	msgRemotePolicyDecisionInvalid   = "remote policy decision response was invalid"
	msgRemotePolicyDecisionUnsuccess = "remote policy decision was not successful"
)

type RemotePDP struct {
	endpoint string
	apiKey   string
	client   *http.Client
}

func NewRemotePDP(baseURL, apiKey string, timeout time.Duration) RemotePDP {
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	return RemotePDP{
		endpoint: remotePDPEndpoint(baseURL),
		apiKey:   strings.TrimSpace(apiKey),
		client:   &http.Client{Timeout: timeout, Transport: otelhttp.NewTransport(http.DefaultTransport)},
	}
}

func (p RemotePDP) Enforce(ctx context.Context, subject, domain, object, action string) (contracts.Decision, error) {
	body, err := json.Marshal(map[string]string{
		"sub": subject,
		"dom": domain,
		"obj": object,
		"act": action,
	})
	if err != nil {
		return denyDecision("remote policy request could not be encoded"), err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(body))
	if err != nil {
		return denyDecision("remote policy request could not be created"), err
	}
	req.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		req.Header.Set("X-API-Key", p.apiKey)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return denyDecision(msgRemotePolicyDecisionFailed), err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return denyDecision(msgRemotePolicyDecisionUnsuccess), fmt.Errorf("remote policy decision returned HTTP %d", resp.StatusCode)
	}
	decision, err := decodeRemoteDecision(resp.Body)
	if err != nil {
		return denyDecision(msgRemotePolicyDecisionInvalid), err
	}
	if decision.Reason == "" {
		decision.Reason = "remote authorization-policy-service decision"
	}
	return decision, nil
}

func decodeRemoteDecision(body io.Reader) (contracts.Decision, error) {
	raw, err := io.ReadAll(body)
	if err != nil {
		return contracts.Decision{}, err
	}
	var envelope struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err == nil && len(envelope.Data) > 0 {
		if !envelope.Success {
			return contracts.Decision{}, fmt.Errorf("remote policy decision envelope was unsuccessful")
		}
		var decision contracts.Decision
		if err := json.Unmarshal(envelope.Data, &decision); err != nil {
			return contracts.Decision{}, err
		}
		return decision, nil
	}
	var decision contracts.Decision
	if err := json.Unmarshal(raw, &decision); err != nil {
		return contracts.Decision{}, err
	}
	return decision, nil
}

func remotePDPEndpoint(baseURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return strings.TrimSpace(baseURL)
	}
	if parsed.Path == "" || parsed.Path == "/" {
		parsed.Path = remotePDPEnforcePath
	} else if !strings.HasSuffix(strings.TrimRight(parsed.Path, "/"), remotePDPEnforcePath) {
		parsed.Path = joinProxyPath(parsed.Path, remotePDPEnforcePath)
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

func denyDecision(reason string) contracts.Decision {
	return contracts.Decision{Allowed: false, Reason: reason, Version: 1}
}
