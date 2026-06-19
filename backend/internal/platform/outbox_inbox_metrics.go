package platform

const (
	metricEventOutboxEvents       = "nexuspaas_event_outbox_events"
	metricEventConsumerLag        = "nexuspaas_event_consumer_lag"
	metricProjectionApplied       = "nexuspaas_projection_applied_total"
	metricProjectionDeadLetters   = "nexuspaas_projection_dead_letters_total"
	metricProjectionRetries       = "nexuspaas_projection_retry_total"
	metricProjectionReplays       = "nexuspaas_projection_replay_total"
	metricLabelProjectionConsumer = "consumer"
)

func (a *App) snapshotOutboxInboxMetrics() {
	if a == nil || a.Metrics == nil || a.Events == nil {
		return
	}
	a.Metrics.SetGauge(metricEventOutboxEvents, nil, int64(len(a.Events.Outbox())))
	for _, status := range a.ProjectionStatuses() {
		labels := map[string]string{metricLabelProjectionConsumer: status.Consumer}
		a.Metrics.SetGauge(metricEventConsumerLag, labels, int64(status.Lag))
		a.Metrics.SetCounter(metricProjectionApplied, labels, status.Applied)
		a.Metrics.SetCounter(metricProjectionDeadLetters, labels, status.DeadLettered)
		a.Metrics.SetCounter(metricProjectionRetries, labels, status.RetryCount)
		a.Metrics.SetCounter(metricProjectionReplays, labels, status.ReplayCount)
	}
}
