package schedulerquota

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

func resolveAdmissionAcceleratorProfile(ctx context.Context, reader admissionReader, req *submitAdmissionRequest, review *admissionReview) error {
	if req == nil || review == nil {
		return nil
	}
	name := strings.TrimSpace(req.AcceleratorProfile)
	if name == "" {
		return nil
	}
	profile, found := admissionAcceleratorProfileByName(ctx, reader, name)
	if !found {
		return deny(http.StatusUnprocessableEntity, "accelerator profile not found")
	}
	normalized, err := normalizeAcceleratorProfilePayload(shared.CloneMap(profile.Data))
	if err != nil {
		return deny(http.StatusUnprocessableEntity, "accelerator profile is invalid: "+err.Error())
	}
	if !acceleratorProfileEnabled(normalized) {
		return deny(http.StatusUnprocessableEntity, "accelerator profile is disabled")
	}

	review.AcceleratorProfile = shared.FirstNonEmpty(shared.TextValue(normalized, "id"), profile.ID, name)
	review.AcceleratorSelector = admissionStringMap(normalized["node_selector"])
	review.AcceleratorLabels = admissionStringMap(normalized["labels"])
	if err := applyAcceleratorProfileDeviceClass(normalized, req); err != nil {
		return deny(http.StatusUnprocessableEntity, err.Error())
	}
	applyAcceleratorProfileMPSDefaults(normalized, req)
	return nil
}

func admissionAcceleratorProfileByName(ctx context.Context, reader admissionReader, name string) (admissionRecord, bool) {
	if reader == nil || name == "" {
		return admissionRecord{}, false
	}
	for _, record := range reader.ListAcceleratorProfiles(ctx) {
		if record.ID == name || shared.TextValue(record.Data, "id") == name || shared.TextValue(record.Data, "name") == name {
			return record, true
		}
	}
	return admissionRecord{}, false
}

func applyAcceleratorProfileDeviceClass(profile map[string]any, req *submitAdmissionRequest) error {
	allowed := shared.TextValue(profile, "allowed_device_class_name", "allowedDeviceClassName")
	if allowed == "" {
		return nil
	}
	current := strings.TrimSpace(req.DeviceClassName)
	if current == "" {
		req.DeviceClassName = allowed
		return nil
	}
	if !strings.EqualFold(current, allowed) {
		return fmt.Errorf("accelerator profile requires device class %q", allowed)
	}
	req.DeviceClassName = current
	return nil
}

func applyAcceleratorProfileMPSDefaults(profile map[string]any, req *submitAdmissionRequest) {
	if req.SMPercentage == nil {
		if value, found := firstPresent(profile, "default_mps_sm_percentage", "defaultMpsSmPercentage"); found {
			sm := int(getInt64(value, 0))
			if sm > 0 {
				req.SMPercentage = &sm
			}
		}
	}
	if req.PinnedMemoryLimit == nil {
		if pinned := shared.TextValue(profile, "default_pinned_memory_limit", "defaultPinnedMemoryLimit"); pinned != "" {
			req.PinnedMemoryLimit = &pinned
		}
	}
}
