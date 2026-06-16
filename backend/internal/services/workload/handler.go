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
	app.RegisterCustomHandler(http.MethodPost, "/api/v1/configfiles", createConfigFile)
	app.RegisterCustomHandler(http.MethodGet, pathConfigFileID, getConfigFile)
	app.RegisterCustomHandler(http.MethodPut, pathConfigFileID, updateConfigFile)
	app.RegisterCustomHandler(http.MethodPatch, pathConfigFileID, updateConfigFile)
	app.RegisterCustomHandler(http.MethodDelete, pathConfigFileID, deleteConfigFile)
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
	return http.StatusOK, record, nil
}

func updateConfigFile(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	payload, err := platform.DecodeMapWithError(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(msgInvalidBody), nil
	}
	id := pathValue(r, "id")
	configs := configRepository(app)
	if _, found := configs.GetConfig(r.Context(), id); !found {
		return http.StatusNotFound, shared.ErrorData(msgConfigNotFound), nil
	}
	update := normalizeConfigUpdate(payload)
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
	if !configRepository(app).DeleteConfig(r.Context(), id) {
		return http.StatusNotFound, shared.ErrorData(msgConfigNotFound), nil
	}
	publish(app, r, "ConfigFileChanged", "deleted", map[string]any{"id": id})
	return http.StatusOK, map[string]any{"id": id, "deleted": true}, nil
}

func listConfigFilesByProject(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	return http.StatusOK, configFilesForProject(app, r, pathValue(r, "project_id")), nil
}

func listProjectConfigFiles(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	return http.StatusOK, configFilesForProject(app, r, pathValue(r, "id")), nil
}

func configFileTree(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	projectID := pathValue(r, "project_id")
	nodes := make([]map[string]any, 0)
	for _, record := range configFilesForProject(app, r, projectID) {
		path := shared.FirstNonEmpty(shared.TextValue(record.Data, "path"), shared.TextValue(record.Data, "name"), record.ID)
		nodes = append(nodes, map[string]any{"id": record.ID, "path": path, "name": shared.TextValue(record.Data, "name")})
	}
	sort.SliceStable(nodes, func(i, j int) bool { return fmt.Sprint(nodes[i]["path"]) < fmt.Sprint(nodes[j]["path"]) })
	return http.StatusOK, map[string]any{"project_id": projectID, "nodes": nodes}, nil
}

func projectConfigHistory(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	projectID := pathValue(r, "project_id")
	configs := configRepository(app)
	configIDs := map[string]bool{}
	for _, config := range configFilesForProject(app, r, projectID) {
		configIDs[config.ID] = true
	}
	return http.StatusOK, configs.ListVersionsForConfigs(r.Context(), configIDs), nil
}

func startConfigInstance(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	return configInstanceCommand(app, r, "start")
}

func stopConfigInstance(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	return configInstanceCommand(app, r, "stop")
}

func listConfigInstancePods(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	id := pathValue(r, "id")
	return http.StatusOK, configRepository(app).ListInstancesByConfig(r.Context(), id), nil
}

func configInstanceCommand(app *platform.App, r *http.Request, action string) (int, any, *platform.Degraded) {
	id := pathValue(r, "id")
	configs := configRepository(app)
	if _, found := configs.GetConfig(r.Context(), id); !found {
		return http.StatusNotFound, shared.ErrorData(msgConfigNotFound), nil
	}
	payload, err := platform.DecodeMapWithError(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(msgInvalidBody), nil
	}
	record, err := configs.CreateInstanceCommand(r.Context(), id, action, payload, time.Now().UTC())
	if err != nil {
		return createStatus(err), shared.ErrorData("instance command could not be created"), nil
	}
	publish(app, r, "ConfigInstanceCommanded", action, record.Data)
	return http.StatusAccepted, record, nil
}

func configPayload(configs workloadConfigRepository, r *http.Request, payload map[string]any) (map[string]any, error) {
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

func createVersion(configs workloadConfigRepository, r *http.Request, configID string, data map[string]any, reason string) {
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
