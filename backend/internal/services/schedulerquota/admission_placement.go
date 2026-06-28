package schedulerquota

import (
	"context"
	"net/http"
	"strings"

	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

func resolveAdmissionPlacementProfile(ctx context.Context, reader admissionReader, req submitAdmissionRequest, queueName string, review *admissionReview) error {
	if review == nil {
		return nil
	}
	name := strings.TrimSpace(req.PlacementProfile)
	if name == "" {
		return nil
	}
	profile, found := admissionPlacementProfileByName(ctx, reader, name)
	if !found {
		return deny(http.StatusUnprocessableEntity, "placement profile not found")
	}
	if !admissionPlacementProfileEnabled(profile.Data) {
		return deny(http.StatusUnprocessableEntity, "placement profile is disabled")
	}

	review.PlacementProfile = shared.FirstNonEmpty(shared.TextValue(profile.Data, "id"), profile.ID, name)
	review.SchedulerBackend = shared.TextValue(profile.Data, "scheduler_backend", "schedulerBackend")
	review.SchedulerName = shared.TextValue(profile.Data, "scheduler_name", "schedulerName")
	review.GangEnabled = shared.BoolValue(profile.Data, "gang_enabled", "gangEnabled")
	review.GangMinAvailable = shared.IntValue(profile.Data, "gang_min_available", "gangMinAvailable")
	review.PlacementLabels = admissionStringMap(firstValue(profile.Data, "placement_labels", "placementLabels", "labels"))
	review.PlacementAnnotations = admissionStringMap(firstValue(profile.Data, "placement_annotations", "placementAnnotations", "annotations"))
	if strings.EqualFold(review.SchedulerBackend, "kueue") {
		key := shared.TextValue(profile.Data, "queue_label_key", "queueLabelKey")
		if key != "" && strings.TrimSpace(queueName) != "" {
			if review.PlacementLabels == nil {
				review.PlacementLabels = map[string]any{}
			}
			review.PlacementLabels[key] = strings.TrimSpace(queueName)
		}
	}
	if review.SchedulerName == "" {
		review.SchedulerName = defaultSchedulerNameForBackend(review.SchedulerBackend)
	}
	return nil
}

func admissionPlacementProfileByName(ctx context.Context, reader admissionReader, name string) (admissionRecord, bool) {
	if reader == nil || name == "" {
		return admissionRecord{}, false
	}
	for _, record := range reader.ListPlacementProfiles(ctx) {
		if record.ID == name || shared.TextValue(record.Data, "id") == name || shared.TextValue(record.Data, "name") == name {
			return record, true
		}
	}
	return admissionRecord{}, false
}

func admissionPlacementProfileEnabled(data map[string]any) bool {
	value, found := firstPresent(data, "enabled", "Enabled")
	if !found {
		return true
	}
	enabled, ok := value.(bool)
	return !ok || enabled
}

func defaultSchedulerNameForBackend(backend string) string {
	if strings.EqualFold(backend, "volcano") {
		return placementVolcanoSchedulerName
	}
	return placementDefaultSchedulerName
}
