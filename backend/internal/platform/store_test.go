package platform

import (
	"context"
	"fmt"
	"regexp"
	"sync"
	"testing"
)

var uuidV4Pattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestNewUUIDFormatAndUniqueness(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		id := NewUUID()
		if !uuidV4Pattern.MatchString(id) {
			t.Fatalf("NewUUID returned non-UUIDv4 value %q", id)
		}
		if seen[id] {
			t.Fatalf("NewUUID returned duplicate value %q", id)
		}
		seen[id] = true
	}
}

func TestStoreCreateReturnsCopies(t *testing.T) {
	store := NewStore()
	payload := map[string]any{"id": "r1", "name": "original"}
	created, err := store.Create(context.Background(), "test:records", payload)
	if err != nil {
		t.Fatal(err)
	}
	payload["name"] = "mutated input"
	created.Data["name"] = "mutated returned record"

	got, ok := store.Get(context.Background(), "test:records", "r1")
	if !ok {
		t.Fatal("created record not found")
	}
	if got.Data["name"] != "original" {
		t.Fatalf("store retained caller alias after Create: %#v", got.Data)
	}
}

func TestStoreCreateDuplicateIDReturnsConflict(t *testing.T) {
	store := NewStore()
	if _, err := store.Create(context.Background(), "test:records", map[string]any{"id": "r1", "name": "original"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(context.Background(), "test:records", map[string]any{"id": "r1", "name": "replacement"}); !IsCreateConflict(err) {
		t.Fatalf("duplicate Create error = %v, want create conflict", err)
	}
	records := store.List(context.Background(), "test:records")
	if len(records) != 1 {
		t.Fatalf("record count = %d, want 1", len(records))
	}
	if records[0].Data["name"] != "original" {
		t.Fatalf("duplicate Create overwrote record: %#v", records[0].Data)
	}
}

func TestStoreGetAndListReturnCopies(t *testing.T) {
	store := NewStore()
	_, _ = store.Create(context.Background(), "test:records", map[string]any{"id": "r1", "name": "original"})

	got, ok := store.Get(context.Background(), "test:records", "r1")
	if !ok {
		t.Fatal("record not found")
	}
	got.Data["name"] = "mutated get"
	gotAgain, _ := store.Get(context.Background(), "test:records", "r1")
	if gotAgain.Data["name"] != "original" {
		t.Fatalf("Get returned store alias: %#v", gotAgain.Data)
	}

	listed := store.List(context.Background(), "test:records")
	if len(listed) != 1 {
		t.Fatalf("list length = %d, want 1", len(listed))
	}
	listed[0].Data["name"] = "mutated list"
	listedAgain := store.List(context.Background(), "test:records")
	if listedAgain[0].Data["name"] != "original" {
		t.Fatalf("List returned store alias: %#v", listedAgain[0].Data)
	}
}

func TestStoreUpdateCopyOnWrite(t *testing.T) {
	store := NewStore()
	before, _ := store.Create(context.Background(), "test:records", map[string]any{"id": "r1", "name": "original", "count": 1})

	updated, ok := store.Update(context.Background(), "test:records", "r1", map[string]any{"count": 2})
	if !ok {
		t.Fatal("update failed")
	}
	before.Data["name"] = "mutated old record"
	updated.Data["count"] = 99

	got, ok := store.Get(context.Background(), "test:records", "r1")
	if !ok {
		t.Fatal("updated record not found")
	}
	if got.Version != 2 || got.Data["name"] != "original" || got.Data["count"] != 2 {
		t.Fatalf("Update did not isolate old/returned records: version=%d data=%#v", got.Version, got.Data)
	}
	if before.Data["count"] != 1 {
		t.Fatalf("pre-update returned record observed update: %#v", before.Data)
	}
}

func TestStoreConcurrentReadersAndWriters(t *testing.T) {
	store := NewStore()
	_, _ = store.Create(context.Background(), "test:records", map[string]any{"id": "r1", "value": 0})

	const iterations = 500
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			_, _ = store.Update(context.Background(), "test:records", "r1", map[string]any{
				"value": i,
				"text":  fmt.Sprintf("value-%d", i),
			})
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			record, ok := store.Get(context.Background(), "test:records", "r1")
			if !ok {
				t.Error("record missing during concurrent Get")
				return
			}
			record.Data["reader"] = i
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			for _, record := range store.List(context.Background(), "test:records") {
				record.Data["reader"] = i
			}
		}
	}()
	wg.Wait()

	got, ok := store.Get(context.Background(), "test:records", "r1")
	if !ok {
		t.Fatal("record missing after concurrent access")
	}
	if _, mutated := got.Data["reader"]; mutated {
		t.Fatalf("reader mutation leaked into store: %#v", got.Data)
	}
}
