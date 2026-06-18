package services

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

type resourceClassification string

const (
	classCohostedOnlyFallback resourceClassification = "cohosted_only_fallback"
)

type serviceResourceKey struct {
	service  string
	resource string
}

var resourceLiteralPattern = regexp.MustCompile(`"([a-z0-9-]+-service:[^"]+)"`)

func TestServiceResourceConstantsAreClassifiedForIsolation(t *testing.T) {
	registered := registeredResourceDependencyKeys()
	explicit := explicitResourceClassifications()
	seenRegistered := map[serviceResourceKey]bool{}

	missing, err := collectServiceResourceClassificationGaps(registered, explicit, seenRegistered)
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) > 0 {
		t.Fatalf("unclassified service resource constants:\n%s", strings.Join(missing, "\n"))
	}

	for key := range registered {
		if !seenRegistered[key] {
			t.Fatalf("registered resource dependency %s -> %s is not backed by a source resource constant", key.service, key.resource)
		}
	}
}

func collectServiceResourceClassificationGaps(
	registered map[serviceResourceKey]bool,
	explicit map[serviceResourceKey]resourceClassification,
	seenRegistered map[serviceResourceKey]bool,
) ([]string, error) {
	var missing []string
	err := filepath.WalkDir(".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !shouldScanSourceFile(path, entry) {
			return nil
		}
		gaps, err := classifySourceResourceLiterals(path, registered, explicit, seenRegistered)
		if err != nil {
			return err
		}
		missing = append(missing, gaps...)
		return nil
	})
	return missing, err
}

func shouldScanSourceFile(path string, entry fs.DirEntry) bool {
	return !entry.IsDir() && strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go")
}

func classifySourceResourceLiterals(
	path string,
	registered map[serviceResourceKey]bool,
	explicit map[serviceResourceKey]resourceClassification,
	seenRegistered map[serviceResourceKey]bool,
) ([]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	owner := serviceOwnerForSource(path)
	var missing []string
	for _, resource := range resourceLiterals(raw) {
		if gap, ok := classifyResourceLiteral(path, owner, resource, registered, explicit, seenRegistered); ok {
			missing = append(missing, gap)
		}
	}
	return missing, nil
}

func resourceLiterals(raw []byte) []string {
	matches := resourceLiteralPattern.FindAllStringSubmatch(string(raw), -1)
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		out = append(out, match[1])
	}
	return out
}

func classifyResourceLiteral(
	path, owner, resource string,
	registered map[serviceResourceKey]bool,
	explicit map[serviceResourceKey]resourceClassification,
	seenRegistered map[serviceResourceKey]bool,
) (string, bool) {
	if resourceOwner(resource) == owner {
		return "", false
	}
	key := serviceResourceKey{service: owner, resource: resource}
	if registered[key] {
		seenRegistered[key] = true
		return "", false
	}
	if explicit[key] == classCohostedOnlyFallback {
		return "", false
	}
	return filepath.ToSlash(path) + ": " + owner + " -> " + resource, true
}

func registeredResourceDependencyKeys() map[serviceResourceKey]bool {
	out := map[serviceResourceKey]bool{}
	for _, dependency := range serviceOwnerReadDependencies() {
		out[serviceResourceKey{service: dependency.service, resource: dependency.resource}] = true
	}
	return out
}

func explicitResourceClassifications() map[serviceResourceKey]resourceClassification {
	return map[serviceResourceKey]resourceClassification{
		{serviceAuditCompliance, serviceOrgProject + ":project_members"}: classCohostedOnlyFallback,

		{serviceAuthorizationPolicy, serviceIdentity + ":roles"}:                  classCohostedOnlyFallback,
		{serviceAuthorizationPolicy, serviceIdentity + ":users"}:                  classCohostedOnlyFallback,
		{serviceAuthorizationPolicy, serviceImageRegistry + ":image_allow_lists"}: classCohostedOnlyFallback,
		{serviceAuthorizationPolicy, serviceOrgProject + ":projects"}:             classCohostedOnlyFallback,
		{serviceAuthorizationPolicy, serviceSchedulerQuota + ":plans"}:            classCohostedOnlyFallback,

		{serviceImageRegistry, serviceIdentity + ":roles"}:                   classCohostedOnlyFallback,
		{serviceImageRegistry, serviceIdentity + ":users"}:                   classCohostedOnlyFallback,
		{serviceImageRegistry, serviceOrgProject + ":project_members"}:       classCohostedOnlyFallback,
		{serviceImageRegistry, serviceOrgProject + ":projects"}:              classCohostedOnlyFallback,
		{serviceImageRegistry, serviceOrgProject + ":user_groups"}:           classCohostedOnlyFallback,
		{serviceIntegrationProxy, serviceIdentity + ":roles"}:                classCohostedOnlyFallback,
		{serviceIntegrationProxy, serviceIdentity + ":users"}:                classCohostedOnlyFallback,
		{serviceOrgProject, serviceIdentity + ":roles"}:                      classCohostedOnlyFallback,
		{serviceOrgProject, serviceIdentity + ":users"}:                      classCohostedOnlyFallback,
		{serviceRequestNotification, serviceOrgProject + ":project_members"}: classCohostedOnlyFallback,
		{serviceRequestNotification, serviceOrgProject + ":projects"}:        classCohostedOnlyFallback,
		{serviceRequestNotification, serviceOrgProject + ":user_groups"}:     classCohostedOnlyFallback,
		{serviceStorage, serviceIdentity + ":roles"}:                         classCohostedOnlyFallback,
		{serviceStorage, serviceIdentity + ":users"}:                         classCohostedOnlyFallback,
		{serviceStorage, serviceOrgProject + ":project_members"}:             classCohostedOnlyFallback,
		{serviceStorage, serviceOrgProject + ":projects"}:                    classCohostedOnlyFallback,
		{serviceStorage, serviceOrgProject + ":user_groups"}:                 classCohostedOnlyFallback,

		{serviceIDE, serviceAuthorizationPolicy + ":roles"}:  classCohostedOnlyFallback,
		{serviceIDE, serviceIdentity + ":roles"}:             classCohostedOnlyFallback,
		{serviceIDE, serviceIdentity + ":users"}:             classCohostedOnlyFallback,
		{serviceIDE, serviceOrgProject + ":project_members"}: classCohostedOnlyFallback,
		{serviceIDE, serviceOrgProject + ":projects"}:        classCohostedOnlyFallback,
		{serviceIDE, serviceOrgProject + ":user_groups"}:     classCohostedOnlyFallback,

		{serviceUsageObservability, serviceAuditCompliance + ":audit_logs"}: classCohostedOnlyFallback,
		{serviceUsageObservability, serviceAuthorizationPolicy + ":roles"}:  classCohostedOnlyFallback,
		{serviceUsageObservability, serviceIdentity + ":roles"}:             classCohostedOnlyFallback,
		{serviceUsageObservability, serviceIdentity + ":users"}:             classCohostedOnlyFallback,
		{serviceUsageObservability, serviceOrgProject + ":project_members"}: classCohostedOnlyFallback,
		{serviceUsageObservability, serviceOrgProject + ":projects"}:        classCohostedOnlyFallback,
		{serviceUsageObservability, serviceOrgProject + ":user_groups"}:     classCohostedOnlyFallback,
		{serviceUsageObservability, serviceRequestNotification + ":forms"}:  classCohostedOnlyFallback,
		{serviceUsageObservability, serviceSchedulerQuota + ":live_quotas"}: classCohostedOnlyFallback,
		{serviceUsageObservability, serviceSchedulerQuota + ":queues"}:      classCohostedOnlyFallback,
		{serviceUsageObservability, serviceWorkload + ":jobs"}:              classCohostedOnlyFallback,
	}
}

func serviceOwnerForSource(path string) string {
	path = filepath.ToSlash(path)
	prefixes := []struct {
		prefix string
		owner  string
	}{
		{"auditcompliance/", serviceAuditCompliance},
		{"authorizationpolicy/", serviceAuthorizationPolicy},
		{"clusterread/", serviceUsageObservability},
		{"dashboard/", serviceUsageObservability},
		{"gpuusage/", serviceUsageObservability},
		{"identity/", serviceIdentity},
		{"ideworkspace/", serviceIDE},
		{"imageregistry/", serviceImageRegistry},
		{"integrationproxy/", serviceIntegrationProxy},
		{"k8scontrol/", serviceK8sControl},
		{"mediaupload/", serviceMediaUpload},
		{"orgproject/", serviceOrgProject},
		{"requestnotification/", serviceRequestNotification},
		{"resourcehours/", serviceUsageObservability},
		{"schedulerquota/", serviceSchedulerQuota},
		{"storage/", serviceStorage},
		{"workload/", serviceWorkload},
	}
	for _, item := range prefixes {
		if strings.HasPrefix(path, item.prefix) {
			return item.owner
		}
	}
	return ""
}

func resourceOwner(resource string) string {
	owner, _, found := strings.Cut(resource, ":")
	if !found {
		return ""
	}
	return owner
}

func TestServiceOwnerReadDependencyResourcesAreUnique(t *testing.T) {
	var keys []string
	seen := map[string]bool{}
	for _, dependency := range serviceOwnerReadDependencies() {
		key := dependency.service + " -> " + dependency.resource
		if seen[key] {
			t.Fatalf("duplicate owner-read dependency %s", key)
		}
		seen[key] = true
		keys = append(keys, key)
	}
	if !slices.IsSorted(keys) {
		t.Fatalf("owner-read dependencies must stay sorted for deterministic startup diagnostics: %v", keys)
	}
}
