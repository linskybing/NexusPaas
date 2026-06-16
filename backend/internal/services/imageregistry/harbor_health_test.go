package imageregistry

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

type stubHarborAdapter struct {
	result contracts.AdapterResult
	err    error
	calls  int
}

func (s *stubHarborAdapter) Call(_ context.Context, _ string, _ bool) (contracts.AdapterResult, error) {
	s.calls++
	return s.result, s.err
}

func harborHealthRecord(t *testing.T, app *platform.App) map[string]any {
	t.Helper()
	records := app.Store.List(context.Background(), harborHealthResource)
	if len(records) != 1 {
		t.Fatalf("harbor_health records = %d, want 1", len(records))
	}
	return records[0].Data
}

func TestCheckHarborHealthRecordsHealthy(t *testing.T) {
	adapter := &stubHarborAdapter{result: contracts.AdapterResult{Adapter: "harbor"}}
	app := platform.NewApp(platform.Config{ServiceName: serviceName})
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)

	if err := checkHarborHealth(context.Background(), adapter, app.Store, now); err != nil {
		t.Fatalf("check: %v", err)
	}
	if got := harborHealthRecord(t, app)["healthy"]; got != true {
		t.Fatalf("healthy = %v, want true", got)
	}
}

func TestCheckHarborHealthRecordsDegradedAndError(t *testing.T) {
	cases := []struct {
		name    string
		adapter *stubHarborAdapter
	}{
		{"degraded", &stubHarborAdapter{result: contracts.AdapterResult{Degraded: true, Code: "unreachable"}}},
		{"error", &stubHarborAdapter{err: errors.New("dial tcp: refused")}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := platform.NewApp(platform.Config{ServiceName: serviceName})
			if err := checkHarborHealth(context.Background(), tc.adapter, app.Store, time.Now()); err != nil {
				t.Fatalf("check should not propagate error: %v", err)
			}
			if got := harborHealthRecord(t, app)["healthy"]; got != false {
				t.Fatalf("healthy = %v, want false", got)
			}
		})
	}
}

func TestCheckHarborHealthUpsertsSingleRecord(t *testing.T) {
	adapter := &stubHarborAdapter{result: contracts.AdapterResult{Adapter: "harbor"}}
	app := platform.NewApp(platform.Config{ServiceName: serviceName})
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if err := checkHarborHealth(ctx, adapter, app.Store, time.Now()); err != nil {
			t.Fatalf("check %d: %v", i, err)
		}
	}
	if got := len(app.Store.List(ctx, harborHealthResource)); got != 1 {
		t.Fatalf("records = %d, want 1 (upsert)", got)
	}
	if adapter.calls != 3 {
		t.Fatalf("adapter calls = %d, want 3", adapter.calls)
	}
}

func TestCheckHarborHealthNilAdapterNoop(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: serviceName})
	if err := checkHarborHealth(context.Background(), nil, app.Store, time.Now()); err != nil {
		t.Fatalf("nil adapter should be a no-op: %v", err)
	}
	if got := len(app.Store.List(context.Background(), harborHealthResource)); got != 0 {
		t.Fatalf("nil adapter wrote %d records, want 0", got)
	}
}
