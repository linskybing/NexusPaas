package platform

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestTypedPostgresResourceRoutesImageBuildJobs(t *testing.T) {
	spec, ok := typedPostgresResourceFor(imageRegistryBuildJobsResource)
	if !ok || spec.table != "image_build_jobs" {
		t.Fatalf("typedPostgresResourceFor(%q) = %#v, %v, want image_build_jobs table", imageRegistryBuildJobsResource, spec, ok)
	}
	if _, ok := typedPostgresResourceFor("image-registry-service:container_repositories"); ok {
		t.Fatal("only migrated image-registry resources should route to a typed table")
	}
}

func TestImageBuildInsertColumnsPromoteQueryFields(t *testing.T) {
	cols := imageRegistryBuildInsertColumns(map[string]any{
		"project_id":      "P1",
		"image_reference": "reg/app:1",
		"build_type":      "dockerfile",
		"status":          "queued",
		"requested_by":    "U1",
		"source_digest":   "sha256:abc",
	}, "", time.Time{})
	got := map[string]any{}
	for _, c := range cols {
		got[c.column] = c.value
	}
	for col, want := range map[string]any{"project_id": "P1", "image_reference": "reg/app:1", "build_type": "dockerfile", "status": "queued", "requested_by": "U1", "source_digest": "sha256:abc"} {
		if got[col] != want {
			t.Fatalf("insert column %q = %v, want %v", col, got[col], want)
		}
	}
	// A build with no source archive leaves source_digest NULL, not "".
	noDigest := imageRegistryBuildInsertColumns(map[string]any{"status": "queued"}, "", time.Time{})
	for _, c := range noDigest {
		if c.column == "source_digest" && c.value != nil {
			t.Fatalf("source_digest = %v, want nil when absent", c.value)
		}
	}
}

// P0-5: build-job writes route to the typed image_build_jobs table, not the
// generic platform_records store, while the full payload is preserved.
func TestPostgresStoreRoutesImageBuildJobsToOwnedTable(t *testing.T) {
	now := time.Date(2026, 7, 1, 8, 0, 0, 0, time.UTC)
	db := &fakePostgresDB{
		queryRows: []*fakePostgresRow{{
			values: []any{
				"build-1",
				[]byte(`{"id":"build-1","project_id":"P1","status":"queued","image_reference":"reg/app:1","logs":"build queued\n"}`),
				1, now, now,
			},
		}},
	}
	store := &PostgresStore{db: db}

	created, err := store.Create(context.Background(), imageRegistryBuildJobsResource, map[string]any{
		"id":              "build-1",
		"project_id":      "P1",
		"image_reference": "reg/app:1",
		"build_type":      "dockerfile",
		"status":          "queued",
		"requested_by":    "U1",
		"logs":            "build queued\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.ID != "build-1" || created.Data["logs"] != "build queued\n" {
		t.Fatalf("created build = %#v, want payload preserved", created)
	}
	if got := strings.Join(db.queries, "\n"); !strings.Contains(got, "INSERT INTO image_build_jobs") || strings.Contains(got, "platform_records") {
		t.Fatalf("build query = %s, want image_build_jobs table without platform_records", got)
	}
}
