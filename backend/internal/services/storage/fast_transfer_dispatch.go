package storage

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

const (
	k8sControlServiceName = "k8s-control-service"

	fastTransferDispatchNotConfigured = "not_configured"
	fastTransferDispatchSubmitted     = "submitted"
	fastTransferDispatchUnavailable   = "unavailable"
	fastTransferDispatchFailed        = "failed"
)

type fastTransferMoverDispatchRequest struct {
	ProjectID        string                       `json:"project_id"`
	TransferID       string                       `json:"transfer_id"`
	TargetNamespace  string                       `json:"target_namespace"`
	Name             string                       `json:"name"`
	Source           fastTransferDispatchEndpoint `json:"source"`
	Target           fastTransferDispatchEndpoint `json:"target"`
	Tool             string                       `json:"tool"`
	ProgressCallback map[string]string            `json:"progress_callback"`
}

type fastTransferDispatchEndpoint struct {
	Namespace string `json:"namespace"`
	PVC       string `json:"pvc"`
	Path      string `json:"path"`
}

type fastTransferMoverDispatchResponse struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Action    string `json:"action"`
	Reason    string `json:"reason,omitempty"`
	Error     string `json:"error,omitempty"`
}

func dispatchFastTransferMoverJob(ctx context.Context, app *platform.App, repo *recordStoreStorageRepository, record map[string]any, now time.Time) map[string]any {
	patch := fastTransferDispatchPatch(app, record)
	if shared.TextValue(patch, "dispatch_status") != "" {
		next := persistFastTransferDispatch(ctx, repo, record, patch, now)
		logFastTransferDispatch(next)
		return next
	}

	var response fastTransferMoverDispatchResponse
	resp, err := platform.NewInternalJSONClient(app, k8sControlServiceName).Do(ctx, platform.InternalJSONRequest{
		Method:   http.MethodPost,
		Path:     "/internal/k8s-control/fast-transfers/mover-jobs",
		Body:     fastTransferMoverDispatchPayload(record),
		Response: &response,
	})
	if err != nil {
		patch = fastTransferDispatchMetadata(fastTransferDispatchUnavailable, err.Error(), "", "")
	} else if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		patch = fastTransferDispatchMetadata(fastTransferDispatchSubmitted, "", response.Namespace, response.Name)
	} else {
		status := fastTransferDispatchUnavailable
		if resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusUnprocessableEntity {
			status = fastTransferDispatchFailed
		}
		patch = fastTransferDispatchMetadata(status, fastTransferDispatchHTTPError(resp, response), response.Namespace, response.Name)
	}
	next := persistFastTransferDispatch(ctx, repo, record, patch, now)
	logFastTransferDispatch(next)
	return next
}

func logFastTransferDispatch(record map[string]any) {
	slog.Info("fast transfer mover dispatch completed",
		"transfer_id", shared.TextValue(record, "id"),
		"project_id", shared.TextValue(record, "project_id"),
		"dispatch_status", shared.TextValue(record, "dispatch_status"),
	)
}

func fastTransferDispatchPatch(app *platform.App, record map[string]any) map[string]any {
	if app == nil || !fastTransferCanSendServiceIdentity(app) {
		return fastTransferDispatchMetadata(fastTransferDispatchNotConfigured, "service identity is not configured", "", "")
	}
	if !app.Config.AllowsService(k8sControlServiceName) && strings.TrimSpace(app.Config.ServiceURLs[k8sControlServiceName]) == "" {
		return fastTransferDispatchMetadata(fastTransferDispatchNotConfigured, "k8s-control-service URL is not configured", "", "")
	}
	if shared.TextValue(record, "id") == "" {
		return fastTransferDispatchMetadata(fastTransferDispatchFailed, "transfer id is missing", "", "")
	}
	return nil
}

func fastTransferCanSendServiceIdentity(app *platform.App) bool {
	return strings.TrimSpace(app.Config.ServiceIdentityName) != "" && strings.TrimSpace(app.Config.ServiceIdentityKey) != "" ||
		strings.TrimSpace(app.Config.ServiceAPIKey) != ""
}

func fastTransferMoverDispatchPayload(record map[string]any) fastTransferMoverDispatchRequest {
	namespace := shared.TextValue(record, "target_namespace", "targetNamespace")
	name := shared.TextValue(record, "name")
	return fastTransferMoverDispatchRequest{
		ProjectID:       shared.TextValue(record, "project_id", "projectId"),
		TransferID:      shared.TextValue(record, "id"),
		TargetNamespace: namespace,
		Name:            name,
		Source:          fastTransferEndpointFromRecord(record, "source", namespace),
		Target:          fastTransferEndpointFromRecord(record, "target", namespace),
		Tool:            shared.FirstNonBlank(shared.TextValue(record, "tool"), "rsync"),
		ProgressCallback: map[string]string{
			"path": fmt.Sprintf("/internal/storage/projects/%s/transfers/%s/%s/progress", shared.TextValue(record, "project_id", "projectId"), namespace, name),
		},
	}
}

func fastTransferEndpointFromRecord(record map[string]any, key, fallbackNamespace string) fastTransferDispatchEndpoint {
	data, _ := record[key].(map[string]any)
	return fastTransferDispatchEndpoint{
		Namespace: shared.FirstNonBlank(shared.TextValue(data, "namespace"), fallbackNamespace),
		PVC:       shared.TextValue(data, "pvc", "pvc_id", "pvcId"),
		Path:      shared.FirstNonBlank(shared.TextValue(data, "path"), "/"),
	}
}

func fastTransferDispatchHTTPError(resp platform.InternalJSONResponse, response fastTransferMoverDispatchResponse) string {
	if response.Reason != "" {
		return response.Reason
	}
	if response.Error != "" {
		return response.Error
	}
	if resp.EnvelopeError != nil && resp.EnvelopeError.Message != "" {
		return resp.EnvelopeError.Message
	}
	return fmt.Sprintf("k8s-control-service returned %d", resp.StatusCode)
}

func fastTransferDispatchMetadata(status, errText, namespace, name string) map[string]any {
	return map[string]any{
		"dispatch_status":     status,
		"dispatch_error":      truncateDispatchError(errText),
		"mover_job_namespace": namespace,
		"mover_job_name":      name,
	}
}

func persistFastTransferDispatch(ctx context.Context, repo *recordStoreStorageRepository, record, patch map[string]any, now time.Time) map[string]any {
	patch["updated_at"] = now.UTC()
	updated, ok := repo.UpdateFastTransferDispatch(ctx,
		shared.TextValue(record, "project_id", "projectId"),
		shared.TextValue(record, "target_namespace", "targetNamespace"),
		shared.TextValue(record, "name"),
		patch,
	)
	if ok {
		return updated
	}
	next := shared.CloneMap(record)
	for key, value := range patch {
		next[key] = value
	}
	return next
}

func truncateDispatchError(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 200 {
		return value[:200]
	}
	return value
}
