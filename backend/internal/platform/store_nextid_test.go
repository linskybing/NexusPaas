package platform

import (
	"context"
	"sync"
	"testing"
)

func TestNextIDFormatAndSequence(t *testing.T) {
	s := NewStore()
	ctx := context.Background()
	if got := s.NextID("svc:users", "US", 2600001, 0); got != "US2600001" {
		t.Fatalf("first id=%q want US2600001", got)
	}
	if got := s.NextID("svc:users", "US", 2600001, 0); got != "US2600002" {
		t.Fatalf("second id=%q want US2600002", got)
	}
	if got := s.NextID("svc:policies", "PO", 2600001, 7); got != "PO2600001" {
		t.Fatalf("padded id=%q want PO2600001", got)
	}
	_ = ctx
}

func TestNextIDSkipsExistingRecords(t *testing.T) {
	s := NewStore()
	ctx := context.Background()
	// A pre-seeded record with the highest suffix forces the allocator past it.
	if _, err := s.Create(ctx, "svc:policies", map[string]any{"id": "PO2600005"}); err != nil {
		t.Fatal(err)
	}
	if got := s.NextID("svc:policies", "PO", 2600001, 7); got != "PO2600006" {
		t.Fatalf("id=%q want PO2600006 (past seeded PO2600005)", got)
	}
}

func TestNextIDNoReuseAfterDelete(t *testing.T) {
	s := NewStore()
	ctx := context.Background()
	first := s.NextID("svc:g", "G", 1, 7)
	if _, err := s.Create(ctx, "svc:g", map[string]any{"id": first}); err != nil {
		t.Fatal(err)
	}
	if !s.Delete(ctx, "svc:g", first) {
		t.Fatal("delete failed")
	}
	// Even though the highest record was deleted, the high-water mark prevents reuse.
	if second := s.NextID("svc:g", "G", 1, 7); second == first {
		t.Fatalf("id reused after delete: %q", second)
	}
}

func TestNextIDConcurrentUnique(t *testing.T) {
	s := NewStore()
	const workers = 64
	var wg sync.WaitGroup
	var mu sync.Mutex
	seen := map[string]bool{}
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			id := s.NextID("svc:concurrent", "ID", 1, 7)
			mu.Lock()
			if seen[id] {
				t.Errorf("duplicate id allocated: %q", id)
			}
			seen[id] = true
			mu.Unlock()
		}()
	}
	wg.Wait()
	if len(seen) != workers {
		t.Fatalf("got %d unique ids, want %d", len(seen), workers)
	}
}
