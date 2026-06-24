package platform

import (
	"context"
	"encoding/json"
	"strings"
)

const (
	metricWorkloadQueueJobs               = "nexuspaas_workload_queue_jobs"
	metricImageBuildJobs                  = "nexuspaas_image_build_jobs"
	metricWebRTCActiveSessions            = "nexuspaas_webrtc_active_sessions"
	metricWebRTCEgressBitrateKbps         = "nexuspaas_webrtc_egress_bitrate_kbps"
	MetricConfigFileAdmissionRejections   = "nexuspaas_configfile_admission_rejections_total"
	metricKubernetesApplyFailures         = "nexuspaas_kubernetes_apply_failures_total"
	monitoringWorkloadJobsResource        = "workload-service:jobs"
	monitoringImageBuildJobsResource      = "image-registry-service:image_build_jobs"
	monitoringStatusPending               = "pending"
	monitoringStatusRunning               = "running"
	monitoringStatusPreempted             = "preempted"
	monitoringStatusRejected              = "rejected"
	monitoringApplyInfrastructureReason   = "infrastructure_recovery"
	monitoringApplyPermanentFailureReason = "permanent_apply_failure"
)

var monitoringActiveStreamStatuses = map[string]bool{
	"submitted":     true,
	"waiting_infra": true,
	"queued":        true,
	"running":       true,
}

func (a *App) snapshotMonitoringAcceptanceMetrics() {
	if a == nil || a.Metrics == nil || a.Store == nil {
		return
	}
	if a.canSnapshotMonitoringResource(monitoringWorkloadJobsResource) {
		a.snapshotWorkloadMonitoringMetrics()
	}
	if a.canSnapshotMonitoringResource(monitoringImageBuildJobsResource) {
		a.snapshotImageBuildMonitoringMetrics()
	}
}

func (a *App) canSnapshotMonitoringResource(resource string) bool {
	return a != nil && a.Config.AllowsService(resourceOwner(resource))
}

func (a *App) snapshotWorkloadMonitoringMetrics() {
	queueCounts := map[string]int64{
		monitoringStatusPending:   0,
		monitoringStatusRunning:   0,
		monitoringStatusPreempted: 0,
		monitoringStatusRejected:  0,
	}
	applyFailures := map[string]int64{
		monitoringApplyInfrastructureReason:   0,
		monitoringApplyPermanentFailureReason: 0,
	}
	var activeStreamSessions int64
	var activeStreamBitrateKbps int64

	for _, record := range a.Store.List(context.Background(), monitoringWorkloadJobsResource) {
		status := monitoringTextValue(record.Data, "status", "Status")
		switch status {
		case "submitted", "pending", "queued", "waiting_infra":
			queueCounts[monitoringStatusPending]++
		case "running":
			queueCounts[monitoringStatusRunning]++
		case "preempted":
			queueCounts[monitoringStatusPreempted]++
		case "rejected":
			queueCounts[monitoringStatusRejected]++
		}
		if monitoringActiveStreamStatuses[status] && monitoringBoolValue(record.Data, "streaming_session", "streamingSession", "StreamingSession") {
			activeStreamSessions++
			activeStreamBitrateKbps += int64(monitoringIntValue(record.Data, "stream_max_bitrate_kbps", "streamMaxBitrateKbps", "StreamMaxBitrateKbps"))
		}
		if reason, ok := monitoringKubernetesApplyFailureReason(status, record.Data); ok {
			applyFailures[reason]++
		}
	}

	for status, count := range queueCounts {
		a.Metrics.SetGauge(metricWorkloadQueueJobs, map[string]string{"status": status}, count)
	}
	a.Metrics.SetGauge(metricWebRTCActiveSessions, nil, activeStreamSessions)
	a.Metrics.SetGauge(metricWebRTCEgressBitrateKbps, nil, activeStreamBitrateKbps)
	for reason, count := range applyFailures {
		a.Metrics.SetCounter(metricKubernetesApplyFailures, map[string]string{"reason": reason}, count)
	}
}

func (a *App) snapshotImageBuildMonitoringMetrics() {
	counts := map[string]int64{
		monitoringStatusRunning: 0,
		"failed":                0,
		"succeeded":             0,
		"timeout":               0,
	}
	for _, record := range a.Store.List(context.Background(), monitoringImageBuildJobsResource) {
		switch monitoringTextValue(record.Data, "status", "Status") {
		case "running", "building":
			counts[monitoringStatusRunning]++
		case "failed":
			counts["failed"]++
		case "succeeded", "completed":
			counts["succeeded"]++
		case "timeout", "timed_out":
			counts["timeout"]++
		}
	}
	for status, count := range counts {
		a.Metrics.SetGauge(metricImageBuildJobs, map[string]string{"status": status}, count)
	}
}

func monitoringKubernetesApplyFailureReason(status string, data map[string]any) (string, bool) {
	if status == "waiting_infra" {
		return monitoringApplyInfrastructureReason, true
	}
	if status != "failed" {
		return "", false
	}
	reason := monitoringTextValue(data, "status_reason", "statusReason", "error_message", "errorMessage")
	if reason == "" {
		return "", false
	}
	switch {
	case monitoringContainsAny(reason,
		"infrastructure recovery",
		"cluster client unavailable",
		"create namespace",
		"get namespace",
		"pod api unavailable",
	):
		return monitoringApplyInfrastructureReason, true
	case monitoringContainsAny(reason,
		"invalid kubernetes manifest",
		"unsupported kubernetes manifest kind",
		"namespace is required",
		"no workload resources found",
		"raw kubernetes secret resources are rejected",
		"marshal resource",
	):
		return monitoringApplyPermanentFailureReason, true
	default:
		return "", false
	}
}

func monitoringContainsAny(value string, needles ...string) bool {
	value = strings.ToLower(value)
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func monitoringTextValue(data map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := data[key].(string); ok {
			if trimmed := strings.TrimSpace(value); trimmed != "" {
				return strings.ToLower(trimmed)
			}
		}
	}
	return ""
}

func monitoringIntValue(data map[string]any, keys ...string) int {
	for _, key := range keys {
		switch value := data[key].(type) {
		case int:
			return value
		case int64:
			return int(value)
		case float64:
			return int(value)
		case json.Number:
			if n, err := value.Int64(); err == nil {
				return int(n)
			}
		}
	}
	return 0
}

func monitoringBoolValue(data map[string]any, keys ...string) bool {
	for _, key := range keys {
		switch value := data[key].(type) {
		case bool:
			return value
		case string:
			return strings.EqualFold(strings.TrimSpace(value), "true")
		}
	}
	return false
}
