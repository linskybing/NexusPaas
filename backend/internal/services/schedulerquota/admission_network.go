package schedulerquota

import (
	"context"
	"net/http"
	"strings"

	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

const multusNetworksAnnotation = "k8s.v1.cni.cncf.io/networks"

func resolveAdmissionNetworkProfile(ctx context.Context, reader admissionReader, req submitAdmissionRequest, review *admissionReview) error {
	if review == nil {
		return nil
	}
	review.RDMARequired = req.RDMARequired
	review.NICClass = strings.TrimSpace(req.NICClass)
	review.TopologyRequirement = strings.TrimSpace(req.TopologyRequirement)

	name := strings.TrimSpace(req.NetworkProfile)
	if name == "" {
		return nil
	}
	profile, found := admissionNetworkProfileByName(ctx, reader, name)
	if !found {
		return deny(http.StatusUnprocessableEntity, "network profile not found")
	}
	if !admissionNetworkProfileEnabled(profile.Data) {
		return deny(http.StatusUnprocessableEntity, "network profile is disabled")
	}

	review.NetworkProfile = shared.FirstNonEmpty(shared.TextValue(profile.Data, "id"), profile.ID, name)
	review.RDMARequired = req.RDMARequired || shared.BoolValue(profile.Data, "rdma_enabled", "rdmaEnabled")
	review.NICClass = shared.FirstNonEmpty(
		strings.TrimSpace(req.NICClass),
		shared.TextValue(profile.Data, "required_nic_class", "requiredNicClass", "nic_class", "nicClass"),
	)
	review.TopologyRequirement = shared.FirstNonEmpty(
		strings.TrimSpace(req.TopologyRequirement),
		shared.TextValue(profile.Data, "topology_requirement", "topologyRequirement", "topology_policy", "topologyPolicy"),
	)
	review.NetworkAnnotations = admissionStringMap(profile.Data["annotations"])
	if secondary := shared.TextValue(profile.Data, "secondary_network", "secondaryNetwork"); secondary != "" && secondary != "none" {
		if review.NetworkAnnotations == nil {
			review.NetworkAnnotations = map[string]any{}
		}
		if shared.TextValue(review.NetworkAnnotations, multusNetworksAnnotation) == "" {
			review.NetworkAnnotations[multusNetworksAnnotation] = secondary
		}
	}
	review.NetworkEnv = admissionStringMap(profile.Data["network_env"])
	return nil
}

func admissionNetworkProfileByName(ctx context.Context, reader admissionReader, name string) (admissionRecord, bool) {
	if reader == nil || name == "" {
		return admissionRecord{}, false
	}
	for _, record := range reader.ListNetworkProfiles(ctx) {
		if record.ID == name || shared.TextValue(record.Data, "id") == name || shared.TextValue(record.Data, "name") == name {
			return record, true
		}
	}
	return admissionRecord{}, false
}

func admissionNetworkProfileEnabled(data map[string]any) bool {
	value, found := firstPresent(data, "enabled", "Enabled")
	if !found {
		return true
	}
	enabled, ok := value.(bool)
	return !ok || enabled
}

func admissionStringMap(raw any) map[string]any {
	data, ok := raw.(map[string]any)
	if !ok || len(data) == 0 {
		return nil
	}
	out := map[string]any{}
	for key, value := range data {
		text, ok := value.(string)
		if !ok || strings.TrimSpace(key) == "" || strings.TrimSpace(text) == "" {
			continue
		}
		out[key] = text
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
