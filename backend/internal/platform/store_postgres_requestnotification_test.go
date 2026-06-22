package platform

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/linskybing/nexuspaas/backend/internal/contracts"
)

func TestTypedPostgresResourceForCoversIdentityAndRequestNotification(t *testing.T) {
	for _, resource := range []string{
		identityUsersResource,
		requestNotificationFormsResource,
		requestNotificationFormMessagesResource,
		requestNotificationAnnouncementsResource,
		requestNotificationAnnouncementReadsResource,
		requestNotificationNotificationsResource,
	} {
		if _, ok := typedPostgresResourceFor(resource); !ok {
			t.Fatalf("typedPostgresResourceFor(%q) = false, want owned typed table", resource)
		}
	}
	// project_access_* are read-model projections of org-project events, not
	// owned writes, so they intentionally stay on the generic platform_records.
	if _, ok := typedPostgresResourceFor("request-notification-service:project_access_projects"); ok {
		t.Fatal("project_access_projects must stay on platform_records, not a typed table")
	}
}

func TestPostgresStoreRoutesFormsToOwnedTable(t *testing.T) {
	now := time.Date(2026, 6, 22, 8, 0, 0, 0, time.UTC)
	db := &fakePostgresDB{
		queryRows: []*fakePostgresRow{{
			values: []any{
				"form100001",
				[]byte(`{"id":"form100001","user_id":"US1","status":"Pending","description":"kept"}`),
				1,
				now,
				now,
			},
		}},
	}
	store := &PostgresStore{db: db}

	created, err := store.Create(context.Background(), requestNotificationFormsResource, map[string]any{
		"id":          "form100001",
		"user_id":     "US1",
		"title":       "Need GPU",
		"description": "kept",
		"tag":         "resource",
		"status":      "Pending",
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.ID != "form100001" || created.Data["description"] != "kept" {
		t.Fatalf("created form = %#v", created)
	}
	if got := strings.Join(db.queries, "\n"); !strings.Contains(got, "INSERT INTO forms") || strings.Contains(got, "platform_records") {
		t.Fatalf("form query = %s, want forms table without platform_records", got)
	}
}

func TestPostgresStoreFormsListUpdateDeleteUseOwnedTable(t *testing.T) {
	now := time.Date(2026, 6, 22, 9, 0, 0, 0, time.UTC)
	later := now.Add(time.Minute)
	db := &fakePostgresDB{
		queryResults: []*fakePostgresRows{{
			rows: [][]any{
				{"form100001", []byte(`{"id":"form100001","status":"Pending"}`), 1, now, now},
				{"form100002", []byte(`{"id":"form100002","status":"Processing"}`), 2, now, later},
			},
		}},
		queryRows: []*fakePostgresRow{{
			values: []any{
				"form100001",
				[]byte(`{"id":"form100001","status":"Completed"}`),
				2,
				now,
				later,
			},
		}},
		execTags: []pgconn.CommandTag{pgconn.NewCommandTag("DELETE 1")},
	}
	store := &PostgresStore{db: db}
	ctx := context.Background()

	records := store.List(ctx, requestNotificationFormsResource)
	if len(records) != 2 || records[1].Data["status"] != "Processing" {
		t.Fatalf("forms list = %#v", records)
	}
	updated, ok := store.Update(ctx, requestNotificationFormsResource, "form100001", map[string]any{"status": "Completed"})
	if !ok || updated.Version != 2 || updated.Data["status"] != "Completed" {
		t.Fatalf("forms update = %#v ok=%v", updated, ok)
	}
	if !store.Delete(ctx, requestNotificationFormsResource, "form100002") {
		t.Fatal("forms delete returned false")
	}
	queries := strings.Join(db.queries, "\n")
	for _, want := range []string{"FROM forms", "UPDATE forms", "DELETE FROM forms"} {
		if !strings.Contains(queries, want) {
			t.Fatalf("forms queries = %s, want %s", queries, want)
		}
	}
	if strings.Contains(queries, "platform_records") {
		t.Fatalf("forms queries leaked platform_records: %s", queries)
	}
}

func TestPostgresStoreRoutesFormMessagesToOwnedTable(t *testing.T) {
	now := time.Date(2026, 6, 22, 10, 0, 0, 0, time.UTC)
	db := &fakePostgresDB{
		queryRows: []*fakePostgresRow{{
			values: []any{
				"msg100001",
				[]byte(`{"id":"msg100001","form_id":"form100001","user_id":"US1","content":"hello"}`),
				1,
				now,
				now,
			},
		}},
	}
	store := &PostgresStore{db: db}

	if _, err := store.Create(context.Background(), requestNotificationFormMessagesResource, map[string]any{
		"id":      "msg100001",
		"form_id": "form100001",
		"user_id": "US1",
		"content": "hello",
	}); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(db.queries, "\n"); !strings.Contains(got, "INSERT INTO form_messages") || strings.Contains(got, "platform_records") {
		t.Fatalf("form message query = %s, want form_messages table", got)
	}
}

func TestRequestNotificationFormColumnsPromoteAliasesAndNulls(t *testing.T) {
	insert := requestNotificationFormInsertColumns(map[string]any{
		"userId":    "US1",
		"projectId": "PR1",
		"tag":       "bug",
		"title":     "broken",
	}, "form100001", time.Now())
	got := columnValueMap(insert)
	want := map[string]any{
		"user_id":    "US1",
		"project_id": "PR1",
		"tag":        "bug",
		"title":      "broken",
		"status":     "Pending", // defaulted when absent
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("form insert columns = %#v, want %#v", got, want)
	}

	// An absent project_id must persist as a SQL NULL, not an empty string.
	insertNoProject := columnValueMap(requestNotificationFormInsertColumns(map[string]any{"user_id": "US1"}, "form1", time.Now()))
	if insertNoProject["project_id"] != nil {
		t.Fatalf("absent project_id = %#v, want nil", insertNoProject["project_id"])
	}

	// Update only emits columns whose keys are present in the patch.
	update := columnValueMap(requestNotificationFormUpdateColumns(map[string]any{"status": "Completed"}))
	if len(update) != 1 || update["status"] != "Completed" {
		t.Fatalf("form update columns = %#v, want only status", update)
	}
}

func TestRequestNotificationFormMessageColumnsPromoteAliases(t *testing.T) {
	insert := columnValueMap(requestNotificationFormMessageInsertColumns(map[string]any{
		"formId":  "form100001",
		"user_id": "US1",
	}, "msg1", time.Now()))
	if insert["form_id"] != "form100001" || insert["user_id"] != "US1" {
		t.Fatalf("form message insert columns = %#v", insert)
	}
	update := columnValueMap(requestNotificationFormMessageUpdateColumns(map[string]any{"form_id": "form100002"}))
	if len(update) != 1 || update["form_id"] != "form100002" {
		t.Fatalf("form message update columns = %#v, want only form_id", update)
	}
}

func TestPostgresStoreGetRoutesFormsToOwnedTable(t *testing.T) {
	now := time.Date(2026, 6, 22, 13, 0, 0, 0, time.UTC)
	db := &fakePostgresDB{
		queryRows: []*fakePostgresRow{{
			values: []any{"form100001", []byte(`{"id":"form100001","status":"Pending"}`), 1, now, now},
		}},
	}
	store := &PostgresStore{db: db}

	rec, ok := store.Get(context.Background(), requestNotificationFormsResource, "form100001")
	if !ok || rec.ID != "form100001" {
		t.Fatalf("get form = %#v ok=%v", rec, ok)
	}
	if got := strings.Join(db.queries, "\n"); !strings.Contains(got, "FROM forms") || strings.Contains(got, "platform_records") {
		t.Fatalf("get query = %s, want forms table", got)
	}
}

func TestPostgresStoreUpsertWithEventRoutesFormsToOwnedTable(t *testing.T) {
	now := time.Date(2026, 6, 22, 13, 30, 0, 0, time.UTC)
	db := &fakePostgresDB{
		tx: &fakePostgresTx{
			fakePostgresDB: fakePostgresDB{
				queryRows: []*fakePostgresRow{{
					values: []any{"form100001", []byte(`{"id":"form100001","status":"Processing"}`), 2, now, now},
				}},
			},
		},
	}
	store := &PostgresStore{db: db}

	rec, err := store.UpsertWithEvent(context.Background(), requestNotificationFormsResource, "form100001",
		map[string]any{"status": "Processing"},
		func(r contracts.Record[map[string]any]) contracts.Event {
			return contracts.Event{
				EventID:       "evt-1",
				Name:          "FormUpdated",
				Source:        "request-notification-service",
				TraceID:       "trace-1",
				SchemaVersion: 1,
				Data:          r.Data,
			}
		})
	if err != nil {
		t.Fatal(err)
	}
	if rec.ID != "form100001" || rec.Data["status"] != "Processing" {
		t.Fatalf("upsert form = %#v", rec)
	}
	if !db.tx.committed {
		t.Fatal("transaction not committed")
	}
	queries := strings.Join(db.tx.queries, "\n")
	if !strings.Contains(queries, "UPDATE forms") || strings.Contains(queries, "platform_records") {
		t.Fatalf("upsert-with-event queries = %s, want forms table in-tx", queries)
	}
}

func TestPostgresStoreRoutesAnnouncementsToOwnedTable(t *testing.T) {
	now := time.Date(2026, 6, 22, 11, 0, 0, 0, time.UTC)
	later := now.Add(time.Minute)
	db := &fakePostgresDB{
		queryResults: []*fakePostgresRows{{
			rows: [][]any{
				{"ann100001", []byte(`{"id":"ann100001","priority":"info","is_pinned":false}`), 1, now, now},
			},
		}},
		queryRows: []*fakePostgresRow{{
			values: []any{
				"ann100001",
				[]byte(`{"id":"ann100001","priority":"critical","is_pinned":true,"content":"kept"}`),
				2,
				now,
				later,
			},
		}},
		execTags: []pgconn.CommandTag{pgconn.NewCommandTag("DELETE 1")},
	}
	store := &PostgresStore{db: db}
	ctx := context.Background()

	if records := store.List(ctx, requestNotificationAnnouncementsResource); len(records) != 1 {
		t.Fatalf("announcements list = %#v", records)
	}
	updated, ok := store.Update(ctx, requestNotificationAnnouncementsResource, "ann100001", map[string]any{
		"priority":     "critical",
		"is_pinned":    true,
		"published_at": now.Format(time.RFC3339),
		"expires_at":   later.Format(time.RFC3339),
	})
	if !ok || updated.Data["content"] != "kept" {
		t.Fatalf("announcement update = %#v ok=%v", updated, ok)
	}
	if !store.Delete(ctx, requestNotificationAnnouncementsResource, "ann100001") {
		t.Fatal("announcement delete returned false")
	}
	queries := strings.Join(db.queries, "\n")
	for _, want := range []string{"FROM announcements", "UPDATE announcements", "DELETE FROM announcements"} {
		if !strings.Contains(queries, want) {
			t.Fatalf("announcement queries = %s, want %s", queries, want)
		}
	}
	if strings.Contains(queries, "platform_records") {
		t.Fatalf("announcement queries leaked platform_records: %s", queries)
	}
}

func TestPostgresStoreRoutesAnnouncementReadsAndNotificationsToOwnedTables(t *testing.T) {
	now := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)
	db := &fakePostgresDB{
		queryRows: []*fakePostgresRow{
			{values: []any{"ann1|US1", []byte(`{"id":"ann1|US1","announcement_id":"ann1","user_id":"US1"}`), 1, now, now}},
			{values: []any{"US1|n1", []byte(`{"id":"US1|n1","user_id":"US1","notification_id":"n1","read":true}`), 1, now, now}},
		},
	}
	store := &PostgresStore{db: db}
	ctx := context.Background()

	if _, err := store.Create(ctx, requestNotificationAnnouncementReadsResource, map[string]any{
		"id":              "ann1|US1",
		"announcement_id": "ann1",
		"user_id":         "US1",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(ctx, requestNotificationNotificationsResource, map[string]any{
		"id":              "US1|n1",
		"user_id":         "US1",
		"notification_id": "n1",
		"read":            true,
	}); err != nil {
		t.Fatal(err)
	}
	queries := strings.Join(db.queries, "\n")
	for _, want := range []string{"INSERT INTO announcement_reads", "INSERT INTO notifications"} {
		if !strings.Contains(queries, want) {
			t.Fatalf("queries = %s, want %s", queries, want)
		}
	}
	if strings.Contains(queries, "platform_records") {
		t.Fatalf("messaging queries leaked platform_records: %s", queries)
	}
}

func TestRequestNotificationAnnouncementColumnsPromoteTypesAndNulls(t *testing.T) {
	published := time.Date(2026, 6, 22, 8, 0, 0, 0, time.UTC)
	insert := columnValueMap(requestNotificationAnnouncementInsertColumns(map[string]any{
		"is_pinned":    true,
		"published_at": published.Format(time.RFC3339),
		"createdBy":    "US1",
	}, "ann1", time.Now()))
	if insert["priority"] != "info" { // defaulted when absent
		t.Fatalf("priority = %#v, want info", insert["priority"])
	}
	if insert["is_pinned"] != true {
		t.Fatalf("is_pinned = %#v, want true", insert["is_pinned"])
	}
	if insert["expires_at"] != nil { // absent nullable time stays NULL
		t.Fatalf("expires_at = %#v, want nil", insert["expires_at"])
	}
	if insert["created_by"] != "US1" { // camelCase alias resolved
		t.Fatalf("created_by = %#v, want US1", insert["created_by"])
	}

	// Update only emits columns present in the patch.
	update := columnValueMap(requestNotificationAnnouncementUpdateColumns(map[string]any{"priority": "critical"}))
	if len(update) != 1 || update["priority"] != "critical" {
		t.Fatalf("announcement update columns = %#v, want only priority", update)
	}
}

func TestRequestNotificationAnnouncementReadColumnsPromoteAliases(t *testing.T) {
	insert := columnValueMap(requestNotificationAnnouncementReadInsertColumns(map[string]any{
		"announcementId": "ann1",
		"user_id":        "US1",
	}, "ann1|US1", time.Now()))
	if insert["announcement_id"] != "ann1" || insert["user_id"] != "US1" {
		t.Fatalf("announcement_read insert columns = %#v", insert)
	}
	update := columnValueMap(requestNotificationAnnouncementReadUpdateColumns(map[string]any{"user_id": "US2"}))
	if len(update) != 1 || update["user_id"] != "US2" {
		t.Fatalf("announcement_read update columns = %#v, want only user_id", update)
	}
}

func TestRequestNotificationNotificationColumnsPromoteRead(t *testing.T) {
	insert := columnValueMap(requestNotificationNotificationInsertColumns(map[string]any{
		"userId":         "US1",
		"notificationId": "n1",
		"read":           true,
	}, "US1|n1", time.Now()))
	if insert["user_id"] != "US1" || insert["notification_id"] != "n1" || insert["read"] != true {
		t.Fatalf("notification insert columns = %#v", insert)
	}
	update := columnValueMap(requestNotificationNotificationUpdateColumns(map[string]any{"read": true}))
	if len(update) != 1 || update["read"] != true {
		t.Fatalf("notification update columns = %#v, want only read", update)
	}
}

func columnValueMap(values []identityColumnValue) map[string]any {
	out := map[string]any{}
	for _, value := range values {
		out[value.column] = value.value
	}
	if len(out) != len(values) {
		panic(fmt.Sprintf("duplicate column in %#v", values))
	}
	return out
}
