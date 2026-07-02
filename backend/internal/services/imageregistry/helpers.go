package imageregistry

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

type imageRecord struct {
	ID   string
	Data map[string]any
}

var errImageProjectionDriftUnavailable = errors.New("image registry projection drift unavailable")

type imageProjectionDriftReport struct {
	Missing []imageProjectionDriftFinding
	Orphan  []imageProjectionDriftFinding
	Stale   []imageProjectionDriftFinding
}

type imageProjectionDriftFinding struct {
	SourceResource string
	LocalResource  string
	ID             string
}

type imageProjectionDriftPair struct {
	sourceResource string
	localResource  string
	idFn           func(map[string]any) string
}

var imageProjectionDriftPairs = []imageProjectionDriftPair{
	{sourceResource: identityUsersResource, localResource: imageIdentityUsersResource, idFn: func(row map[string]any) string {
		return imageReadModelID(imageIdentityUsersResource, row)
	}},
	{sourceResource: identityRolesResource, localResource: imageIdentityRolesResource, idFn: func(row map[string]any) string {
		return imageReadModelID(imageIdentityRolesResource, row)
	}},
	{sourceResource: orgProjectsResource, localResource: imageProjectsResource, idFn: func(row map[string]any) string {
		return imageReadModelID(imageProjectsResource, row)
	}},
	{sourceResource: orgProjectMembersResource, localResource: imageProjectMembersResource, idFn: func(row map[string]any) string {
		return imageReadModelID(imageProjectMembersResource, row)
	}},
	{sourceResource: orgUserGroupsResource, localResource: imageUserGroupsResource, idFn: func(row map[string]any) string {
		return imageReadModelID(imageUserGroupsResource, row)
	}},
}

func registryEvent(r *http.Request, name string, data map[string]any) contracts.Event {
	return contracts.Event{
		EventID:        platform.NewUUID(),
		Name:           name,
		Source:         serviceName,
		OccurredAt:     time.Now().UTC(),
		TraceID:        shared.FirstNonBlank(r.Header.Get("X-Trace-ID"), r.Header.Get("X-Request-ID"), platform.NewUUID()),
		SchemaVersion:  1,
		IdempotencyKey: strings.TrimSpace(r.Header.Get(idempotencyKeyHeader)),
		Data:           sanitizeImageEventData(data),
	}
}

func publishEvent(app *platform.App, r *http.Request, name string, data map[string]any) {
	if err := app.Events.Publish(r.Context(), registryEvent(r, name, data)); err != nil {
		slogError(name, err)
	}
}

func slogError(event string, err error) {
	if err != nil {
		slog.Error("image registry event publish failed", "event", event, "error", err)
	}
}

func callHarbor(app *platform.App, r *http.Request, route platform.RouteSpec, fallbackOperation string) (contracts.AdapterResult, *platform.Degraded) {
	operation := shared.FirstNonBlank(route.OperationID, fallbackOperation)
	adapter := app.Adapters["harbor"]
	if adapter == nil {
		result := contracts.AdapterResult{Adapter: "harbor", Operation: operation, Degraded: true, Code: "adapter_not_configured", Message: "external adapter is not registered", Retryable: false}
		return result, adapterDegraded(result)
	}
	result, err := adapter.Call(r.Context(), operation, route.Method == http.MethodGet)
	if err != nil {
		result = contracts.AdapterResult{Adapter: "harbor", Operation: operation, Degraded: true, Code: "adapter_unavailable", Message: err.Error(), Retryable: true}
	}
	if result.Degraded {
		app.Metrics.Inc("harbor_degraded")
		return result, adapterDegraded(result)
	}
	return result, nil
}

func harborDegraded(app *platform.App, r *http.Request, route platform.RouteSpec, fallbackOperation string) *platform.Degraded {
	_, degraded := callHarbor(app, r, route, fallbackOperation)
	return degraded
}

func adapterDegraded(result contracts.AdapterResult) *platform.Degraded {
	return &platform.Degraded{Adapter: result.Adapter, Code: result.Code, Message: result.Message, Retryable: result.Retryable}
}

func imageRows(app *platform.App, r *http.Request, resource string) []map[string]any {
	syncImageReadModels(app, r)
	records := app.Store.List(r.Context(), resource)
	rows := make([]map[string]any, 0, len(records))
	for _, record := range records {
		row := shared.CloneMap(record.Data)
		if row["id"] == nil {
			row["id"] = record.ID
		}
		if resource == imageBuildsResource {
			row = publicImageBuildData(row)
		}
		rows = append(rows, row)
	}
	return rows
}

func sanitizeImageEventData(data map[string]any) map[string]any {
	out := shared.CloneMap(data)
	delete(out, internalImageBuildIdempotencyKeyHash)
	delete(out, internalImageBuildIdempotencyFingerprintHash)
	delete(out, internalImageBuildCancelIdempotencyKeyHash)
	delete(out, internalImageBuildCancelIdempotencyFingerprintHash)
	return out
}

func imageAccessRows(app *platform.App, r *http.Request, localResource, sourceResource string) []map[string]any {
	local := imageRows(app, r, localResource)
	if !sourceCoHosted(app, sourceResource) {
		return local
	}
	source := imageRowsWithoutSync(app, r, sourceResource)
	if len(local) == 0 {
		return source
	}
	return mergeRows(localResource, source, local)
}

func imageRowsWithoutSync(app *platform.App, r *http.Request, resource string) []map[string]any {
	records := app.Store.List(r.Context(), resource)
	rows := make([]map[string]any, 0, len(records))
	for _, record := range records {
		row := shared.CloneMap(record.Data)
		if row["id"] == nil {
			row["id"] = record.ID
		}
		rows = append(rows, row)
	}
	return rows
}

func imageCatalogRows(app *platform.App, r *http.Request) []map[string]any {
	return imageRows(app, r, imageCatalogResource)
}

func requireUser(r *http.Request) (string, int, any, bool) {
	userID := strings.TrimSpace(r.Header.Get("X-User-ID"))
	if userID == "" {
		return "", http.StatusUnauthorized, shared.ErrorData("unauthorized"), false
	}
	return userID, 0, nil, true
}

func requireProjectRead(app *platform.App, r *http.Request, projectID, userID string) (map[string]any, int, any, bool) {
	project, found := findProject(app, r, projectID)
	if !found {
		return nil, http.StatusNotFound, shared.ErrorData("Project not found"), false
	}
	if !canReadProject(projectRole(app, r, project, userID)) {
		return nil, http.StatusForbidden, shared.ErrorData(msgProjectMemberAccess), false
	}
	return project, 0, nil, true
}

func requireProjectManager(app *platform.App, r *http.Request, projectID, userID string) (map[string]any, int, any, bool) {
	project, found := findProject(app, r, projectID)
	if !found {
		return nil, http.StatusNotFound, shared.ErrorData("Project not found"), false
	}
	if !canManageProject(projectRole(app, r, project, userID)) {
		return nil, http.StatusForbidden, shared.ErrorData(msgProjectManagerAccess), false
	}
	return project, 0, nil, true
}

func findProject(app *platform.App, r *http.Request, projectID string) (map[string]any, bool) {
	for _, project := range imageAccessRows(app, r, imageProjectsResource, orgProjectsResource) {
		if idFrom(project, "id", "p_id", "PID", "project_id", "projectId") == projectID {
			return normalizeProject(project), true
		}
	}
	return nil, false
}

func normalizeProject(project map[string]any) map[string]any {
	out := shared.CloneMap(project)
	id := idFrom(out, "id", "p_id", "PID", "project_id", "projectId")
	ownerID := idFrom(out, "owner_id", "ownerId", "GID", "g_id", "gid")
	if id != "" {
		out["id"] = id
		out["project_id"] = id
	}
	if ownerID != "" {
		out["owner_id"] = ownerID
	}
	return out
}

func projectRole(app *platform.App, r *http.Request, project map[string]any, userID string) string {
	if hasAdminPanel(app, r, userID) {
		return "admin"
	}
	if idFrom(project, "personal_user_id", "personalUserID") == userID {
		return "admin"
	}
	if ownerID := idFrom(project, "owner_id", "ownerId", "GID", "g_id"); ownerID != "" {
		if membership, found := findGroupMembership(app, r, ownerID, userID); found {
			return normalizeRole(shared.TextValue(membership, "role"))
		}
	}
	if member, found := findProjectMember(app, r, idFrom(project, "project_id", "id"), userID); found {
		return normalizeRole(shared.TextValue(member, "role"))
	}
	return ""
}

func findProjectMember(app *platform.App, r *http.Request, projectID, userID string) (map[string]any, bool) {
	for _, member := range imageAccessRows(app, r, imageProjectMembersResource, orgProjectMembersResource) {
		if idFrom(member, "project_id", "projectId") == projectID && idFrom(member, "user_id", "userId") == userID {
			return member, true
		}
	}
	return nil, false
}

func findGroupMembership(app *platform.App, r *http.Request, groupID, userID string) (map[string]any, bool) {
	for _, member := range imageAccessRows(app, r, imageUserGroupsResource, orgUserGroupsResource) {
		if idFrom(member, "group_id", "groupId", "gid", "g_id") == groupID && idFrom(member, "user_id", "userId", "uid", "u_id") == userID {
			return member, true
		}
	}
	return nil, false
}

func hasAdminPanel(app *platform.App, r *http.Request, userID string) bool {
	if strings.EqualFold(r.Header.Get("X-User-Role"), "admin") {
		return true
	}
	roles := imageAccessRows(app, r, imageIdentityRolesResource, identityRolesResource)
	for _, user := range imageAccessRows(app, r, imageIdentityUsersResource, identityUsersResource) {
		if idFrom(user, "id", "user_id", "userId") != userID {
			continue
		}
		if recordGrantsAdmin(user) {
			return true
		}
		roleID := idFrom(user, "role_id", "roleId", "role", "Role")
		for _, role := range roles {
			if (idFrom(role, "id", "name") == roleID) && recordGrantsAdmin(role) {
				return true
			}
		}
	}
	return false
}

func recordGrantsAdmin(data map[string]any) bool {
	if shared.BoolValue(data, "admin_panel", "adminPanel", "AdminPanel") {
		return true
	}
	return shared.BoolValue(shared.MapValue(data, "capabilities", "Capabilities"), "admin_panel", "adminPanel", "AdminPanel")
}

func canReadProject(role string) bool {
	return role == "admin" || role == "manager" || role == "user"
}

func canManageProject(role string) bool {
	return role == "admin" || role == "manager"
}

func ruleEnabled(rule map[string]any) bool {
	if enabled, ok := rule["enabled"].(bool); ok {
		return enabled
	}
	return true
}

func allowRuleFromCatalog(app *platform.App, r *http.Request, tagID, projectID, userID string) map[string]any {
	tag := catalogByID(app, r, tagID)
	repo := shared.FirstNonBlank(shared.TextValue(tag, "repository", "repository_name", "image_name"), "unknown")
	tagName := shared.FirstNonBlank(shared.TextValue(tag, "tag", "tag_name"), defaultTag)
	now := time.Now().UTC()
	rule := map[string]any{
		"id":              ruleID(projectID, tagID),
		"project_id":      projectID,
		"tag_id":          tagID,
		"repository":      repo,
		"tag":             tagName,
		"image_reference": imageRefFromParts(shared.TextValue(tag, "registry"), repo, tagName),
		"enabled":         true,
		"created_by":      userID,
		"created_at":      now,
		"updated_at":      now,
	}
	promoteCatalogImageStatusFields(rule, tag)
	return rule
}

func catalogAllowListRejection(tag map[string]any, requireProvenance bool) string {
	if len(tag) == 0 {
		return "catalog image not found"
	}
	if shared.BoolValue(tag, "deleted", "is_deleted", "isDeleted") {
		return "catalog image is deleted"
	}
	if shared.BoolValue(tag, "unavailable", "is_unavailable", "isUnavailable") {
		return "catalog image is unavailable"
	}
	if shared.TextValue(tag, "digest", "image_digest", "imageDigest") == "" {
		return "catalog image digest is required before publish"
	}
	if !catalogScanPassed(shared.TextValue(tag, "scan_status", "scanStatus")) {
		return "catalog image scan must pass before publish"
	}
	// Supply-chain enforcement is opt-in (IMAGE_PUBLISH_REQUIRE_PROVENANCE): when on,
	// the catalog image must carry SBOM + signature metadata (presence), and the
	// caller (publishCatalog) additionally requires a verified build-pipeline
	// record for the digest via buildProvenanceRejection — provenance fields are
	// only trusted when the dispatcher pipeline wrote them.
	if requireProvenance {
		if shared.TextValue(tag, "sbom_digest", "sbomDigest", "sbom_ref", "sbom") == "" {
			return "catalog image SBOM is required before publish"
		}
		if shared.TextValue(tag, "signature", "signature_ref", "signatureRef", "signed") == "" {
			return "catalog image signature is required before publish"
		}
	}
	return ""
}

// buildProvenanceRejection is the verified half of the provenance gate: the
// catalog digest must correspond to a build record whose pipeline actually
// succeeded (SBOM generated, scan passed, image signed). Catalog metadata alone
// is not proof — only the dispatcher pipeline writes these terminal statuses.
func buildProvenanceRejection(app *platform.App, r *http.Request, tag map[string]any) string {
	digest := shared.TextValue(tag, "digest", "image_digest", "imageDigest")
	for _, record := range app.Store.List(r.Context(), imageBuildsResource) {
		if shared.TextValue(record.Data, "image_digest") != digest {
			continue
		}
		if shared.TextValue(record.Data, "status") == "succeeded" &&
			shared.TextValue(record.Data, "sbom_status") == "succeeded" &&
			shared.TextValue(record.Data, "scan_status") == "passed" &&
			shared.TextValue(record.Data, "signature_status") == "signed" {
			return ""
		}
	}
	return "no verified build provenance for catalog image digest"
}

func catalogScanPassed(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "success", "succeeded", "passed", "pass", "ok", "clean":
		return true
	default:
		return false
	}
}

func enrichRuleWithCatalog(app *platform.App, r *http.Request, rule map[string]any) map[string]any {
	out := shared.CloneMap(rule)
	if tag := catalogByID(app, r, shared.TextValue(rule, "tag_id", "tagId")); len(tag) > 0 {
		promoteCatalogImageStatusFields(out, tag)
		out["catalog"] = tag
	}
	return out
}

func promoteCatalogImageStatusFields(row, catalog map[string]any) {
	promoteCatalogField(row, catalog, "digest", "digest", "image_digest", "imageDigest")
	promoteCatalogField(row, catalog, "scan_status", "scan_status", "scanStatus")
	promoteCatalogBoolField(row, catalog, "deleted", "deleted", "is_deleted", "isDeleted")
	promoteCatalogBoolField(row, catalog, "unavailable", "unavailable", "is_unavailable", "isUnavailable")
	promoteCatalogField(row, catalog, "status", "status")
	promoteCatalogField(row, catalog, "sbom_digest", "sbom_digest", "sbomDigest", "sbom_ref", "sbom")
	promoteCatalogField(row, catalog, "signature", "signature", "signature_ref", "signatureRef", "signed")
}

func promoteCatalogField(row, catalog map[string]any, target string, sources ...string) {
	if imageStatusFieldPresent(row, target) {
		return
	}
	for _, source := range sources {
		value, ok := catalog[source]
		if ok && imageStatusValuePresent(value) {
			row[target] = value
			return
		}
	}
}

func promoteCatalogBoolField(row, catalog map[string]any, target string, sources ...string) {
	if imageStatusFieldPresent(row, target) {
		return
	}
	for _, source := range sources {
		value, ok := catalog[source]
		if ok && imageStatusValuePresent(value) {
			row[target] = canonicalImageBool(value)
			return
		}
	}
}

func imageStatusFieldPresent(row map[string]any, key string) bool {
	value, ok := row[key]
	return ok && imageStatusValuePresent(value)
}

func imageStatusValuePresent(value any) bool {
	if value == nil {
		return false
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text) != ""
	}
	return true
}

func canonicalImageBool(value any) any {
	text, ok := value.(string)
	if !ok {
		return value
	}
	switch {
	case strings.EqualFold(strings.TrimSpace(text), "true"):
		return true
	case strings.EqualFold(strings.TrimSpace(text), "false"):
		return false
	default:
		return value
	}
}

func catalogByID(app *platform.App, r *http.Request, id string) map[string]any {
	for _, tag := range imageCatalogRows(app, r) {
		if idFrom(tag, "id", "tag_id", "tagId") == id {
			return tag
		}
	}
	return map[string]any{}
}

func imageRequestRecord(app *platform.App, r *http.Request, projectID, userID string, payload map[string]any) (map[string]any, error) {
	imageRef := imageReference(payload)
	if imageRef == "" {
		return nil, fmt.Errorf("image reference is required")
	}
	id := shared.FirstNonBlank(shared.TextValue(payload, "id", "request_id", "requestId"), app.Store.NextID(imageRequestsResource, "IR", 1, 6))
	now := time.Now().UTC()
	return map[string]any{
		"id":              id,
		"project_id":      projectID,
		"image_reference": imageRef,
		"registry":        registryFromReference(imageRef),
		"repository":      repositoryFromReference(imageRef),
		"tag":             tagFromReference(imageRef),
		"status":          "pending",
		"requested_by":    userID,
		"created_at":      now,
		"updated_at":      now,
	}, nil
}

func setImageRequestStatus(app *platform.App, r *http.Request, id, statusValue, actor string) (int, any, *platform.Degraded) {
	statusValue = strings.ToLower(strings.TrimSpace(statusValue))
	if id == "" || statusValue == "" {
		return http.StatusBadRequest, shared.ErrorData("id and status are required"), nil
	}
	if statusValue != "approved" && statusValue != "rejected" && statusValue != "pending" {
		return http.StatusBadRequest, shared.ErrorData("invalid image request status"), nil
	}
	record, found := app.Store.Get(r.Context(), imageRequestsResource, id)
	if !found {
		return http.StatusNotFound, shared.ErrorData("image request not found"), nil
	}
	update := map[string]any{"status": statusValue, "reviewed_by": actor, "updated_at": time.Now().UTC()}
	var build func(contracts.Record[map[string]any]) contracts.Event
	switch statusValue {
	case "approved":
		build = func(rec contracts.Record[map[string]any]) contracts.Event {
			return registryEvent(r, "ImageApproved", rec.Data)
		}
	case "rejected":
		build = func(rec contracts.Record[map[string]any]) contracts.Event {
			return registryEvent(r, "ImageRejected", rec.Data)
		}
	}
	updated, ok, err := app.UpdateRecordWithEvent(r.Context(), imageRequestsResource, id, update, build)
	if err != nil || !ok {
		return http.StatusInternalServerError, shared.ErrorData("image request update failed"), nil
	}
	if statusValue == "approved" {
		upsertApprovedRule(app, r, updated.Data, actor)
	}
	_ = record
	return http.StatusOK, updated.Data, nil
}

func upsertApprovedRule(app *platform.App, r *http.Request, request map[string]any, actor string) {
	projectID := shared.TextValue(request, "project_id", "projectId")
	tagID := shared.FirstNonBlank(shared.TextValue(request, "tag_id", "tagId"), shared.TextValue(request, "image_reference"))
	rule := map[string]any{
		"id":              ruleID(projectID, tagID),
		"project_id":      projectID,
		"tag_id":          tagID,
		"repository":      shared.TextValue(request, "repository"),
		"tag":             shared.TextValue(request, "tag"),
		"image_reference": shared.TextValue(request, "image_reference", "imageReference"),
		"enabled":         true,
		"created_by":      actor,
		"updated_at":      time.Now().UTC(),
	}
	upsertRecord(app, r, projectImagesResource, shared.TextValue(rule, "id"), rule)
}

func findProjectImageRule(app *platform.App, r *http.Request, projectID, id string) (imageRecord, bool) {
	for _, record := range app.Store.List(r.Context(), projectImagesResource) {
		data := record.Data
		if idFrom(data, "project_id", "projectId") != projectID {
			continue
		}
		if record.ID == id || hasImageRuleIdentifier(data, id) {
			return imageRecord{ID: record.ID, Data: data}, true
		}
	}
	return imageRecord{}, false
}

func hasImageRuleIdentifier(data map[string]any, id string) bool {
	for _, key := range []string{"id", "tag_id", "tagId", "image_reference"} {
		if idFrom(data, key) == id {
			return true
		}
	}
	return false
}

func findBuild(app *platform.App, r *http.Request, id string) (imageRecord, bool) {
	for _, record := range app.Store.List(r.Context(), imageBuildsResource) {
		if record.ID == id || hasAnyIdentifier(record.Data, id, "id", "job_name", "jobName", "build_id", "buildId") {
			return imageRecord{ID: record.ID, Data: record.Data}, true
		}
	}
	return imageRecord{}, false
}

func hasAnyIdentifier(data map[string]any, id string, keys ...string) bool {
	for _, key := range keys {
		if idFrom(data, key) == id {
			return true
		}
	}
	return false
}

func uniqueHarborProjects(app *platform.App, r *http.Request) []string {
	seen := map[string]bool{}
	for _, tag := range imageCatalogRows(app, r) {
		name := strings.Split(idFrom(tag, "repository", "repository_name", "image_name"), "/")[0]
		if name != "" {
			seen[name] = true
		}
	}
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func filterRows(rows []map[string]any, key, value string) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		if idFrom(row, key) == value {
			out = append(out, row)
		}
	}
	return out
}

func filterByQuery(rows []map[string]any, r *http.Request, keys ...string) []map[string]any {
	out := rows
	for _, key := range keys {
		value := strings.TrimSpace(r.URL.Query().Get(key))
		if value != "" {
			out = filterRows(out, key, value)
		}
	}
	return out
}

func sortRows(rows []map[string]any, keys ...string) {
	sort.Slice(rows, func(i, j int) bool {
		for _, key := range keys {
			left := idFrom(rows[i], key)
			right := idFrom(rows[j], key)
			if left != right {
				return left < right
			}
		}
		return false
	})
}

func upsertRecord(app *platform.App, r *http.Request, resource, id string, data map[string]any) contracts.Record[map[string]any] {
	data["id"] = id
	if updated, ok := app.Store.Update(r.Context(), resource, id, data); ok {
		return updated
	}
	record, err := app.Store.Create(r.Context(), resource, data)
	if err != nil && platform.IsCreateConflict(err) {
		record, _ = app.Store.Update(r.Context(), resource, id, data)
	}
	return record
}

func imageReference(payload map[string]any) string {
	if ref := shared.TextValue(payload, "image_reference", "imageReference", "image"); ref != "" {
		return canonicalImageRef(ref)
	}
	return imageRefFromParts(shared.TextValue(payload, "registry"), shared.TextValue(payload, "repository", "image_name", "imageName"), shared.TextValue(payload, "tag"))
}

func imageRefFromParts(registry, repo, tag string) string {
	repo = strings.TrimSpace(repo)
	if repo == "" {
		return ""
	}
	registry = shared.FirstNonBlank(registry, defaultRegistry)
	tag = shared.FirstNonBlank(tag, defaultTag)
	return registry + "/" + strings.Trim(repo, "/") + ":" + tag
}

func canonicalImageRef(ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	lastSlash := strings.LastIndex(ref, "/")
	lastColon := strings.LastIndex(ref, ":")
	if lastColon <= lastSlash {
		ref += ":" + defaultTag
	}
	if !strings.Contains(strings.Split(ref, "/")[0], ".") && !strings.Contains(strings.Split(ref, "/")[0], ":") {
		ref = defaultRegistry + "/" + ref
	}
	return ref
}

func registryFromReference(ref string) string {
	parts := strings.SplitN(ref, "/", 2)
	if len(parts) == 2 {
		return parts[0]
	}
	return defaultRegistry
}

func repositoryFromReference(ref string) string {
	rest := strings.TrimPrefix(ref, registryFromReference(ref)+"/")
	if i := strings.LastIndex(rest, ":"); i > -1 {
		return rest[:i]
	}
	return rest
}

func tagFromReference(ref string) string {
	if i := strings.LastIndex(ref, ":"); i > strings.LastIndex(ref, "/") {
		return ref[i+1:]
	}
	return defaultTag
}

func normalizeRole(role string) string {
	role = strings.ToLower(strings.TrimSpace(role))
	if role == "member" {
		return "user"
	}
	return role
}

func projectPathID(r *http.Request) string {
	return strings.TrimSpace(r.PathValue("id"))
}

func ruleID(projectID, tagID string) string {
	return projectID + ":" + tagID
}

func batchResult() map[string]any {
	return map[string]any{"succeeded": 0, "failed": 0, "errors": []string{}}
}

func batchError(id string, data any) string {
	if row, ok := data.(map[string]any); ok {
		return id + ": " + shared.TextValue(row, "message")
	}
	return id + ": failed"
}

func firstStringSlice(payload map[string]any, keys ...string) []string {
	for _, key := range keys {
		if values := shared.StringSlice(payload[key]); len(values) > 0 {
			return values
		}
	}
	return nil
}

func idFrom(data map[string]any, keys ...string) string {
	return shared.TextValue(data, keys...)
}

func newBody(payload map[string]any) io.ReadCloser {
	body, _ := json.Marshal(payload)
	return io.NopCloser(bytes.NewReader(body))
}

func sourceCoHosted(app *platform.App, sourceResource string) bool {
	owner, _, ok := strings.Cut(sourceResource, ":")
	return ok && app.Config.AllowsService(owner)
}

func syncImageReadModels(app *platform.App, r *http.Request) {
	if app == nil || app.Store == nil || app.Events == nil {
		return
	}
	app.RunProjection(r.Context(), imageProjectionConsumer, func(event contracts.Event) error {
		return applyImageProjectionEvent(app, r, event)
	})
}

func imageProjectionDrift(ctx context.Context, app *platform.App) (imageProjectionDriftReport, error) {
	var report imageProjectionDriftReport
	if app == nil || app.Store == nil {
		return report, errImageProjectionDriftUnavailable
	}
	for _, pair := range imageProjectionDriftPairs {
		sourceRows := imageProjectionDriftIndex(imageProjectionDriftRecordMaps(ctx, app, pair.sourceResource), pair.idFn)
		localRows := imageProjectionDriftIndex(imageProjectionDriftRecordMaps(ctx, app, pair.localResource), pair.idFn)
		report.addImageProjectionPairDrift(pair, sourceRows, localRows)
	}
	report.sort()
	return report, nil
}

func (r *imageProjectionDriftReport) addImageProjectionPairDrift(pair imageProjectionDriftPair, sourceRows, localRows map[string]map[string]any) {
	r.addImageProjectionMissingAndStale(pair, sourceRows, localRows)
	r.addImageProjectionOrphans(pair, sourceRows, localRows)
}

func (r *imageProjectionDriftReport) addImageProjectionMissingAndStale(pair imageProjectionDriftPair, sourceRows, localRows map[string]map[string]any) {
	for id, sourceRow := range sourceRows {
		localRow, ok := localRows[id]
		finding := imageProjectionDriftFinding{
			SourceResource: pair.sourceResource,
			LocalResource:  pair.localResource,
			ID:             id,
		}
		if !ok {
			r.Missing = append(r.Missing, finding)
			continue
		}
		if !reflect.DeepEqual(sourceRow, localRow) {
			r.Stale = append(r.Stale, finding)
		}
	}
}

func (r *imageProjectionDriftReport) addImageProjectionOrphans(pair imageProjectionDriftPair, sourceRows, localRows map[string]map[string]any) {
	for id := range localRows {
		if _, ok := sourceRows[id]; ok {
			continue
		}
		r.Orphan = append(r.Orphan, imageProjectionDriftFinding{
			SourceResource: pair.sourceResource,
			LocalResource:  pair.localResource,
			ID:             id,
		})
	}
}

func imageProjectionDriftRecordMaps(ctx context.Context, app *platform.App, resource string) []map[string]any {
	if app == nil || app.Store == nil {
		return nil
	}
	records := app.Store.List(ctx, resource)
	out := make([]map[string]any, 0, len(records))
	for _, record := range records {
		out = append(out, shared.CloneMap(record.Data))
	}
	return out
}

func imageProjectionDriftIndex(rows []map[string]any, idFn func(map[string]any) string) map[string]map[string]any {
	out := map[string]map[string]any{}
	if idFn == nil {
		return out
	}
	for _, row := range rows {
		id, normalized := imageProjectionDriftNormalize(row, idFn)
		if id == "" {
			continue
		}
		out[id] = normalized
	}
	return out
}

func imageProjectionDriftNormalize(row map[string]any, idFn func(map[string]any) string) (string, map[string]any) {
	normalized := shared.CloneMap(row)
	id := idFn(normalized)
	if id == "" {
		return "", nil
	}
	normalized["id"] = id
	return id, normalized
}

func (r *imageProjectionDriftReport) sort() {
	sortImageProjectionDriftFindings(r.Missing)
	sortImageProjectionDriftFindings(r.Orphan)
	sortImageProjectionDriftFindings(r.Stale)
}

func sortImageProjectionDriftFindings(findings []imageProjectionDriftFinding) {
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].LocalResource != findings[j].LocalResource {
			return findings[i].LocalResource < findings[j].LocalResource
		}
		if findings[i].ID != findings[j].ID {
			return findings[i].ID < findings[j].ID
		}
		return findings[i].SourceResource < findings[j].SourceResource
	})
}

func applyImageProjectionEvent(app *platform.App, r *http.Request, event contracts.Event) error {
	resource, data, deleted, ok := imageProjection(event)
	if !ok {
		return nil
	}
	if deleted {
		deleteImageProjection(app, r, resource, data)
		return nil
	}
	return upsertImageProjection(app, r, resource, data)
}

func deleteImageProjection(app *platform.App, r *http.Request, resource string, data map[string]any) {
	if id := imageReadModelID(resource, data); id != "" {
		app.Store.Delete(r.Context(), resource, id)
	}
}

func upsertImageProjection(app *platform.App, r *http.Request, resource string, data map[string]any) error {
	id := imageReadModelID(resource, data)
	if id == "" {
		return nil
	}
	data["id"] = id
	if _, ok := app.Store.Update(r.Context(), resource, id, data); ok {
		return nil
	}
	if _, err := app.Store.Create(r.Context(), resource, data); err != nil {
		return recoverImageProjectionCreateConflict(app, r, resource, id, data, err)
	}
	return nil
}

func recoverImageProjectionCreateConflict(app *platform.App, r *http.Request, resource, id string, data map[string]any, err error) error {
	if platform.IsCreateConflict(err) {
		if _, ok := app.Store.Update(r.Context(), resource, id, data); ok {
			return nil
		}
	}
	return fmt.Errorf("image projection upsert failed for %s/%s: %w", resource, id, err)
}

func imageProjection(event contracts.Event) (string, map[string]any, bool, bool) {
	data := imageEventData(event)
	switch strings.ToLower(event.Name) {
	case "usercreated", "userupdated", "userdisabled":
		return imageIdentityUsersResource, data, false, true
	case "userdeleted":
		return imageIdentityUsersResource, data, true, true
	case "rolecreated", "roleupdated":
		return imageIdentityRolesResource, data, false, true
	case "roledeleted":
		return imageIdentityRolesResource, data, true, true
	case "projectcreated", "projectupdated":
		return imageProjectsResource, data, false, true
	case "projectdeleted":
		return imageProjectsResource, data, true, true
	case "project_membercreated", "project_memberupdated":
		return imageProjectMembersResource, data, false, true
	case "project_memberdeleted":
		return imageProjectMembersResource, data, true, true
	case "groupmembershipchanged":
		action := strings.ToLower(idFrom(data, "action"))
		return imageUserGroupsResource, data, action == "delete" || action == "deleted", true
	default:
		return "", nil, false, false
	}
}

func imageEventData(event contracts.Event) map[string]any {
	for _, key := range []string{"new", "record", "project", "member", "user", "role"} {
		if data, ok := event.Data[key].(map[string]any); ok {
			return shared.CloneMap(data)
		}
	}
	return shared.CloneMap(event.Data)
}

func imageReadModelID(resource string, data map[string]any) string {
	id := idFrom(data, "id", "ID")
	projectID := idFrom(data, "project_id", "projectId", "p_id", "PID")
	userID := idFrom(data, "user_id", "userId", "uid", "u_id")
	groupID := idFrom(data, "group_id", "groupId", "gid", "g_id")
	name := idFrom(data, "name", "Name")
	roleID := idFrom(data, "role_id", "roleId", "RoleID")
	switch resource {
	case imageProjectsResource:
		return shared.FirstNonBlank(id, projectID)
	case imageProjectMembersResource:
		if projectID != "" && userID != "" {
			return projectID + ":" + userID
		}
	case imageUserGroupsResource:
		if userID != "" && groupID != "" {
			return userID + ":" + groupID
		}
	case imageIdentityUsersResource:
		return shared.FirstNonBlank(id, userID, name)
	case imageIdentityRolesResource:
		return shared.FirstNonBlank(id, roleID, name)
	}
	return shared.FirstNonBlank(id, projectID, userID, groupID, roleID, name)
}

func mergeRows(resource string, source, local []map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(source)+len(local))
	seen := map[string]bool{}
	for _, row := range local {
		if id := imageReadModelID(resource, row); id != "" {
			seen[id] = true
		}
		out = append(out, row)
	}
	for _, row := range source {
		id := imageReadModelID(resource, row)
		if id == "" || !seen[id] {
			out = append(out, row)
		}
	}
	return out
}
