package storage

import "testing"

func TestStorageMergeSortAndBatchHelperEdges(t *testing.T) {
	local := []map[string]any{
		{"id": "local", "name": "local row"},
		{"project_id": "P1", "name": "project row"},
	}
	source := []map[string]any{
		{"id": "local", "name": "duplicate"},
		{"user_id": "U1", "name": "source user"},
		{"name": "source without id"},
	}
	merged := mergeRows(source, local)
	if len(merged) != 4 || merged[0]["name"] != "local row" || merged[1]["name"] != "project row" ||
		merged[2]["name"] != "source user" || merged[3]["name"] != "source without id" {
		t.Fatalf("mergeRows = %#v, want local rows plus non-duplicate source rows", merged)
	}

	sortRows(merged, "name")
	if merged[0]["name"] != "local row" || merged[len(merged)-1]["name"] != "source without id" {
		t.Fatalf("sortRows by name = %#v", merged)
	}

	payload := map[string]any{
		"items":       []any{map[string]any{"id": "I1"}, "skip"},
		"permissions": []any{map[string]any{"id": "P1"}},
	}
	items := payloadItems(payload)
	if len(items) != 2 || items[0]["id"] != "I1" || items[1]["id"] != "P1" {
		t.Fatalf("payloadItems = %#v, want item and permission maps", items)
	}

	if got := batchError("alice", map[string]any{"message": "denied"}); got != "alice: denied" {
		t.Fatalf("batchError map = %q, want alice: denied", got)
	}
	if got := batchError("bob", "failed"); got != "bob: failed" {
		t.Fatalf("batchError fallback = %q, want bob: failed", got)
	}
}

func TestStorageMountPlanItemHelpers(t *testing.T) {
	direct := []map[string]any{{"pvc_id": "pvc1"}}
	if got := mountPlanItems(direct); len(got) != 1 || got[0]["pvc_id"] != "pvc1" {
		t.Fatalf("mountPlanItems direct = %#v", got)
	}
	if direct[0]["pvc_id"] != "pvc1" {
		t.Fatal("mountPlanItems should copy the slice header without mutating input")
	}

	mixed := []any{map[string]any{"pvc_id": "pvc2"}, "skip"}
	if got := mountPlanItems(mixed); len(got) != 1 || got[0]["pvc_id"] != "pvc2" {
		t.Fatalf("mountPlanItems mixed = %#v", got)
	}
	if got := mountPlanPayloadItems(map[string]any{"storageMounts": map[string]any{"pvc_id": "pvc3"}}); len(got) != 1 || got[0]["pvc_id"] != "pvc3" {
		t.Fatalf("mountPlanPayloadItems = %#v, want pvc3", got)
	}
	if got := mountPlanItems("bad"); got != nil {
		t.Fatalf("mountPlanItems unsupported = %#v, want nil", got)
	}
}
