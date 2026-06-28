package k8scontrol

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/platform/cluster"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

type fastTransferMoverJobRequest struct {
	ProjectID        string                    `json:"project_id"`
	TransferID       string                    `json:"transfer_id"`
	TargetNamespace  string                    `json:"target_namespace"`
	Name             string                    `json:"name"`
	Source           fastTransferMoverEndpoint `json:"source"`
	Target           fastTransferMoverEndpoint `json:"target"`
	Tool             string                    `json:"tool"`
	ProgressCallback struct {
		Path string `json:"path"`
	} `json:"progress_callback"`
}

type fastTransferMoverEndpoint struct {
	Namespace string `json:"namespace"`
	PVC       string `json:"pvc"`
	Path      string `json:"path"`
}

func createFastTransferMoverJob(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	if app == nil || !app.ServiceRequestAuthorized(r) {
		return http.StatusUnauthorized, shared.ErrorData("service authentication is required"), nil
	}
	var req fastTransferMoverJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return http.StatusBadRequest, shared.ErrorData("invalid request body"), nil
	}
	result := ensureFastTransferMoverJob(r, app, req)
	logFastTransferMoverJobResult(result)
	switch result.Action {
	case cluster.FastTransferMoverActionCreated:
		return http.StatusCreated, result, nil
	case cluster.FastTransferMoverActionAlreadyExists:
		return http.StatusOK, result, nil
	case cluster.FastTransferMoverActionInvalid:
		return http.StatusUnprocessableEntity, result, nil
	default:
		return http.StatusBadGateway, result, nil
	}
}

func ensureFastTransferMoverJob(r *http.Request, app *platform.App, req fastTransferMoverJobRequest) cluster.FastTransferMoverJobResult {
	namespace := strings.TrimSpace(req.TargetNamespace)
	name := strings.TrimSpace(req.Name)
	if !strings.HasPrefix(name, "fast-transfer-") {
		name = "fast-transfer-" + name
	}
	if app == nil || app.Cluster == nil {
		return cluster.FastTransferMoverJobResult{
			Namespace: namespace,
			Name:      name,
			Action:    cluster.FastTransferMoverActionDegraded,
			Reason:    "cluster client unavailable",
		}
	}
	return app.Cluster.EnsureFastTransferMoverJob(r.Context(), cluster.FastTransferMoverJobOptions{
		ProjectID:           req.ProjectID,
		TransferID:          req.TransferID,
		Namespace:           namespace,
		Name:                name,
		Source:              cluster.FastTransferMoverEndpoint{Namespace: req.Source.Namespace, PVC: req.Source.PVC, Path: req.Source.Path},
		Target:              cluster.FastTransferMoverEndpoint{Namespace: req.Target.Namespace, PVC: req.Target.PVC, Path: req.Target.Path},
		Tool:                req.Tool,
		Image:               app.Config.FastTransferMoverImage,
		ProgressURL:         fastTransferProgressCallbackURL(app, req),
		ProgressServiceName: fastTransferProgressServiceName(app),
		ProgressServiceKey:  fastTransferProgressServiceKey(app),
	})
}

func fastTransferProgressCallbackURL(app *platform.App, req fastTransferMoverJobRequest) string {
	if app == nil {
		return ""
	}
	base := strings.TrimRight(strings.TrimSpace(app.Config.ServiceURLs["storage-service"]), "/")
	if base == "" || fastTransferProgressServiceKey(app) == "" {
		return ""
	}
	expected := fastTransferProgressCallbackPath(req)
	if expected == "" || !fastTransferProgressCallbackPathAllowed(req.ProgressCallback.Path, expected) {
		return ""
	}
	return base + expected
}

func fastTransferProgressCallbackPath(req fastTransferMoverJobRequest) string {
	projectID := strings.TrimSpace(req.ProjectID)
	namespace := strings.TrimSpace(req.TargetNamespace)
	name := strings.TrimSpace(req.Name)
	if projectID == "" || namespace == "" || name == "" {
		return ""
	}
	return "/internal/storage/projects/" + projectID + "/transfers/" + namespace + "/" + name + "/progress"
}

func fastTransferProgressCallbackPathAllowed(got, expected string) bool {
	got = strings.TrimSpace(got)
	if got == "" {
		return true
	}
	if parsed, err := url.Parse(got); err == nil && parsed.IsAbs() {
		return false
	}
	return got == expected
}

func fastTransferProgressServiceName(app *platform.App) string {
	if app == nil {
		return ""
	}
	return strings.TrimSpace(app.Config.ServiceIdentityName)
}

func fastTransferProgressServiceKey(app *platform.App) string {
	if app == nil {
		return ""
	}
	if key := strings.TrimSpace(app.Config.ServiceIdentityKey); key != "" && fastTransferProgressServiceName(app) != "" {
		return key
	}
	if len(app.Config.ServiceTrustedIdentities) > 0 || app.Config.StrictRuntimeChecks() {
		return ""
	}
	return strings.TrimSpace(app.Config.ServiceAPIKey)
}

func logFastTransferMoverJobResult(result cluster.FastTransferMoverJobResult) {
	slog.Info("fast transfer mover job dispatch completed",
		"namespace", result.Namespace,
		"name", result.Name,
		"action", result.Action,
		"reason", result.Reason,
	)
}
