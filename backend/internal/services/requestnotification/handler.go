package requestnotification

import (
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

const (
	serviceName = "request-notification-service"
	// These resources are owned by request-notification-service. Keeping the
	// source of truth behind RecordStore makes configured runtimes durable and
	// shared across service replicas instead of relying on process-local maps.
	formsResource             = serviceName + ":forms"
	formMessagesResource      = serviceName + ":form_messages"
	announcementsResource     = serviceName + ":announcements"
	announcementReadsResource = serviceName + ":announcement_reads"
	notificationsResource     = serviceName + ":notifications"
	projectAccessConsumer     = serviceName + ":project_access_projection"
	projectAccessMembers      = serviceName + ":project_access_members"
	projectAccessProjects     = serviceName + ":project_access_projects"
	projectAccessUserGroups   = serviceName + ":project_access_user_groups"
	orgProjectMembersResource = "org-project-service:project_members"
	orgProjectsResource       = "org-project-service:projects"
	orgUserGroupsResource     = "org-project-service:user_groups"

	msgInvalidRequestBody   = "invalid request body"
	msgFormNotFound         = "Form not found"
	msgFormAccessDenied     = "form access denied"
	msgAnnouncementNotFound = "announcement not found"
)

type Service struct{}

type Form struct {
	ID          string    `json:"id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	UserID      string    `json:"user_id"`
	ProjectID   *string   `json:"project_id,omitempty"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Tag         string    `json:"tag"`
	Status      string    `json:"status"`
}

type FormMessage struct {
	ID        string         `json:"id"`
	FormID    string         `json:"form_id"`
	UserID    string         `json:"user_id"`
	Content   string         `json:"content"`
	CreatedAt time.Time      `json:"created_at"`
	User      *MessageSender `json:"user,omitempty"`
}

type MessageSender struct {
	ID         string `json:"id"`
	Username   string `json:"username"`
	SystemRole int    `json:"system_role"`
}

type Announcement struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Content     string     `json:"content"`
	Priority    string     `json:"priority"`
	IsPinned    bool       `json:"is_pinned"`
	PublishedAt time.Time  `json:"published_at"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	CreatedBy   string     `json:"created_by"`
	CreatorName string     `json:"creator_name,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type ActiveAnnouncement struct {
	Announcement
	IsRead bool `json:"is_read"`
}

type Notification struct {
	ID     string     `json:"id"`
	UserID string     `json:"user_id"`
	Read   bool       `json:"read"`
	ReadAt *time.Time `json:"read_at,omitempty"`
}

type userContext struct {
	ID         string
	Username   string
	Admin      bool
	SystemRole int
}

func Register(app *platform.App) {
	svc := NewService()
	for _, route := range []struct {
		method  string
		pattern string
		handler platform.HandlerFunc
	}{
		{http.MethodPost, "/api/v1/forms", svc.createForm},
		{http.MethodGet, "/api/v1/forms", svc.listAllForms},
		{http.MethodGet, "/api/v1/forms/my", svc.listMyForms},
		{http.MethodGet, "/api/v1/forms/{id}", svc.getForm},
		{http.MethodPut, "/api/v1/forms/{id}", svc.updateFormStatus},
		{http.MethodPut, "/api/v1/forms/{id}/status", svc.updateFormStatus},
		{http.MethodPut, "/api/v1/forms/batch/status", svc.batchUpdateFormStatus},
		{http.MethodPost, "/api/v1/forms/{id}/messages", svc.createMessage},
		{http.MethodGet, "/api/v1/forms/{id}/messages", svc.listMessages},
		{http.MethodPost, "/api/v1/admin/announcements", svc.createAnnouncement},
		{http.MethodGet, "/api/v1/admin/announcements", svc.listAnnouncements},
		{http.MethodPut, "/api/v1/admin/announcements/{id}", svc.updateAnnouncement},
		{http.MethodDelete, "/api/v1/admin/announcements/{id}", svc.deleteAnnouncement},
		{http.MethodGet, "/api/v1/announcements/active", svc.listActiveAnnouncements},
		{http.MethodGet, "/api/v1/announcements/unread-count", svc.unreadCount},
		{http.MethodGet, "/api/v1/announcements/{id}", svc.getAnnouncement},
		{http.MethodPut, "/api/v1/announcements/{id}/read", svc.markAnnouncementRead},
		{http.MethodPut, "/api/v1/notifications/{id}/read", svc.markNotificationRead},
		{http.MethodPut, "/api/v1/notifications/read-all", svc.markAllNotificationsRead},
		{http.MethodDelete, "/api/v1/notifications/clear-all", svc.clearNotifications},
	} {
		app.RegisterCustomHandler(route.method, route.pattern, route.handler)
	}
	registerProjectAccessProjectionReconciler(app)
}

func NewService() *Service {
	return &Service{}
}

func (s *Service) createForm(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	user, code, data, ok := requireUser(r)
	if !ok {
		return code, data, nil
	}
	payload, err := platform.DecodeMapWithError(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(msgInvalidRequestBody), nil
	}
	title := asText(payload["title"])
	description := asText(payload["description"])
	tag := asText(payload["tag"])
	if title == "" || description == "" {
		return http.StatusBadRequest, shared.ErrorData("title and description are required"), nil
	}
	if tag != "" && !validTag(tag) {
		return http.StatusBadRequest, shared.ErrorData("invalid tag"), nil
	}
	projectID := optionalString(payload["project_id"])
	if projectID != nil && !s.canCreateProjectForm(app, r, user.ID, *projectID, user.Admin) {
		return http.StatusForbidden, shared.ErrorData("project membership required to create a project-linked form"), nil
	}
	store, code, data, ok := requireStore(app)
	if !ok {
		return code, data, nil
	}
	now := time.Now().UTC()
	form := &Form{
		ID:          store.NextID(formsResource, "form", 100001, 0),
		CreatedAt:   now,
		UpdatedAt:   now,
		UserID:      user.ID,
		ProjectID:   projectID,
		Title:       title,
		Description: description,
		Tag:         tag,
		Status:      "Pending",
	}
	if _, err := app.CreateRecordWithEvent(r.Context(), formsResource, formToMap(form), func(contracts.Record[map[string]any]) contracts.Event {
		return formEvent(r, "FormCreated", *form)
	}); err != nil {
		slog.Error("form create failed", "form_id", form.ID, "error", err)
		return http.StatusInternalServerError, shared.ErrorData("form could not be created"), nil
	}
	return http.StatusCreated, cloneForm(form), nil
}

func (s *Service) listMyForms(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	user, code, data, ok := requireUser(r)
	if !ok {
		return code, data, nil
	}
	store, code, data, ok := requireStore(app)
	if !ok {
		return code, data, nil
	}
	out := []Form{}
	for _, form := range listForms(r, store) {
		if user.Admin || form.UserID == user.ID {
			out = append(out, form)
		}
	}
	sortForms(out)
	if r.URL.Query().Get("page") != "" {
		return http.StatusOK, paginateForms(out, r, 500), nil
	}
	return http.StatusOK, out, nil
}

func (s *Service) listAllForms(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	if _, code, data, ok := requireAdmin(r); !ok {
		return code, data, nil
	}
	store, code, data, ok := requireStore(app)
	if !ok {
		return code, data, nil
	}
	out := listForms(r, store)
	sortForms(out)
	if r.URL.Query().Get("page") != "" {
		return http.StatusOK, paginateForms(out, r, 500), nil
	}
	return http.StatusOK, out, nil
}

func (s *Service) getForm(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	id := r.PathValue("id")
	user, code, data, ok := requireUser(r)
	if !ok {
		return code, data, nil
	}
	store, code, data, ok := requireStore(app)
	if !ok {
		return code, data, nil
	}
	form, found := getFormByID(r, store, id)
	if !found {
		return http.StatusNotFound, shared.ErrorData(msgFormNotFound), nil
	}
	if form.UserID != user.ID && !user.Admin {
		return http.StatusForbidden, shared.ErrorData(msgFormAccessDenied), nil
	}
	return http.StatusOK, form, nil
}

func (s *Service) updateFormStatus(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	if _, code, data, ok := requireAdmin(r); !ok {
		return code, data, nil
	}
	payload, err := platform.DecodeMapWithError(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(msgInvalidRequestBody), nil
	}
	status := asText(payload["status"])
	return s.transitionForm(app, r, r.PathValue("id"), status)
}

func (s *Service) batchUpdateFormStatus(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	if _, code, data, ok := requireAdmin(r); !ok {
		return code, data, nil
	}
	payload, err := platform.DecodeMapWithError(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(msgInvalidRequestBody), nil
	}
	status := asText(payload["status"])
	ids, ok := stringSlice(payload["ids"])
	if !ok || len(ids) == 0 || status == "" {
		return http.StatusBadRequest, shared.ErrorData("ids and status are required"), nil
	}
	result := map[string]any{"succeeded": 0, "failed": 0, "errors": []string{}}
	for _, id := range ids {
		code, data, _ := s.transitionForm(app, r, id, status)
		if code >= 400 {
			result["failed"] = result["failed"].(int) + 1
			result["errors"] = append(result["errors"].([]string), asText(data.(map[string]any)["message"]))
			continue
		}
		result["succeeded"] = result["succeeded"].(int) + 1
	}
	return http.StatusOK, result, nil
}

func (s *Service) transitionForm(app *platform.App, r *http.Request, id, status string) (int, any, *platform.Degraded) {
	if !validStatus(status) {
		return http.StatusBadRequest, shared.ErrorData("invalid status"), nil
	}
	store, code, data, ok := requireStore(app)
	if !ok {
		return code, data, nil
	}
	form, found := getFormByID(r, store, id)
	if !found {
		return http.StatusNotFound, shared.ErrorData(msgFormNotFound), nil
	}
	if !validTransition(form.Status, status) {
		return http.StatusBadRequest, shared.ErrorData("invalid transition from " + form.Status + " to " + status), nil
	}
	form.Status = status
	form.UpdatedAt = time.Now().UTC()
	if _, updated, err := app.UpdateRecordWithEvent(r.Context(), formsResource, id, formToMap(&form), func(contracts.Record[map[string]any]) contracts.Event {
		return formEvent(r, "FormUpdated", form)
	}); err != nil || !updated {
		return http.StatusNotFound, shared.ErrorData(msgFormNotFound), nil
	}
	return http.StatusOK, form, nil
}

func (s *Service) createMessage(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	user, code, data, ok := requireUser(r)
	if !ok {
		return code, data, nil
	}
	payload, err := platform.DecodeMapWithError(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(msgInvalidRequestBody), nil
	}
	content := asText(payload["content"])
	if content == "" {
		return http.StatusBadRequest, shared.ErrorData("content is required"), nil
	}
	store, code, data, ok := requireStore(app)
	if !ok {
		return code, data, nil
	}
	formID := r.PathValue("id")
	form, found := getFormByID(r, store, formID)
	if !found {
		return http.StatusNotFound, shared.ErrorData(msgFormNotFound), nil
	}
	if form.UserID != user.ID && !user.Admin {
		return http.StatusForbidden, shared.ErrorData(msgFormAccessDenied), nil
	}
	if form.Status == "Completed" {
		return http.StatusBadRequest, shared.ErrorData("cannot add message to completed form"), nil
	}
	msg := FormMessage{
		ID:        store.NextID(formMessagesResource, "msg", 100001, 0),
		FormID:    formID,
		UserID:    user.ID,
		Content:   content,
		CreatedAt: time.Now().UTC(),
		User:      &MessageSender{ID: user.ID, Username: user.Username, SystemRole: user.SystemRole},
	}
	if _, err := store.Create(r.Context(), formMessagesResource, messageToMap(msg)); err != nil {
		slog.Error("form message create failed", "form_id", formID, "message_id", msg.ID, "error", err)
		return http.StatusInternalServerError, shared.ErrorData("message could not be created"), nil
	}
	return http.StatusOK, msg, nil
}

func (s *Service) listMessages(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	formID := r.PathValue("id")
	user, code, data, ok := requireUser(r)
	if !ok {
		return code, data, nil
	}
	store, code, data, ok := requireStore(app)
	if !ok {
		return code, data, nil
	}
	form, found := getFormByID(r, store, formID)
	if !found {
		return http.StatusNotFound, shared.ErrorData(msgFormNotFound), nil
	}
	if form.UserID != user.ID && !user.Admin {
		return http.StatusForbidden, shared.ErrorData(msgFormAccessDenied), nil
	}
	out := listMessagesForForm(r, store, formID)
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return http.StatusOK, out, nil
}

func (s *Service) createAnnouncement(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	user, code, data, ok := requireAdmin(r)
	if !ok {
		return code, data, nil
	}
	payload, err := platform.DecodeMapWithError(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(msgInvalidRequestBody), nil
	}
	title := asText(payload["title"])
	content := asText(payload["content"])
	if title == "" || content == "" {
		return http.StatusBadRequest, shared.ErrorData("title and content are required"), nil
	}
	if len(title) > 200 {
		return http.StatusBadRequest, shared.ErrorData("title must be 200 characters or fewer"), nil
	}
	priority := normalizedPriority(asText(payload["priority"]))
	isPinned := asBool(payload["is_pinned"])
	expiresAt, ok := optionalTime(payload["expires_at"])
	if !ok {
		return http.StatusBadRequest, shared.ErrorData("invalid expires_at"), nil
	}
	store, code, data, ok := requireStore(app)
	if !ok {
		return code, data, nil
	}
	now := time.Now().UTC()
	if isPinned && countPinned(r, store, "") >= 3 {
		return http.StatusBadRequest, shared.ErrorData("cannot pin more than 3 announcements"), nil
	}
	announcement := &Announcement{
		ID:          store.NextID(announcementsResource, "ann", 100001, 0),
		Title:       title,
		Content:     content,
		Priority:    priority,
		IsPinned:    isPinned,
		PublishedAt: now,
		ExpiresAt:   expiresAt,
		CreatedBy:   user.ID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if _, err := store.Create(r.Context(), announcementsResource, announcementToMap(announcement)); err != nil {
		slog.Error("announcement create failed", "announcement_id", announcement.ID, "error", err)
		return http.StatusInternalServerError, shared.ErrorData("announcement could not be created"), nil
	}
	return http.StatusCreated, cloneAnnouncement(announcement), nil
}

func (s *Service) updateAnnouncement(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	if _, code, data, ok := requireAdmin(r); !ok {
		return code, data, nil
	}
	payload, err := platform.DecodeMapWithError(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(msgInvalidRequestBody), nil
	}
	title := asText(payload["title"])
	if title == "" {
		return http.StatusBadRequest, shared.ErrorData("title is required"), nil
	}
	if len(title) > 200 {
		return http.StatusBadRequest, shared.ErrorData("title must be 200 characters or fewer"), nil
	}
	id := r.PathValue("id")
	store, code, data, ok := requireStore(app)
	if !ok {
		return code, data, nil
	}
	current, found := getAnnouncementByID(r, store, id)
	if !found {
		return http.StatusNotFound, shared.ErrorData(msgAnnouncementNotFound), nil
	}
	isPinned := asBool(payload["is_pinned"])
	if isPinned && !current.IsPinned && countPinned(r, store, id) >= 3 {
		return http.StatusBadRequest, shared.ErrorData("cannot pin more than 3 announcements"), nil
	}
	expiresAt, ok := optionalTime(payload["expires_at"])
	if !ok {
		return http.StatusBadRequest, shared.ErrorData("invalid expires_at"), nil
	}
	current.Title = title
	current.Content = asText(payload["content"])
	current.Priority = normalizedPriority(asText(payload["priority"]))
	current.IsPinned = isPinned
	current.ExpiresAt = expiresAt
	current.UpdatedAt = time.Now().UTC()
	if _, updated := store.Update(r.Context(), announcementsResource, id, announcementToMap(&current)); !updated {
		return http.StatusNotFound, shared.ErrorData(msgAnnouncementNotFound), nil
	}
	return http.StatusOK, current, nil
}

func (s *Service) deleteAnnouncement(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	if _, code, data, ok := requireAdmin(r); !ok {
		return code, data, nil
	}
	id := r.PathValue("id")
	store, code, data, ok := requireStore(app)
	if !ok {
		return code, data, nil
	}
	if !store.Delete(r.Context(), announcementsResource, id) {
		return http.StatusNotFound, shared.ErrorData(msgAnnouncementNotFound), nil
	}
	deleteAnnouncementReads(r, store, id)
	return http.StatusOK, map[string]any{"deleted": true}, nil
}

func (s *Service) listAnnouncements(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	if _, code, data, ok := requireAdmin(r); !ok {
		return code, data, nil
	}
	page := parsePositiveInt(r.URL.Query().Get("page"), 1)
	limit := parsePositiveInt(r.URL.Query().Get("limit"), 20)
	if limit > 100 {
		limit = 20
	}
	store, code, data, ok := requireStore(app)
	if !ok {
		return code, data, nil
	}
	search := strings.ToLower(r.URL.Query().Get("search"))
	priority := r.URL.Query().Get("priority")
	items := []Announcement{}
	for _, a := range listAnnouncementsFromStore(r, store) {
		if priority != "" && a.Priority != priority {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(a.Title+" "+a.Content), search) {
			continue
		}
		items = append(items, a)
	}
	sortAnnouncements(items)
	total := len(items)
	start := (page - 1) * limit
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}
	return http.StatusOK, map[string]any{"list": items[start:end], "total": total, "page": page, "page_size": limit}, nil
}

func (s *Service) listActiveAnnouncements(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	user, code, data, ok := requireUser(r)
	if !ok {
		return code, data, nil
	}
	store, code, data, ok := requireStore(app)
	if !ok {
		return code, data, nil
	}
	now := time.Now().UTC()
	out := []ActiveAnnouncement{}
	read := announcementReadSet(r, store, user.ID)
	for _, a := range listAnnouncementsFromStore(r, store) {
		if !isActive(a, now) {
			continue
		}
		out = append(out, ActiveAnnouncement{Announcement: a, IsRead: read[a.ID]})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].IsPinned != out[j].IsPinned {
			return out[i].IsPinned
		}
		return out[i].PublishedAt.After(out[j].PublishedAt)
	})
	return http.StatusOK, out, nil
}

func (s *Service) getAnnouncement(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	if _, code, data, ok := requireUser(r); !ok {
		return code, data, nil
	}
	id := r.PathValue("id")
	store, code, data, ok := requireStore(app)
	if !ok {
		return code, data, nil
	}
	now := time.Now().UTC()
	a, found := getAnnouncementByID(r, store, id)
	if !found || !isActive(a, now) {
		return http.StatusNotFound, shared.ErrorData(msgAnnouncementNotFound), nil
	}
	return http.StatusOK, a, nil
}

func (s *Service) markAnnouncementRead(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	id := r.PathValue("id")
	user, code, data, ok := requireUser(r)
	if !ok {
		return code, data, nil
	}
	store, code, data, ok := requireStore(app)
	if !ok {
		return code, data, nil
	}
	now := time.Now().UTC()
	a, found := getAnnouncementByID(r, store, id)
	if !found || !isActive(a, now) {
		return http.StatusNotFound, shared.ErrorData(msgAnnouncementNotFound), nil
	}
	if !upsertAnnouncementRead(r, store, id, user.ID, now) {
		return http.StatusInternalServerError, shared.ErrorData("announcement read state could not be saved"), nil
	}
	return http.StatusOK, nil, nil
}

func (s *Service) unreadCount(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	user, code, data, ok := requireUser(r)
	if !ok {
		return code, data, nil
	}
	store, code, data, ok := requireStore(app)
	if !ok {
		return code, data, nil
	}
	now := time.Now().UTC()
	read := announcementReadSet(r, store, user.ID)
	count := int64(0)
	for _, a := range listAnnouncementsFromStore(r, store) {
		if !isActive(a, now) {
			continue
		}
		if !read[a.ID] {
			count++
		}
	}
	return http.StatusOK, map[string]int64{"count": count}, nil
}

func (s *Service) markNotificationRead(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	id := r.PathValue("id")
	if id == "" {
		return http.StatusBadRequest, shared.ErrorData("Notification ID required"), nil
	}
	user, code, data, ok := requireUser(r)
	if !ok {
		return code, data, nil
	}
	store, code, data, ok := requireStore(app)
	if !ok {
		return code, data, nil
	}
	now := time.Now().UTC()
	if !upsertNotification(r, store, Notification{ID: id, UserID: user.ID, Read: true, ReadAt: &now}) {
		return http.StatusInternalServerError, shared.ErrorData("notification state could not be saved"), nil
	}
	return http.StatusOK, nil, nil
}

func (s *Service) markAllNotificationsRead(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	user, code, data, ok := requireUser(r)
	if !ok {
		return code, data, nil
	}
	store, code, data, ok := requireStore(app)
	if !ok {
		return code, data, nil
	}
	now := time.Now().UTC()
	for _, notification := range listNotificationsForUser(r, store, user.ID) {
		notification.Read = true
		notification.ReadAt = &now
		if !upsertNotification(r, store, notification) {
			return http.StatusInternalServerError, shared.ErrorData("notification state could not be saved"), nil
		}
	}
	return http.StatusOK, nil, nil
}

func (s *Service) clearNotifications(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	user, code, data, ok := requireUser(r)
	if !ok {
		return code, data, nil
	}
	store, code, data, ok := requireStore(app)
	if !ok {
		return code, data, nil
	}
	clearNotificationsForUser(r, store, user.ID)
	return http.StatusOK, nil, nil
}
