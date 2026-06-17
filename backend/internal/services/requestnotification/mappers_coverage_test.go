package requestnotification

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestRequestNotificationMapperHelperEdges(t *testing.T) {
	now := time.Date(2026, time.June, 17, 12, 0, 0, 0, time.UTC)
	later := now.Add(time.Hour)

	forms := []Form{
		{ID: "late", CreatedAt: later},
		{ID: "early", CreatedAt: now},
	}
	sortForms(forms)
	if forms[0].ID != "early" || forms[1].ID != "late" {
		t.Fatalf("sortForms = %#v, want oldest first", forms)
	}

	announcements := []Announcement{
		{ID: "old", PublishedAt: now},
		{ID: "pinned", IsPinned: true, PublishedAt: now},
		{ID: "new", PublishedAt: later},
	}
	sortAnnouncements(announcements)
	if announcements[0].ID != "pinned" || announcements[1].ID != "new" || announcements[2].ID != "old" {
		t.Fatalf("sortAnnouncements = %#v, want pinned then newest first", announcements)
	}

	if got := intValue(int64(42)); got != 42 {
		t.Fatalf("intValue int64 = %d, want 42", got)
	}
	if got := intValue(float64(7)); got != 7 {
		t.Fatalf("intValue float64 = %d, want 7", got)
	}
	if parsed, ok := parseStoredTime(now); !ok || !parsed.Equal(now) {
		t.Fatalf("parseStoredTime time = %v ok=%v, want now", parsed, ok)
	}
	if _, ok := parseStoredTime(123); ok {
		t.Fatal("parseStoredTime unsupported value succeeded unexpectedly")
	}
	if ptr, ok := optionalStoredTime("bad-time"); ok || ptr != nil {
		t.Fatalf("optionalStoredTime bad = %v ok=%v, want nil false", ptr, ok)
	}
}

func TestRequestNotificationReadUpsertsAndDeletes(t *testing.T) {
	store := platform.NewStore()
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	now := time.Date(2026, time.June, 17, 12, 0, 0, 0, time.UTC)

	if !upsertAnnouncementRead(req, store, "A1", "U1", now) {
		t.Fatal("upsertAnnouncementRead create failed")
	}
	if !upsertAnnouncementRead(req, store, "A1", "U1", now.Add(time.Minute)) {
		t.Fatal("upsertAnnouncementRead update failed")
	}
	reads := announcementReadSet(req, store, "U1")
	if !reads["A1"] {
		t.Fatalf("announcementReadSet = %#v, want A1", reads)
	}
	deleteAnnouncementReads(req, store, "A1")
	if got := announcementReadSet(req, store, "U1"); len(got) != 0 {
		t.Fatalf("announcement reads after delete = %#v, want empty", got)
	}

	readAt := now
	if !upsertNotification(req, store, Notification{ID: "N1", UserID: "U1", Read: true, ReadAt: &readAt}) {
		t.Fatal("upsertNotification create failed")
	}
	if !upsertNotification(req, store, Notification{ID: "N1", UserID: "U1", Read: false}) {
		t.Fatal("upsertNotification update failed")
	}
	notifications := listNotificationsForUser(req, store, "U1")
	if len(notifications) != 1 || notifications[0].ID != "N1" || notifications[0].Read {
		t.Fatalf("notifications = %#v, want updated unread N1", notifications)
	}
	clearNotificationsForUser(req, store, "U1")
	if got := listNotificationsForUser(req, store, "U1"); len(got) != 0 {
		t.Fatalf("notifications after clear = %#v, want empty", got)
	}
}

func TestRequestNotificationProjectAccessProjectionDeleteNoops(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	store := platform.NewStore()
	repo := projectAccessRepoFromStore(store, platform.Config{ServiceName: serviceName})

	if deleteProjectAccessReadModel(repo, req, "unknown", map[string]any{"id": "x"}) {
		t.Fatal("deleteProjectAccessReadModel unknown resource = true, want false")
	}
	if err := upsertProjectAccessReadModel(repo, req, "unknown", map[string]any{"id": "x"}); err != nil {
		t.Fatalf("upsertProjectAccessReadModel unknown resource error = %v, want nil", err)
	}
	if got := repo.ListProjects(req.Context()); len(got) != 0 {
		t.Fatalf("unknown projection mutated projects = %#v", got)
	}
}
