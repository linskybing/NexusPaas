package workload

import (
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

const (
	serviceName       = "workload-service"
	pathConfigFileID  = "/api/v1/configfiles/{id}"
	msgInvalidBody    = "invalid request body"
	msgConfigNotFound = "config file not found"
)

func Register(app *platform.App) {
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/configfiles", listConfigFiles)
	app.RegisterCustomHandler(http.MethodPost, "/api/v1/configfiles", createConfigFile)
	app.RegisterCustomHandler(http.MethodGet, pathConfigFileID, getConfigFile)
	app.RegisterCustomHandler(http.MethodPut, pathConfigFileID, updateConfigFile)
	app.RegisterCustomHandler(http.MethodPatch, pathConfigFileID, updateConfigFile)
	app.RegisterCustomHandler(http.MethodDelete, pathConfigFileID, deleteConfigFile)
	app.RegisterCustomHandler(http.MethodPost, pathConfigFileID+"/versions", commitConfigFileVersion)
	app.RegisterCustomHandler(http.MethodGet, pathConfigFileID+"/versions", listConfigFileVersions)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/configfiles/tree", listConfigFileTree)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/configfiles/project/{project_id}", listConfigFilesByProject)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/projects/{id}/config-files", listProjectConfigFiles)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/configfiles/project/{project_id}/tree", configFileTree)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/configfiles/project/{project_id}/history", projectConfigHistory)
	app.RegisterCustomHandler(http.MethodPost, pathConfigFileID+"/instance", startConfigInstance)
	app.RegisterCustomHandler(http.MethodDelete, pathConfigFileID+"/instance", stopConfigInstance)
	app.RegisterCustomHandler(http.MethodGet, pathConfigFileID+"/instance/pods", listConfigInstancePods)
	app.RegisterCustomHandler(http.MethodPost, "/api/v1/jobs", submitJob)
	app.RegisterCustomHandler(http.MethodGet, "/internal/workload/preemption-context", workloadPreemptionContext)
	app.RegisterCustomHandler(http.MethodPost, "/internal/workload/jobs/{id}/preempt", workloadPreemptJob)
	app.RegisterCustomHandler(http.MethodPost, "/internal/workload/jobs/{id}/evict", workloadEvictJob)
	registerJobAccessHandlers(app)
	// Service-key-gated read contract: scheduler-quota submit-admission lists jobs to
	// count running/queued usage (problem.md #3). List-only — admission never fetches a
	// single job cross-service.
	app.RegisterReadContract(jobsResource, "/internal/workload/jobs", "")
	registerRuntimeReaper(app)
	registerIdleReaper(app)
	registerJobDispatcher(app)
	registerStatusReconciler(app)
}

func createConfigFile(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	payload, err := platform.DecodeMapWithError(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(msgInvalidBody), nil
	}
	configs := configRepository(app)
	config, err := configPayload(configs, r, payload)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(err.Error()), nil
	}
	if status, data, ok := requireProjectAccess(app, r, shared.TextValue(config, "project_id", "projectId")); !ok {
		return status, data, nil
	}
	record, err := configs.CreateConfig(r.Context(), config)
	if err != nil {
		return createStatus(err), shared.ErrorData("config file could not be created"), nil
	}
	createVersion(configs, r, record.ID, config, "created")
	publish(app, r, "ConfigFileChanged", "created", record.Data)
	return http.StatusCreated, record, nil
}

func getConfigFile(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	record, found := configRepository(app).GetConfig(r.Context(), pathValue(r, "id"))
	if !found {
		return http.StatusNotFound, shared.ErrorData(msgConfigNotFound), nil
	}
	if status, data, ok := requireProjectAccess(app, r, configProjectID(record)); !ok {
		return status, data, nil
	}
	return http.StatusOK, record, nil
}

func updateConfigFile(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	payload, err := platform.DecodeMapWithError(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(msgInvalidBody), nil
	}
	id := pathValue(r, "id")
	configs := configRepository(app)
	existing, found := configs.GetConfig(r.Context(), id)
	if !found {
		return http.StatusNotFound, shared.ErrorData(msgConfigNotFound), nil
	}
	update := normalizeConfigUpdate(payload)
	currentProjectID := configProjectID(existing)
	if targetProjectID := shared.TextValue(update, "project_id", "projectId"); targetProjectID != "" && targetProjectID != currentProjectID {
		return http.StatusBadRequest, shared.ErrorData("project_id is immutable"), nil
	}
	if status, data, ok := requireProjectAccess(app, r, currentProjectID); !ok {
		return status, data, nil
	}
	record, ok := configs.UpdateConfig(r.Context(), id, update)
	if !ok {
		return http.StatusInternalServerError, shared.ErrorData("config file update failed"), nil
	}
	createVersion(configs, r, id, record.Data, "updated")
	publish(app, r, "ConfigFileChanged", "updated", record.Data)
	return http.StatusOK, record, nil
}

func deleteConfigFile(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	id := pathValue(r, "id")
	configs := configRepository(app)
	record, found := configs.GetConfig(r.Context(), id)
	if !found {
		return http.StatusNotFound, shared.ErrorData(msgConfigNotFound), nil
	}
	if status, data, ok := requireProjectAccess(app, r, configProjectID(record)); !ok {
		return status, data, nil
	}
	configs.DeleteConfig(r.Context(), id)
	publish(app, r, "ConfigFileChanged", "deleted", map[string]any{"id": id})
	return http.StatusOK, map[string]any{"id": id, "deleted": true}, nil
}

func listConfigFiles(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	projects, all, status, data, ok := authorizedWorkloadProjects(app, r)
	if !ok {
		return status, data, nil
	}
	return http.StatusOK, filterRecordsForAuthorizedProjects(configRepository(app).ListConfigs(r.Context()), projects, all), nil
}

func listConfigFilesByProject(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	projectID := pathValue(r, "project_id")
	if status, data, ok := requireProjectAccess(app, r, projectID); !ok {
		return status, data, nil
	}
	return http.StatusOK, configFilesForProject(app, r, projectID), nil
}

func listProjectConfigFiles(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	projectID := pathValue(r, "id")
	if status, data, ok := requireProjectAccess(app, r, projectID); !ok {
		return status, data, nil
	}
	return http.StatusOK, configFilesForProject(app, r, projectID), nil
}

func configFileTree(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	projectID := pathValue(r, "project_id")
	if status, data, ok := requireProjectAccess(app, r, projectID); !ok {
		return status, data, nil
	}
	nodes := make([]map[string]any, 0)
	for _, record := range configFilesForProject(app, r, projectID) {
		path := shared.FirstNonEmpty(shared.TextValue(record.Data, "path"), shared.TextValue(record.Data, "name"), record.ID)
		nodes = append(nodes, map[string]any{"id": record.ID, "path": path, "name": shared.TextValue(record.Data, "name")})
	}
	sort.SliceStable(nodes, func(i, j int) bool { return fmt.Sprint(nodes[i]["path"]) < fmt.Sprint(nodes[j]["path"]) })
	return http.StatusOK, map[string]any{"project_id": projectID, "nodes": nodes}, nil
}

func listConfigFileTree(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	projects, all, status, data, ok := authorizedWorkloadProjects(app, r)
	if !ok {
		return status, data, nil
	}
	nodes := make([]map[string]any, 0)
	for _, record := range filterRecordsForAuthorizedProjects(configRepository(app).ListConfigs(r.Context()), projects, all) {
		path := shared.FirstNonEmpty(shared.TextValue(record.Data, "path"), shared.TextValue(record.Data, "name"), record.ID)
		nodes = append(nodes, map[string]any{"id": record.ID, "project_id": configProjectID(record), "path": path, "name": shared.TextValue(record.Data, "name")})
	}
	sort.SliceStable(nodes, func(i, j int) bool {
		return fmt.Sprint(nodes[i]["project_id"], "/", nodes[i]["path"]) < fmt.Sprint(nodes[j]["project_id"], "/", nodes[j]["path"])
	})
	return http.StatusOK, map[string]any{"nodes": nodes}, nil
}

func projectConfigHistory(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	projectID := pathValue(r, "project_id")
	if status, data, ok := requireProjectAccess(app, r, projectID); !ok {
		return status, data, nil
	}
	configs := configRepository(app)
	configIDs := map[string]bool{}
	for _, config := range configFilesForProject(app, r, projectID) {
		configIDs[config.ID] = true
	}
	return http.StatusOK, configs.ListVersionsForConfigs(r.Context(), configIDs), nil
}

func commitConfigFileVersion(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	configID := pathValue(r, "id")
	if workloadProjectAccessEnforced(app) {
		record, found := configRepository(app).GetConfig(r.Context(), configID)
		if !found {
			return http.StatusNotFound, shared.ErrorData(msgConfigNotFound), nil
		}
		if status, data, ok := requireProjectAccess(app, r, configProjectID(record)); !ok {
			return status, data, nil
		}
	}
	payload, err := platform.DecodeMapWithError(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(msgInvalidBody), nil
	}
	record, err := configRepository(app).CommitVersion(r.Context(), configID, payload, time.Now().UTC())
	if err != nil {
		return createStatus(err), shared.ErrorData("config version could not be created"), nil
	}
	publish(app, r, "ConfigCommitted", "committed", record.Data)
	return http.StatusCreated, record, nil
}

func listConfigFileVersions(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	configID := pathValue(r, "id")
	record, found := configRepository(app).GetConfig(r.Context(), configID)
	if !found {
		return http.StatusNotFound, shared.ErrorData(msgConfigNotFound), nil
	}
	if status, data, ok := requireProjectAccess(app, r, configProjectID(record)); !ok {
		return status, data, nil
	}
	return http.StatusOK, configRepository(app).ListVersionsForConfigs(r.Context(), map[string]bool{configID: true}), nil
}

func startConfigInstance(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	return configInstanceCommand(app, r, "start")
}

func stopConfigInstance(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	return configInstanceCommand(app, r, "stop")
}

func listConfigInstancePods(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	id := pathValue(r, "id")
	record, found := configRepository(app).GetConfig(r.Context(), id)
	if !found {
		return http.StatusNotFound, shared.ErrorData(msgConfigNotFound), nil
	}
	if status, data, ok := requireProjectAccess(app, r, configProjectID(record)); !ok {
		return status, data, nil
	}
	return http.StatusOK, configRepository(app).ListInstancesByConfig(r.Context(), id), nil
}

func configInstanceCommand(app *platform.App, r *http.Request, action string) (int, any, *platform.Degraded) {
	id := pathValue(r, "id")
	configs := configRepository(app)
	record, found := configs.GetConfig(r.Context(), id)
	if !found {
		return http.StatusNotFound, shared.ErrorData(msgConfigNotFound), nil
	}
	if status, data, ok := requireProjectAccess(app, r, configProjectID(record)); !ok {
		return status, data, nil
	}
	payload, err := platform.DecodeMapWithError(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(msgInvalidBody), nil
	}
	command, err := configs.CreateInstanceCommand(r.Context(), id, action, payload, time.Now().UTC())
	if err != nil {
		return createStatus(err), shared.ErrorData("instance command could not be created"), nil
	}
	publish(app, r, "ConfigInstanceCommanded", action, command.Data)
	return http.StatusAccepted, command, nil
}

func configPayload(configs *recordStoreWorkloadConfigRepository, r *http.Request, payload map[string]any) (map[string]any, error) {
	projectID := shared.FirstNonEmpty(shared.TextValue(payload, "project_id", "projectId"), strings.TrimSpace(r.URL.Query().Get("project_id")))
	if projectID == "" {
		return nil, fmt.Errorf("project_id is required")
	}
	name := shared.FirstNonEmpty(shared.TextValue(payload, "name"), shared.TextValue(payload, "filename"), shared.TextValue(payload, "path"))
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	config := normalizeConfigUpdate(payload)
	config["project_id"] = projectID
	config["name"] = name
	if shared.TextValue(config, "id") == "" {
		config["id"] = configs.NextConfigID()
	}
	config["created_at"] = time.Now().UTC().Format(time.RFC3339)
	return config, nil
}

func normalizeConfigUpdate(payload map[string]any) map[string]any {
	update := shared.CloneMap(payload)
	if projectID := shared.TextValue(payload, "projectId"); projectID != "" {
		update["project_id"] = projectID
	}
	update["updated_at"] = time.Now().UTC().Format(time.RFC3339)
	return update
}

func configFilesForProject(app *platform.App, r *http.Request, projectID string) []contracts.Record[map[string]any] {
	return configRepository(app).ListConfigsByProject(r.Context(), projectID)
}

func configProjectID(record contracts.Record[map[string]any]) string {
	return shared.TextValue(record.Data, "project_id", "projectId")
}

func createVersion(configs *recordStoreWorkloadConfigRepository, r *http.Request, configID string, data map[string]any, reason string) {
	if _, err := configs.CreateVersion(r.Context(), configID, data, reason, time.Now().UTC()); err != nil && !platform.IsCreateConflict(err) {
		slog.Warn("config version create failed", "config_id", configID, "error", err)
	}
}

func createStatus(err error) int {
	if platform.IsCreateConflict(err) {
		return http.StatusConflict
	}
	return http.StatusInternalServerError
}

func pathValue(r *http.Request, name string) string {
	return strings.TrimSpace(r.PathValue(name))
}

func publish(app *platform.App, r *http.Request, name, action string, data map[string]any) {
	event := contracts.Event{
		EventID:        platform.NewUUID(),
		Name:           name,
		Source:         serviceName,
		OccurredAt:     time.Now().UTC(),
		TraceID:        shared.FirstNonEmpty(r.Header.Get("X-Trace-ID"), "workload-local"),
		SchemaVersion:  1,
		IdempotencyKey: r.Header.Get("Idempotency-Key"),
		Data:           shared.CloneMap(data),
	}
	event.Data["action"] = action
	if err := app.Events.Publish(r.Context(), event); err != nil {
		slog.Error("workload event publish failed", "event", name, "error", err)
	}
}
