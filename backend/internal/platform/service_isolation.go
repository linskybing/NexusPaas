package platform

import (
	"fmt"
	"sort"
	"strings"
)

// RegisterStoreDependencies records in-process store resources that a service
// accesses outside its owned catalog. Until those accesses are replaced by
// service APIs, owner commands, or event-fed read models, production startup
// can fail loudly instead of returning silently empty data or writing
// split-brain records in isolated deployments.
func (a *App) RegisterStoreDependencies(service string, resources ...string) {
	service = strings.TrimSpace(service)
	if service == "" {
		return
	}
	if a.storeDependencies[service] == nil {
		a.storeDependencies[service] = map[string]bool{}
	}
	for _, resource := range resources {
		resource = strings.TrimSpace(resource)
		if resource != "" {
			a.storeDependencies[service][resource] = true
		}
	}
}

// ValidateServiceIsolation reports direct generic store access that crosses
// service ownership boundaries when the current process hosts only one service.
// It is intentionally silent for SERVICE_NAME=all, where all owners are
// co-hosted.
func (a *App) ValidateServiceIsolation() error {
	gaps := a.serviceIsolationGaps()
	if len(gaps) == 0 {
		return nil
	}
	return fmt.Errorf("external in-process store dependencies for isolated service: %s", strings.Join(gaps, ", "))
}

func (a *App) serviceIsolationGaps() []string {
	if a.Config.ServiceName == "" || a.Config.ServiceName == "all" {
		return nil
	}
	var gaps []string
	for service, resources := range a.storeDependencies {
		if !a.Config.AllowsService(service) {
			continue
		}
		for resource := range resources {
			if ownsResource(service, resource) || a.Config.AllowsService(resourceOwner(resource)) {
				continue
			}
			// A configured domain read contract plus service authentication means
			// the read is resolved through an owner API (finding 5). Generic
			// internal-records fallback is intentionally not accepted here.
			if hasDomainReadContract(resource) && a.Config.ServiceURLs[resourceOwner(resource)] != "" && a.Config.ServiceAPIKey != "" {
				continue
			}
			gaps = append(gaps, service+" -> "+resource)
		}
	}
	sort.Strings(gaps)
	return gaps
}

func ownsResource(service, resource string) bool {
	owner := resourceOwner(resource)
	return owner == "" || owner == service
}

func hasDomainReadContract(resource string) bool {
	_, ok := domainReadContracts[resource]
	return ok
}

func resourceOwner(resource string) string {
	owner, _, found := strings.Cut(resource, ":")
	if !found {
		return ""
	}
	return owner
}
