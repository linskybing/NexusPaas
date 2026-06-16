package requestnotification

import (
	"log/slog"
	"net/http"
	"sort"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

func publishFormEvent(app *platform.App, r *http.Request, name string, form Form) {
	if app == nil || app.Events == nil {
		return
	}
	traceID := platform.TraceID(r)
	if traceID == "" {
		traceID = platform.NewUUID()
	}
	if err := app.Events.Publish(r.Context(), contracts.Event{
		EventID:        platform.NewUUID(),
		Name:           name,
		Source:         serviceName,
		OccurredAt:     time.Now().UTC(),
		TraceID:        traceID,
		SchemaVersion:  1,
		IdempotencyKey: r.Header.Get("Idempotency-Key"),
		Data:           formToMap(&form),
	}); err != nil {
		slog.Error("form event publish failed", "event", name, "form_id", form.ID, "error", err)
	}
}

func requireStore(app *platform.App) (platform.RecordStore, int, any, bool) {
	if app == nil || app.Store == nil {
		return nil, http.StatusServiceUnavailable, shared.ErrorData("request notification store unavailable"), false
	}
	return app.Store, 0, nil, true
}

func listForms(r *http.Request, store platform.RecordStore) []Form {
	out := []Form{}
	for _, record := range store.List(r.Context(), formsResource) {
		if form, ok := formFromMap(record.Data); ok {
			out = append(out, form)
		}
	}
	return out
}

func getFormByID(r *http.Request, store platform.RecordStore, id string) (Form, bool) {
	record, ok := store.Get(r.Context(), formsResource, id)
	if !ok {
		return Form{}, false
	}
	return formFromMap(record.Data)
}

func formToMap(f *Form) map[string]any {
	data := map[string]any{
		"id":          f.ID,
		"user_id":     f.UserID,
		"title":       f.Title,
		"description": f.Description,
		"tag":         f.Tag,
		"status":      f.Status,
		"created_at":  f.CreatedAt.Format(time.RFC3339),
		"updated_at":  f.UpdatedAt.Format(time.RFC3339),
	}
	if f.ProjectID != nil {
		data["project_id"] = *f.ProjectID
	}
	return data
}

func formFromMap(data map[string]any) (Form, bool) {
	createdAt, ok := timeValue(data, "created_at")
	if !ok {
		return Form{}, false
	}
	updatedAt, ok := timeValue(data, "updated_at")
	if !ok {
		return Form{}, false
	}
	form := Form{
		ID:          valueFrom(data, "id"),
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
		UserID:      valueFrom(data, "user_id", "userId"),
		Title:       valueFrom(data, "title"),
		Description: valueFrom(data, "description"),
		Tag:         valueFrom(data, "tag"),
		Status:      valueFrom(data, "status"),
	}
	if projectID := valueFrom(data, "project_id", "projectId"); projectID != "" {
		form.ProjectID = &projectID
	}
	return form, form.ID != ""
}

func messageToMap(m FormMessage) map[string]any {
	data := map[string]any{
		"id":         m.ID,
		"form_id":    m.FormID,
		"user_id":    m.UserID,
		"content":    m.Content,
		"created_at": m.CreatedAt.Format(time.RFC3339),
	}
	if m.User != nil {
		data["user"] = map[string]any{"id": m.User.ID, "username": m.User.Username, "system_role": m.User.SystemRole}
	}
	return data
}

func listMessagesForForm(r *http.Request, store platform.RecordStore, formID string) []FormMessage {
	out := []FormMessage{}
	for _, record := range store.List(r.Context(), formMessagesResource) {
		msg, ok := messageFromMap(record.Data)
		if ok && msg.FormID == formID {
			out = append(out, msg)
		}
	}
	return out
}

func messageFromMap(data map[string]any) (FormMessage, bool) {
	createdAt, ok := timeValue(data, "created_at")
	if !ok {
		return FormMessage{}, false
	}
	msg := FormMessage{
		ID:        valueFrom(data, "id"),
		FormID:    valueFrom(data, "form_id", "formId"),
		UserID:    valueFrom(data, "user_id", "userId"),
		Content:   valueFrom(data, "content"),
		CreatedAt: createdAt,
	}
	if user := senderFromAny(data["user"]); user != nil {
		msg.User = user
	}
	return msg, msg.ID != "" && msg.FormID != ""
}

func senderFromAny(value any) *MessageSender {
	raw, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	return &MessageSender{ID: valueFrom(raw, "id"), Username: valueFrom(raw, "username"), SystemRole: intValue(raw["system_role"])}
}

func announcementToMap(a *Announcement) map[string]any {
	data := map[string]any{
		"id":           a.ID,
		"title":        a.Title,
		"content":      a.Content,
		"priority":     a.Priority,
		"is_pinned":    a.IsPinned,
		"published_at": a.PublishedAt.Format(time.RFC3339),
		"created_by":   a.CreatedBy,
		"creator_name": a.CreatorName,
		"created_at":   a.CreatedAt.Format(time.RFC3339),
		"updated_at":   a.UpdatedAt.Format(time.RFC3339),
	}
	if a.ExpiresAt != nil {
		data["expires_at"] = a.ExpiresAt.Format(time.RFC3339)
	}
	return data
}

func listAnnouncementsFromStore(r *http.Request, store platform.RecordStore) []Announcement {
	out := []Announcement{}
	for _, record := range store.List(r.Context(), announcementsResource) {
		if announcement, ok := announcementFromMap(record.Data); ok {
			out = append(out, announcement)
		}
	}
	return out
}

func getAnnouncementByID(r *http.Request, store platform.RecordStore, id string) (Announcement, bool) {
	record, ok := store.Get(r.Context(), announcementsResource, id)
	if !ok {
		return Announcement{}, false
	}
	return announcementFromMap(record.Data)
}

func announcementFromMap(data map[string]any) (Announcement, bool) {
	publishedAt, ok := timeValue(data, "published_at")
	if !ok {
		return Announcement{}, false
	}
	createdAt, ok := timeValue(data, "created_at")
	if !ok {
		return Announcement{}, false
	}
	updatedAt, ok := timeValue(data, "updated_at")
	if !ok {
		return Announcement{}, false
	}
	expiresAt, ok := optionalStoredTime(data["expires_at"])
	if !ok {
		return Announcement{}, false
	}
	announcement := Announcement{
		ID:          valueFrom(data, "id"),
		Title:       valueFrom(data, "title"),
		Content:     valueFrom(data, "content"),
		Priority:    normalizedPriority(valueFrom(data, "priority")),
		IsPinned:    boolValue(data["is_pinned"]),
		PublishedAt: publishedAt,
		ExpiresAt:   expiresAt,
		CreatedBy:   valueFrom(data, "created_by", "createdBy"),
		CreatorName: valueFrom(data, "creator_name", "creatorName"),
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
	}
	return announcement, announcement.ID != ""
}

func countPinned(r *http.Request, store platform.RecordStore, exceptID string) int {
	count := 0
	for _, announcement := range listAnnouncementsFromStore(r, store) {
		if announcement.ID != exceptID && announcement.IsPinned {
			count++
		}
	}
	return count
}

func announcementReadSet(r *http.Request, store platform.RecordStore, userID string) map[string]bool {
	out := map[string]bool{}
	for _, record := range store.List(r.Context(), announcementReadsResource) {
		if valueFrom(record.Data, "user_id", "userId") == userID {
			out[valueFrom(record.Data, "announcement_id", "announcementId")] = true
		}
	}
	return out
}

func upsertAnnouncementRead(r *http.Request, store platform.RecordStore, announcementID, userID string, readAt time.Time) bool {
	id := announcementReadID(announcementID, userID)
	data := map[string]any{
		"id":              id,
		"announcement_id": announcementID,
		"user_id":         userID,
		"read_at":         readAt.Format(time.RFC3339),
	}
	if _, ok := store.Update(r.Context(), announcementReadsResource, id, data); ok {
		return true
	}
	if _, err := store.Create(r.Context(), announcementReadsResource, data); err != nil {
		if platform.IsCreateConflict(err) {
			_, ok := store.Update(r.Context(), announcementReadsResource, id, data)
			return ok
		}
		slog.Error("announcement read create failed", "announcement_id", announcementID, "user_id", userID, "error", err)
		return false
	}
	return true
}

func deleteAnnouncementReads(r *http.Request, store platform.RecordStore, announcementID string) {
	for _, record := range store.List(r.Context(), announcementReadsResource) {
		if valueFrom(record.Data, "announcement_id", "announcementId") == announcementID {
			store.Delete(r.Context(), announcementReadsResource, record.ID)
		}
	}
}

func announcementReadID(announcementID, userID string) string {
	return announcementID + "|" + userID
}

func notificationToMap(n Notification) map[string]any {
	data := map[string]any{
		"id":              notificationRecordID(n.UserID, n.ID),
		"notification_id": n.ID,
		"user_id":         n.UserID,
		"read":            n.Read,
	}
	if n.ReadAt != nil {
		data["read_at"] = n.ReadAt.Format(time.RFC3339)
	}
	return data
}

func listNotificationsForUser(r *http.Request, store platform.RecordStore, userID string) []Notification {
	out := []Notification{}
	for _, record := range store.List(r.Context(), notificationsResource) {
		notification, ok := notificationFromMap(record.Data)
		if ok && notification.UserID == userID {
			out = append(out, notification)
		}
	}
	return out
}

func notificationFromMap(data map[string]any) (Notification, bool) {
	readAt, ok := optionalStoredTime(data["read_at"])
	if !ok {
		return Notification{}, false
	}
	notification := Notification{
		ID:     valueFrom(data, "notification_id", "notificationId", "id"),
		UserID: valueFrom(data, "user_id", "userId"),
		Read:   boolValue(data["read"]),
		ReadAt: readAt,
	}
	return notification, notification.ID != "" && notification.UserID != ""
}

func upsertNotification(r *http.Request, store platform.RecordStore, notification Notification) bool {
	id := notificationRecordID(notification.UserID, notification.ID)
	data := notificationToMap(notification)
	if _, ok := store.Update(r.Context(), notificationsResource, id, data); ok {
		return true
	}
	if _, err := store.Create(r.Context(), notificationsResource, data); err != nil {
		if platform.IsCreateConflict(err) {
			_, ok := store.Update(r.Context(), notificationsResource, id, data)
			return ok
		}
		slog.Error("notification upsert failed", "notification_id", notification.ID, "user_id", notification.UserID, "error", err)
		return false
	}
	return true
}

func clearNotificationsForUser(r *http.Request, store platform.RecordStore, userID string) {
	for _, record := range store.List(r.Context(), notificationsResource) {
		if valueFrom(record.Data, "user_id", "userId") == userID {
			store.Delete(r.Context(), notificationsResource, record.ID)
		}
	}
}

func notificationRecordID(userID, notificationID string) string {
	return userID + "|" + notificationID
}

func timeValue(data map[string]any, key string) (time.Time, bool) {
	return parseStoredTime(data[key])
}

func optionalStoredTime(value any) (*time.Time, bool) {
	if value == nil || asText(value) == "" {
		return nil, true
	}
	parsed, ok := parseStoredTime(value)
	if !ok {
		return nil, false
	}
	return &parsed, true
}

func parseStoredTime(value any) (time.Time, bool) {
	switch typed := value.(type) {
	case time.Time:
		return typed, true
	case string:
		parsed, err := time.Parse(time.RFC3339, typed)
		return parsed, err == nil
	default:
		return time.Time{}, false
	}
}

func boolValue(value any) bool {
	parsed, _ := value.(bool)
	return parsed
}

func intValue(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
}

func cloneForm(form *Form) Form {
	out := *form
	return out
}

func cloneAnnouncement(a *Announcement) Announcement {
	out := *a
	return out
}

func sortForms(forms []Form) {
	sort.Slice(forms, func(i, j int) bool { return forms[i].CreatedAt.Before(forms[j].CreatedAt) })
}

func sortAnnouncements(items []Announcement) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].IsPinned != items[j].IsPinned {
			return items[i].IsPinned
		}
		return items[i].PublishedAt.After(items[j].PublishedAt)
	})
}
