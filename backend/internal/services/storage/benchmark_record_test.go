package storage

import (
	"net/http"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestStorageBenchmarkRecordCreateAndList(t *testing.T) {
	app := newStorageTestApp(t)

	missingReq := storageRequest(http.MethodPost, "/api/v1/storage/benchmark-records", `{"cluster_id":"prod-gpu-a"}`, "ADMIN")
	code, data, _ := createStorageBenchmarkRecord(app, missingReq, platform.RouteSpec{})
	assertStorageStatus(t, code, data, http.StatusBadRequest)

	createReq := storageRequest(http.MethodPost, "/api/v1/storage/benchmark-records", `{
		"cluster_id":"prod-gpu-a",
		"node_pool":"gpu-hpc-h100",
		"storage_profile":"local-nvme-scratch",
		"metrics":{"fio_read_gbps":11.8,"fio_write_gbps":9.4,"stage_in_gbps":7.2}
	}`, "ADMIN")
	code, data, _ = createStorageBenchmarkRecord(app, createReq, platform.RouteSpec{})
	assertStorageStatus(t, code, data, http.StatusCreated)
	record := data.(contracts.Record[map[string]any])
	if record.Data["storage_profile"] != "local-nvme-scratch" || record.Data["metrics"] == nil {
		t.Fatalf("benchmark record = %#v, want storage profile metrics", record.Data)
	}
	assertFastTransferEventSeen(t, app, "StorageBenchmarkRecorded")

	listReq := storageRequest(http.MethodGet, "/api/v1/storage/benchmark-records", "", "U2")
	if got := storageRows(app, listReq, storageBenchmarkRecordsResource); len(got) != 1 || got[0]["storage_profile"] != "local-nvme-scratch" {
		t.Fatalf("benchmark rows = %#v, want created record", got)
	}
}
