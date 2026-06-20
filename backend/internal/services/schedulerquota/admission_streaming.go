package schedulerquota

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

const (
	defaultStreamMaxBitrateKbps        = 12000
	defaultStreamMaxConcurrentSessions = 64
	defaultStreamEgressBudgetKbps      = 800000
)

func applyAdmissionStreamConfig(req *submitAdmissionRequest, cfg platform.Config) {
	if req == nil {
		return
	}
	if req.StreamMaxBitrateKbps <= 0 {
		req.StreamMaxBitrateKbps = firstPositive(cfg.StreamMaxBitrateKbps, defaultStreamMaxBitrateKbps)
	}
	req.StreamBitrateCapKbps = firstPositive(cfg.StreamMaxBitrateKbps, defaultStreamMaxBitrateKbps)
	req.StreamSessionCap = firstPositive(cfg.StreamMaxConcurrentSessions, defaultStreamMaxConcurrentSessions)
	req.StreamEgressBudgetKbps = firstPositive(cfg.StreamEgressBudgetKbps, defaultStreamEgressBudgetKbps)
}

// enforceAdmissionStreaming reads the req.Stream* caps directly: applyAdmissionStreamConfig
// always runs first (admission.go) and guarantees they are positive.
func enforceAdmissionStreaming(req submitAdmissionRequest, usage admissionUsage) error {
	if !req.StreamingSession {
		return nil
	}
	requested := req.StreamMaxBitrateKbps
	if requested > req.StreamBitrateCapKbps {
		return deny(http.StatusConflict, fmt.Sprintf("stream bitrate cap exceeded: requested %d Kbps, limit %d Kbps", requested, req.StreamBitrateCapKbps))
	}
	if usage.ActiveStreamSessions >= req.StreamSessionCap {
		return deny(http.StatusConflict, fmt.Sprintf("stream session cap exceeded: using %d, limit %d", usage.ActiveStreamSessions, req.StreamSessionCap))
	}
	if usage.ActiveStreamBitrateKbps+requested > req.StreamEgressBudgetKbps {
		return deny(http.StatusConflict, fmt.Sprintf("stream egress budget exceeded: using %d Kbps, requested %d Kbps, limit %d Kbps", usage.ActiveStreamBitrateKbps, requested, req.StreamEgressBudgetKbps))
	}
	return nil
}

func admissionJobStreamingSession(data map[string]any) bool {
	return shared.BoolValue(data, "streaming_session", "streamingSession", "StreamingSession")
}

func admissionJobStreamBitrate(data map[string]any) int {
	return firstPositive(shared.IntValue(data, "stream_max_bitrate_kbps", "streamMaxBitrateKbps", "StreamMaxBitrateKbps"), defaultStreamMaxBitrateKbps)
}

func activeAdmissionStatus(status string) bool {
	return activeAdmissionStatuses[strings.ToLower(strings.TrimSpace(status))]
}

func firstPositive(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}
