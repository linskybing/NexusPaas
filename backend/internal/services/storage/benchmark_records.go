package storage

import (
	"net/http"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

const storageBenchmarkRecordsResource = serviceName + ":storage_benchmark_records"

func createStorageBenchmarkRecord(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	payload, err := platform.DecodeMapWithError(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(msgInvalidRequestBody), nil
	}
	if shared.TextValue(payload, "storage_profile", "storageProfile") == "" {
		return http.StatusBadRequest, shared.ErrorData("storage_profile is required"), nil
	}
	record := shared.CloneMap(payload)
	if shared.TextValue(record, "id") == "" {
		record["id"] = app.Store.NextID(storageBenchmarkRecordsResource, "benchmark-", 1, 5)
	}
	record["storage_profile"] = shared.TextValue(payload, "storage_profile", "storageProfile")
	if _, ok := record["created_at"]; !ok {
		record["created_at"] = time.Now().UTC()
	}
	created, err := app.CreateRecordWithEvent(r.Context(), storageBenchmarkRecordsResource, record, func(record contracts.Record[map[string]any]) contracts.Event {
		payload := shared.CloneMap(record.Data)
		payload["benchmark_record_id"] = record.ID
		payload["action"] = "recorded"
		return storageEvent(r, "StorageBenchmarkRecorded", payload)
	})
	if err != nil {
		return http.StatusConflict, shared.ErrorData("storage benchmark record could not be saved"), nil
	}
	return http.StatusCreated, created, nil
}
