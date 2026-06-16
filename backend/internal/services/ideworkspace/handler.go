package ideworkspace

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"math"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

const (
	serviceName                = "ide-service"
	authorizationRolesResource = "authorization-policy-service:roles"
	ideIdentityRolesResource   = serviceName + ":ide_identity_roles"
	ideIdentityUsersResource   = serviceName + ":ide_identity_users"
	idePolicyRolesResource     = serviceName + ":ide_policy_roles"
	ideProjectMembersResource  = serviceName + ":ide_project_members"
	ideProjectsResource        = serviceName + ":ide_projects"
	ideUserGroupsResource      = serviceName + ":ide_user_groups"
	identityRolesResource      = "identity-service:roles"
	identityUsersResource      = "identity-service:users"
	orgProjectMembersResource  = "org-project-service:project_members"
	orgProjectsResource        = "org-project-service:projects"
	orgUserGroupsResource      = "org-project-service:user_groups"
	sessionsResource           = serviceName + ":ide_sessions"
	msgInvalidAuthClaims       = "invalid authentication claims"
	msgIDESessionUpdateSkipped = "ide session update skipped"
)

type Image struct {
	Key                string `json:"key"`
	Label              string `json:"label"`
	Image              string `json:"image"`
	IDEType            string `json:"ide_type"`
	Description        string `json:"description"`
	Framework          string `json:"framework"`
	GPURequired        bool   `json:"gpu_required"`
	RootSession        bool   `json:"root_session"`
	RequiresRootPolicy bool   `json:"requires_root_policy"`
}

var baseImages = []Image{
	{Key: "jupyter-base", Label: "Base Notebook", Image: "jupyter/base-notebook:latest", IDEType: "jupyter", Description: "Basic Python environment"},
	{Key: "jupyter-pytorch", Label: "PyTorch (CUDA)", Image: "jupyter/pytorch-notebook:latest", IDEType: "jupyter", Description: "PyTorch with GPU support", Framework: "pytorch", GPURequired: true},
	{Key: "jupyter-tensorflow", Label: "TensorFlow (CUDA)", Image: "jupyter/tensorflow-notebook:latest", IDEType: "jupyter", Description: "TensorFlow with GPU support", Framework: "tensorflow", GPURequired: true},
	{Key: "vscode-base", Label: "Base VS Code", Image: "coder/code-server:latest", IDEType: "vscode", Description: "VS Code with code-server"},
	{Key: "vscode-cuda", Label: "CUDA Dev", Image: "coder/code-server-cuda:latest", IDEType: "vscode", Description: "CUDA development environment", Framework: "cuda", GPURequired: true},
	{Key: "vscode-pytorch", Label: "PyTorch", Image: "coder/code-server-pytorch:latest", IDEType: "vscode", Description: "PyTorch with code-server", Framework: "pytorch", GPURequired: true},
}

var deviceClassRe = regexp.MustCompile(`^[a-z0-9]([-a-z0-9./]*[a-z0-9])?$`)

func Register(app *platform.App) {
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/ide", listIDEs)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/ide/images", listImages)
	app.RegisterCustomHandler(http.MethodPost, "/api/v1/ide/start", startIDE)
	app.RegisterCustomHandler(http.MethodPost, "/api/v1/ide/stop", stopIDE)
	app.RegisterCustomHandler(http.MethodPost, "/api/v1/ide/delete", deleteIDE)
}

func listImages(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	ideType := strings.TrimSpace(r.URL.Query().Get("ide_type"))
	projectID := strings.TrimSpace(r.URL.Query().Get("project_id"))
	allowRoot := false
	if projectID != "" {
		userID := currentUserID(r)
		if userID == "" {
			return http.StatusUnauthorized, map[string]any{"message": msgInvalidAuthClaims}, nil
		}
		if !canAccessProject(app, r, userID, projectID) {
			return http.StatusForbidden, map[string]any{"message": "project member access required"}, nil
		}
		allowRoot = projectAllowsRoot(app, r, projectID)
	}
	return http.StatusOK, imagesForPolicy(ideType, allowRoot), nil
}

func listIDEs(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID := currentUserID(r)
	if userID == "" {
		return http.StatusUnauthorized, map[string]any{"message": msgInvalidAuthClaims}, nil
	}
	projectID := strings.TrimSpace(r.URL.Query().Get("project_id"))
	canViewAll := false
	if projectID != "" {
		if !canAccessProject(app, r, userID, projectID) {
			return http.StatusForbidden, map[string]any{"message": "project member access required"}, nil
		}
		canViewAll = hasAdminPanel(app, r, userID) || projectRoleCanViewAll(app, r, userID, projectID)
	}

	out := []map[string]any{}
	for _, record := range app.Store.List(r.Context(), sessionsResource) {
		session := cloneMap(record.Data)
		if projectID != "" && textValue(session, "project_id", "projectId", "ProjectID") != projectID {
			continue
		}
		if projectID == "" && textValue(session, "user_id", "userId", "UserID") != userID {
			continue
		}
		if projectID != "" && !canViewAll && textValue(session, "user_id", "userId", "UserID") != userID {
			continue
		}
		out = append(out, session)
	}
	sort.Slice(out, func(i, j int) bool {
		return textValue(out[i], "pod_name", "podName") < textValue(out[j], "pod_name", "podName")
	})
	return http.StatusOK, out, nil
}

func startIDE(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	req, status, data, ok := parseStartRequest(r)
	if !ok {
		return status, data, nil
	}
	userID, username, status, data, ok := requireIdentity(r)
	if !ok {
		return status, data, nil
	}
	if status, data, ok := validateStartRequest(app, r, userID, req); !ok {
		return status, data, nil
	}

	podName := podName(userID, req.ideType)
	session := startSessionRecord(req, userID, username, podName, time.Now().UTC())
	if status, data, ok := upsertStartSession(app, r, podName, session); !ok {
		return status, data, nil
	}
	if req.blocking {
		return http.StatusOK, map[string]any{"status": "started", "pod_name": podName}, nil
	}
	return http.StatusOK, map[string]any{"status": "submitted"}, nil
}

func validateStartRequest(app *platform.App, r *http.Request, userID string, req startRequest) (int, any, bool) {
	if !canAccessProject(app, r, userID, req.projectID) {
		return http.StatusForbidden, map[string]any{"message": "project member access required for IDE workspace"}, false
	}
	image := findImage(req.imageKey)
	if !imageExists(req.imageKey, req.ideType) {
		return invalidStartImage(req, image)
	}
	if image != nil && image.RequiresRootPolicy && !projectAllowsRoot(app, r, req.projectID) {
		return http.StatusForbidden, map[string]any{"message": "root IDE profile is not allowed by project policy"}, false
	}
	if req.executorType != "" && req.executorType != "scheduler" && !hasAdminPanel(app, r, userID) {
		return http.StatusForbidden, map[string]any{"message": "executor_type override requires AdminPanel"}, false
	}
	return 0, nil, true
}

func invalidStartImage(req startRequest, image *Image) (int, any, bool) {
	if image == nil {
		return http.StatusBadRequest, map[string]any{"message": fmt.Sprintf("invalid image_key: %s", req.imageKey)}, false
	}
	return http.StatusBadRequest, map[string]any{"message": fmt.Sprintf("image %s is not compatible with IDE type %s", req.imageKey, req.ideType)}, false
}

func startSessionRecord(req startRequest, userID, username, podName string, now time.Time) map[string]any {
	return map[string]any{
		"id":                  podName,
		"pod_name":            podName,
		"project_id":          req.projectID,
		"user_id":             userID,
		"username":            username,
		"ide_type":            req.ideType,
		"image_key":           req.imageKey,
		"gpu":                 req.gpu,
		"status":              "running",
		"storage_ids":         req.storageIDs,
		"queue_name":          req.queueName,
		"sm_percentage":       req.smPercentage,
		"pinned_memory_limit": req.pinnedMemoryLimit,
		"device_class_name":   req.deviceClassName,
		"started_at":          now,
		"updated_at":          now,
	}
}

func upsertStartSession(app *platform.App, r *http.Request, podName string, session map[string]any) (int, any, bool) {
	if existing, ok := app.Store.Get(r.Context(), sessionsResource, podName); ok {
		session["started_at"] = existing.Data["started_at"]
		if _, ok := app.Store.Update(r.Context(), sessionsResource, podName, session); !ok {
			slog.Warn(msgIDESessionUpdateSkipped, "pod_name", podName)
		}
		return 0, nil, true
	}
	if _, err := app.Store.Create(r.Context(), sessionsResource, session); err == nil {
		return 0, nil, true
	} else if !platform.IsCreateConflict(err) {
		return http.StatusInternalServerError, map[string]any{"message": "IDE session could not be started"}, false
	}
	if existing, ok := app.Store.Get(r.Context(), sessionsResource, podName); ok {
		session["started_at"] = existing.Data["started_at"]
	}
	if _, ok := app.Store.Update(r.Context(), sessionsResource, podName, session); !ok {
		slog.Warn(msgIDESessionUpdateSkipped, "pod_name", podName)
	}
	return 0, nil, true
}

func stopIDE(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	return transitionIDE(app, r, "stopped", "IDE workspace stopped")
}

func deleteIDE(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	return transitionIDE(app, r, "deleted", "IDE workspace deleted")
}

func transitionIDE(app *platform.App, r *http.Request, state, _ string) (int, any, *platform.Degraded) {
	req, status, data, ok := parseLifecycleRequest(r)
	if !ok {
		return status, data, nil
	}
	userID, username, status, data, ok := requireIdentity(r)
	if !ok {
		return status, data, nil
	}
	if !canAccessProject(app, r, userID, req.projectID) {
		return http.StatusForbidden, map[string]any{"message": "project member access required for IDE workspace"}, nil
	}
	id := podName(userID, req.ideType)
	if _, ok := app.Store.Get(r.Context(), sessionsResource, id); ok {
		if _, ok := app.Store.Update(r.Context(), sessionsResource, id, map[string]any{
			"status":     state,
			"updated_at": time.Now().UTC(),
		}); !ok {
			slog.Warn(msgIDESessionUpdateSkipped, "pod_name", id, "state", state)
		}
	} else {
		session := map[string]any{
			"id":         id,
			"pod_name":   id,
			"project_id": req.projectID,
			"user_id":    userID,
			"username":   username,
			"ide_type":   req.ideType,
			"status":     state,
			"updated_at": time.Now().UTC(),
		}
		if _, err := app.Store.Create(r.Context(), sessionsResource, session); err != nil {
			if !platform.IsCreateConflict(err) {
				return http.StatusInternalServerError, map[string]any{"message": "IDE session could not be updated"}, nil
			}
			if _, ok := app.Store.Update(r.Context(), sessionsResource, id, map[string]any{
				"status":     state,
				"updated_at": session["updated_at"],
			}); !ok {
				slog.Warn(msgIDESessionUpdateSkipped, "pod_name", id, "state", state)
			}
		}
	}
	return http.StatusOK, nil, nil
}

type startRequest struct {
	projectID         string
	ideType           string
	imageKey          string
	gpu               float64
	executorType      string
	storageIDs        []string
	queueName         string
	smPercentage      any
	pinnedMemoryLimit any
	deviceClassName   string
	blocking          bool
}

type lifecycleRequest struct {
	projectID string
	ideType   string
}

func parseStartRequest(r *http.Request) (startRequest, int, any, bool) {
	payload, status, data, ok := decodeOptionalJSON(r)
	if !ok {
		return startRequest{}, status, data, false
	}
	req := buildStartRequest(r, payload)
	if status, data, ok := parseStartGPU(r, payload, &req); !ok {
		return req, status, data, false
	}
	if status, data, ok := normalizeStartRequest(&req); !ok {
		return req, status, data, false
	}
	if raw, ok := payload["blocking"].(bool); ok {
		req.blocking = raw
	}
	return req, 0, nil, true
}

func buildStartRequest(r *http.Request, payload map[string]any) startRequest {
	req := startRequest{
		projectID:         firstNonEmpty(textValue(payload, "project_id", "projectId"), r.URL.Query().Get("project_id")),
		ideType:           firstNonEmpty(textValue(payload, "ide_type", "ideType"), r.URL.Query().Get("type"), "jupyter"),
		imageKey:          firstNonEmpty(textValue(payload, "image_key", "imageKey"), r.URL.Query().Get("image_key")),
		executorType:      firstNonEmpty(textValue(payload, "executor_type", "executorType"), r.URL.Query().Get("executor_type")),
		storageIDs:        stringSlice(payload["storage_ids"]),
		queueName:         textValue(payload, "queue_name", "queueName"),
		smPercentage:      payload["sm_percentage"],
		pinnedMemoryLimit: payload["pinned_memory_limit"],
		deviceClassName:   firstNonEmpty(textValue(payload, "device_class_name", "deviceClassName"), r.URL.Query().Get("device_class_name")),
		blocking:          true,
	}
	if req.imageKey == "" {
		req.imageKey = req.ideType + "-base"
	}
	return req
}

func parseStartGPU(r *http.Request, payload map[string]any, req *startRequest) (int, any, bool) {
	if raw := r.URL.Query().Get("gpu"); raw != "" {
		gpu, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return http.StatusBadRequest, map[string]any{"message": "invalid gpu value"}, false
		}
		req.gpu = gpu
		return 0, nil, true
	}
	req.gpu = floatValue(payload, "gpu", "GPU")
	return 0, nil, true
}

func normalizeStartRequest(req *startRequest) (int, any, bool) {
	if status, data, ok := validateStartRequestShape(*req); !ok {
		return status, data, false
	}
	if status, data, ok := normalizeSMPercentage(req); !ok {
		return status, data, false
	}
	if status, data, ok := normalizePinnedMemory(req); !ok {
		return status, data, false
	}
	if req.deviceClassName != "" && !deviceClassRe.MatchString(req.deviceClassName) {
		return http.StatusBadRequest, map[string]any{"message": "invalid device class name"}, false
	}
	return 0, nil, true
}

func validateStartRequestShape(req startRequest) (int, any, bool) {
	if math.IsNaN(req.gpu) || math.IsInf(req.gpu, 0) {
		return http.StatusBadRequest, map[string]any{"message": "gpu must be a finite number"}, false
	}
	if req.gpu < 0 {
		return http.StatusBadRequest, map[string]any{"message": "gpu must be >= 0"}, false
	}
	if req.projectID == "" {
		return http.StatusBadRequest, map[string]any{"message": "project_id is required"}, false
	}
	if !validIDEType(req.ideType) {
		return http.StatusBadRequest, map[string]any{"message": "invalid ide_type: must be 'jupyter' or 'vscode'"}, false
	}
	return 0, nil, true
}

func normalizeSMPercentage(req *startRequest) (int, any, bool) {
	if req.smPercentage == nil {
		return 0, nil, true
	}
	sm := int(floatFromAny(req.smPercentage))
	if sm < 1 || sm > 100 {
		return http.StatusBadRequest, map[string]any{"message": "sm_percentage must be between 1 and 100"}, false
	}
	req.smPercentage = sm
	return 0, nil, true
}

func normalizePinnedMemory(req *startRequest) (int, any, bool) {
	raw := strings.TrimSpace(fmt.Sprint(req.pinnedMemoryLimit))
	if raw == "" || raw == "<nil>" {
		req.pinnedMemoryLimit = nil
		return 0, nil, true
	}
	if strings.HasPrefix(raw, "-") || strings.Contains(raw, "not-a-quantity") {
		return http.StatusBadRequest, map[string]any{"message": "invalid pinned memory limit"}, false
	}
	req.pinnedMemoryLimit = raw
	return 0, nil, true
}

func parseLifecycleRequest(r *http.Request) (lifecycleRequest, int, any, bool) {
	payload, status, data, ok := decodeOptionalJSON(r)
	if !ok {
		return lifecycleRequest{}, status, data, false
	}
	req := lifecycleRequest{
		projectID: firstNonEmpty(textValue(payload, "project_id", "projectId"), r.URL.Query().Get("project_id")),
		ideType:   firstNonEmpty(textValue(payload, "ide_type", "ideType"), r.URL.Query().Get("type"), "jupyter"),
	}
	if req.projectID == "" {
		return req, http.StatusBadRequest, map[string]any{"message": "project_id is required"}, false
	}
	if !validIDEType(req.ideType) {
		return req, http.StatusBadRequest, map[string]any{"message": "invalid ide_type: must be 'jupyter' or 'vscode'"}, false
	}
	return req, 0, nil, true
}

func decodeOptionalJSON(r *http.Request) (map[string]any, int, any, bool) {
	if r.Body == nil || r.ContentLength == 0 {
		return map[string]any{}, 0, nil, true
	}
	defer r.Body.Close()
	var payload map[string]any
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		return nil, http.StatusBadRequest, map[string]any{"message": "invalid request body: " + err.Error()}, false
	}
	if payload == nil {
		payload = map[string]any{}
	}
	return payload, 0, nil, true
}

func requireIdentity(r *http.Request) (string, string, int, any, bool) {
	userID := currentUserID(r)
	if userID == "" {
		return "", "", http.StatusUnauthorized, map[string]any{"message": msgInvalidAuthClaims}, false
	}
	username := strings.TrimSpace(r.Header.Get("X-Username"))
	if username == "" {
		return "", "", http.StatusUnauthorized, map[string]any{"message": msgInvalidAuthClaims}, false
	}
	return userID, username, 0, nil, true
}

func imagesForPolicy(ideType string, allowRoot bool) []Image {
	out := []Image{}
	for _, image := range allImages() {
		if ideType != "" && image.IDEType != ideType {
			continue
		}
		if image.RequiresRootPolicy && !allowRoot {
			continue
		}
		out = append(out, image)
	}
	return out
}

func allImages() []Image {
	out := append([]Image{}, baseImages...)
	for _, image := range baseImages {
		root := image
		root.Key += "-root"
		root.Label += " (Root)"
		root.Description += " with root shell access"
		root.RootSession = true
		root.RequiresRootPolicy = true
		out = append(out, root)
	}
	return out
}

func findImage(key string) *Image {
	for _, image := range allImages() {
		if image.Key == key {
			copy := image
			return &copy
		}
	}
	return nil
}

func imageExists(key, ideType string) bool {
	image := findImage(key)
	return image != nil && image.IDEType == ideType
}

func validIDEType(value string) bool {
	return value == "jupyter" || value == "vscode"
}

func podName(userID, ideType string) string {
	return "ide-" + strings.ToLower(userID) + "-" + strings.ToLower(ideType)
}

func canAccessProject(app *platform.App, r *http.Request, userID, projectID string) bool {
	if hasAdminPanel(app, r, userID) {
		return true
	}
	for _, project := range ideRecords(app, r, ideProjectsResource, orgProjectsResource) {
		if recordID(project) != projectID {
			continue
		}
		if textValue(project.Data, "personal_user_id", "personalUserID", "PersonalUserID") == userID {
			return true
		}
		if projectOwnedByUserGroup(app, r, project.Data, userID) {
			return true
		}
	}
	for _, member := range ideRecords(app, r, ideProjectMembersResource, orgProjectMembersResource) {
		if textValue(member.Data, "project_id", "projectId", "ProjectID") == projectID && textValue(member.Data, "user_id", "userId", "UserID") == userID {
			return true
		}
	}
	return false
}

func projectRoleCanViewAll(app *platform.App, r *http.Request, userID, projectID string) bool {
	for _, member := range ideRecords(app, r, ideProjectMembersResource, orgProjectMembersResource) {
		if textValue(member.Data, "project_id", "projectId", "ProjectID") != projectID || textValue(member.Data, "user_id", "userId", "UserID") != userID {
			continue
		}
		role := strings.ToLower(textValue(member.Data, "role", "Role"))
		return role == "admin" || role == "manager"
	}
	return false
}

func projectOwnedByUserGroup(app *platform.App, r *http.Request, project map[string]any, userID string) bool {
	ownerID := textValue(project, "owner_id", "ownerId", "OwnerID", "GID", "group_id", "groupId")
	if ownerID == "" {
		return false
	}
	for _, member := range ideRecords(app, r, ideUserGroupsResource, orgUserGroupsResource) {
		if textValue(member.Data, "group_id", "groupId", "GroupID") == ownerID && textValue(member.Data, "user_id", "userId", "UserID") == userID {
			return true
		}
	}
	return false
}

func projectAllowsRoot(app *platform.App, r *http.Request, projectID string) bool {
	for _, project := range ideRecords(app, r, ideProjectsResource, orgProjectsResource) {
		if recordID(project) == projectID {
			return boolValue(project.Data, "allow_run_as_root", "allowRunAsRoot", "AllowRunAsRoot")
		}
	}
	return false
}

func hasAdminPanel(app *platform.App, r *http.Request, userID string) bool {
	for _, user := range ideRecords(app, r, ideIdentityUsersResource, identityUsersResource) {
		if recordID(user) != userID && textValue(user.Data, "user_id", "userId", "UserID") != userID {
			continue
		}
		if recordGrantsAdminPanel(user.Data) {
			return true
		}
		roleID := textValue(user.Data, "role_id", "roleId", "RoleID")
		for _, role := range roleRecords(app, r) {
			if recordID(role) == roleID {
				return recordGrantsAdminPanel(role.Data)
			}
		}
		return false
	}
	return false
}

func roleRecords(app *platform.App, r *http.Request) []contracts.Record[map[string]any] {
	records := ideRecords(app, r, ideIdentityRolesResource, identityRolesResource)
	return append(records, ideRecords(app, r, idePolicyRolesResource, authorizationRolesResource)...)
}

func recordGrantsAdminPanel(data map[string]any) bool {
	if boolValue(data, "admin_panel", "adminPanel", "AdminPanel") {
		return true
	}
	capabilities := mapValue(data, "capabilities", "Capabilities")
	return boolValue(capabilities, "admin_panel", "adminPanel", "AdminPanel")
}

func currentUserID(r *http.Request) string {
	return strings.TrimSpace(r.Header.Get("X-User-ID"))
}

func textValue(data map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := data[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func floatValue(data map[string]any, keys ...string) float64 {
	for _, key := range keys {
		if value, ok := data[key]; ok {
			return floatFromAny(value)
		}
	}
	return 0
}

func floatFromAny(value any) float64 {
	switch value := value.(type) {
	case float64:
		return value
	case float32:
		return float64(value)
	case int:
		return float64(value)
	case int64:
		return float64(value)
	case json.Number:
		if parsed, err := value.Float64(); err == nil {
			return parsed
		}
	}
	return 0
}

func stringSlice(value any) []string {
	raw, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if text, ok := item.(string); ok {
			out = append(out, text)
		}
	}
	return out
}

func boolValue(data map[string]any, keys ...string) bool {
	for _, key := range keys {
		switch value := data[key].(type) {
		case bool:
			return value
		case string:
			return strings.EqualFold(value, "true")
		}
	}
	return false
}

func mapValue(data map[string]any, keys ...string) map[string]any {
	for _, key := range keys {
		if value, ok := data[key].(map[string]any); ok {
			return value
		}
	}
	return nil
}

func recordID(record contracts.Record[map[string]any]) string {
	return firstNonEmpty(record.ID, textValue(record.Data, "id", "ID", "p_id", "pID", "PID", "project_id", "projectId", "ProjectID"))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func cloneMap(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	return maps.Clone(in)
}
