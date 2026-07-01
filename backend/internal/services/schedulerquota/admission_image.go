package schedulerquota

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

// enforceAdmissionImageAllowList rejects a submit whose workload container images
// are not on the project's published image allow-list. It is in-code
// defense-in-depth, gated by K8S_IMAGE_CHECK_ENABLED (baked into the request as
// EnforceImageAllowList); external policy-engine enforcement is separate. When the
// flag is off the check is skipped, so existing submit behavior is unchanged.
func enforceAdmissionImageAllowList(ctx context.Context, reader admissionReader, req submitAdmissionRequest) error {
	if !req.EnforceImageAllowList {
		return nil
	}
	images := admissionImagesFromRequest(req)
	if len(images) == 0 {
		return nil
	}
	allowed := allowedImageReferences(reader.ListImageAllowRules(ctx), req.ProjectID)
	for _, image := range images {
		if !allowed[normalizeImageRef(image)] {
			return deny(http.StatusForbidden, fmt.Sprintf("image %q is not on the project allow list", image))
		}
	}
	return nil
}

// admissionImagesFromRequest collects every distinct container/init-container image
// across the submitted resources (including Volcano task templates), reusing the
// same PodSpec extraction the resource-limit and runtime-socket guards use.
func admissionImagesFromRequest(req submitAdmissionRequest) []string {
	var images []string
	seen := map[string]bool{}
	for _, resource := range req.Resources {
		obj, ok := admissionResourceRawObject(resource)
		if !ok {
			continue
		}
		appendAdmissionObjectImages(obj, seen, &images)
	}
	return images
}

func appendAdmissionObjectImages(obj map[string]any, seen map[string]bool, images *[]string) {
	for _, podSpec := range admissionResourcePodSpecs(obj) {
		appendPodSpecImages(podSpec, seen, images)
	}
}

func appendPodSpecImages(podSpec map[string]any, seen map[string]bool, images *[]string) {
	for _, key := range []string{"containers", "initContainers"} {
		for _, container := range containersFromSpec(podSpec, key) {
			appendImageRef(strings.TrimSpace(stringField(container, "image")), seen, images)
		}
	}
}

func appendImageRef(image string, seen map[string]bool, images *[]string) {
	if image == "" || seen[image] {
		return
	}
	seen[image] = true
	*images = append(*images, image)
}

// allowedImageReferences builds the set of normalized image references that are
// allow-listed for the project: enabled, non-deleted rules whose project_id matches
// the project or the "*" wildcard.
func allowedImageReferences(rules []admissionRecord, projectID string) map[string]bool {
	set := map[string]bool{}
	for _, rule := range rules {
		data := rule.Data
		if !imageRuleAppliesToProject(data, projectID) {
			continue
		}
		if shared.BoolValue(data, "deleted", "is_deleted", "isDeleted") {
			continue
		}
		if !imageRuleEnabled(data) {
			continue
		}
		for _, ref := range imageRuleReferences(data) {
			set[normalizeImageRef(ref)] = true
		}
	}
	return set
}

func imageRuleAppliesToProject(data map[string]any, projectID string) bool {
	ruleProject := strings.TrimSpace(shared.TextValue(data, "project_id", "projectId"))
	return ruleProject == "*" || ruleProject == strings.TrimSpace(projectID)
}

func imageRuleEnabled(data map[string]any) bool {
	if enabled, ok := data["enabled"].(bool); ok {
		return enabled
	}
	return true
}

func imageRuleReferences(data map[string]any) []string {
	var refs []string
	if ref := strings.TrimSpace(shared.TextValue(data, "image_reference", "imageReference")); ref != "" {
		refs = append(refs, ref)
	}
	repo := strings.TrimSpace(shared.TextValue(data, "repository", "repository_name", "image_name"))
	if repo != "" {
		composed := repo
		if tag := strings.TrimSpace(shared.TextValue(data, "tag", "tag_name")); tag != "" {
			composed = repo + ":" + tag
		}
		if registry := strings.TrimSpace(shared.TextValue(data, "registry")); registry != "" {
			composed = registry + "/" + composed
		}
		refs = append(refs, composed)
	}
	return refs
}

// normalizeImageRef defaults a missing tag to ":latest" so "repo" and "repo:latest"
// compare equal. Digest-pinned refs ("...@sha256:...") are compared verbatim.
// ponytail: exact-reference matching with :latest defaulting; tighten to a full
// OCI reference parser only if registry-host/tag aliasing causes real mismatches.
func normalizeImageRef(ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" || strings.Contains(ref, "@") {
		return ref
	}
	lastSlash := strings.LastIndex(ref, "/")
	lastColon := strings.LastIndex(ref, ":")
	if lastColon <= lastSlash { // no tag (a colon before the last slash is a registry port)
		return ref + ":latest"
	}
	return ref
}
