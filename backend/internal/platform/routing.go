package platform

import (
	"hash"
	"hash/fnv"
	"net/http"
	"strings"
)

const routeWildcardBucket = "*"

func (a *App) routeCandidates(method, path string) []RouteSpec {
	if a.routeIndex == nil || a.routeIndexHash != routesFingerprint(a.Routes) {
		a.rebuildRouteIndex()
	}
	bucket := routeBucket(path)
	candidates := append([]RouteSpec{}, a.routeIndex[routeIndexKey(method, bucket)]...)
	if bucket != routeWildcardBucket {
		candidates = append(candidates, a.routeIndex[routeIndexKey(method, routeWildcardBucket)]...)
	}
	return candidates
}

func (a *App) catalogRouteCandidates(method, path string) []RouteSpec {
	bucket := routeBucket(path)
	candidates := []RouteSpec{}
	for _, route := range a.CatalogRoutes {
		if route.Method != method {
			continue
		}
		routeBucket := routeBucket(route.Pattern)
		if routeBucket == bucket || routeBucket == routeWildcardBucket {
			candidates = append(candidates, route)
		}
	}
	return candidates
}

func (a *App) rebuildRouteIndex() {
	next := map[string][]RouteSpec{}
	for _, route := range a.Routes {
		key := routeIndexKey(route.Method, routeBucket(route.Pattern))
		next[key] = append(next[key], route)
	}
	a.routeIndex = next
	a.routeIndexHash = routesFingerprint(a.Routes)
}

func routeIndexKey(method, bucket string) string {
	return method + "\x00" + bucket
}

func routeBucket(path string) string {
	parts := pathSegments(path)
	if len(parts) >= 2 && parts[0] == "api" && strings.HasPrefix(strings.ToLower(parts[1]), "v") {
		parts = parts[2:]
	}
	if len(parts) == 0 {
		return ""
	}
	if routeBucketWildcard(parts[0]) {
		return routeWildcardBucket
	}
	return parts[0]
}

func pathSegments(path string) []string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}

func routeBucketWildcard(segment string) bool {
	return strings.HasPrefix(segment, "{") && strings.HasSuffix(segment, "}")
}

func routesFingerprint(routes []RouteSpec) uint64 {
	h := fnv.New64a()
	for _, route := range routes {
		writeRouteFingerprint(h, route.Method)
		writeRouteFingerprint(h, route.Pattern)
		writeRouteFingerprint(h, route.Resource)
		writeRouteFingerprint(h, route.Action)
		writeRouteFingerprint(h, route.IDParam)
		writeRouteFingerprint(h, route.OperationID)
	}
	return h.Sum64()
}

func writeRouteFingerprint(h hash.Hash64, value string) {
	_, _ = h.Write([]byte(value))
	_, _ = h.Write([]byte{0})
}

func pathID(r *http.Request, name string) string {
	if name == "" {
		name = "id"
	}
	if value := r.PathValue(name); value != "" {
		return value
	}
	for _, candidate := range []string{"userId", "podName", "requestId", "buildId", "reservationId", "messageId"} {
		if value := r.PathValue(candidate); value != "" {
			return value
		}
	}
	return ""
}

func operationID(service, method, pattern string) string {
	clean := strings.Trim(pattern, "/")
	clean = strings.ReplaceAll(clean, "/", "_")
	clean = strings.ReplaceAll(clean, "{", "")
	clean = strings.ReplaceAll(clean, "}", "")
	clean = strings.ReplaceAll(clean, "...", "all")
	clean = strings.ReplaceAll(clean, "-", "_")
	return strings.ToLower(method + "_" + service + "_" + clean)
}

func canonicalPattern(pattern string) string {
	parts := strings.Split(strings.Trim(pattern, "/"), "/")
	for i, part := range parts {
		if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "...}") {
			parts[i] = "{...}"
			continue
		}
		if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") {
			parts[i] = "{}"
		}
	}
	return "/" + strings.Join(parts, "/")
}

func extractPathParams(pattern, path string) (map[string]string, bool) {
	params := map[string]string{}
	patternParts := strings.Split(strings.Trim(pattern, "/"), "/")
	pathParts := strings.Split(strings.Trim(path, "/"), "/")
	for i, part := range patternParts {
		if strings.HasSuffix(part, "...}") {
			name := strings.TrimSuffix(strings.TrimPrefix(part, "{"), "...}")
			params[name] = strings.Join(pathParts[i:], "/")
			return params, len(pathParts) >= i
		}
		if i >= len(pathParts) {
			return nil, false
		}
		if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") {
			name := strings.TrimSuffix(strings.TrimPrefix(part, "{"), "}")
			params[name] = pathParts[i]
			continue
		}
		if part != pathParts[i] {
			return nil, false
		}
	}
	return params, len(patternParts) == len(pathParts)
}

func routeSpecificity(pattern string) int {
	score := 0
	for _, part := range strings.Split(strings.Trim(pattern, "/"), "/") {
		switch {
		case strings.HasPrefix(part, "{") && strings.HasSuffix(part, "...}"):
		case strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}"):
			score++
		default:
			score += 2
		}
	}
	return score
}

func routeService(route RouteSpec) string {
	parts := strings.Split(route.Resource, ":")
	if len(parts) > 1 {
		return parts[0]
	}
	return ""
}

func isInternalRoutePattern(pattern string) bool {
	pattern = "/" + strings.TrimLeft(pattern, "/")
	return strings.HasPrefix(pattern, "/internal/") || strings.HasPrefix(pattern, "/api/v1/internal/")
}

func IsInternalRoutePattern(pattern string) bool {
	return isInternalRoutePattern(pattern)
}
