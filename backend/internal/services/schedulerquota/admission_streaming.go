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

func enforceAdmissionStreaming(req submitAdmissionRequest, usage admissionUsage) error {
	if !req.StreamingSession {
		return nil
	}
	requested := streamRequestBitrate(req)
	if requested > streamBitrateCap(req) {
		return deny(http.StatusConflict, fmt.Sprintf("stream bitrate cap exceeded: requested %d Kbps, limit %d Kbps", requested, streamBitrateCap(req)))
	}
	if usage.ActiveStreamSessions >= streamSessionCap(req) {
		return deny(http.StatusConflict, fmt.Sprintf("stream session cap exceeded: using %d, limit %d", usage.ActiveStreamSessions, streamSessionCap(req)))
	}
	if usage.ActiveStreamBitrateKbps+requested > streamEgressBudget(req) {
		return deny(http.StatusConflict, fmt.Sprintf("stream egress budget exceeded: using %d Kbps, requested %d Kbps, limit %d Kbps", usage.ActiveStreamBitrateKbps, requested, streamEgressBudget(req)))
	}
	return nil
}

func admissionJobStreamingSession(data map[string]any) bool {
	return shared.BoolValue(data, "streaming_session", "streamingSession", "StreamingSession")
}

func admissionJobStreamBitrate(data map[string]any) int {
	return firstPositive(shared.IntValue(data, "stream_max_bitrate_kbps", "streamMaxBitrateKbps", "StreamMaxBitrateKbps"), defaultStreamMaxBitrateKbps)
}

func streamRequestBitrate(req submitAdmissionRequest) int {
	return firstPositive(req.StreamMaxBitrateKbps, defaultStreamMaxBitrateKbps)
}

func streamBitrateCap(req submitAdmissionRequest) int {
	return firstPositive(req.StreamBitrateCapKbps, defaultStreamMaxBitrateKbps)
}

func streamSessionCap(req submitAdmissionRequest) int {
	return firstPositive(req.StreamSessionCap, defaultStreamMaxConcurrentSessions)
}

func streamEgressBudget(req submitAdmissionRequest) int {
	return firstPositive(req.StreamEgressBudgetKbps, defaultStreamEgressBudgetKbps)
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
