package platform

import "time"

const (
	requestNotificationFormsResource             = "request-notification-service:forms"
	requestNotificationFormMessagesResource      = "request-notification-service:form_messages"
	requestNotificationAnnouncementsResource     = "request-notification-service:announcements"
	requestNotificationAnnouncementReadsResource = "request-notification-service:announcement_reads"
	requestNotificationNotificationsResource     = "request-notification-service:notifications"
)

// requestNotificationPostgresResources maps request-notification-owned resources
// to their typed, service-owned tables (migration 0002). Like identity, the full
// record is kept in the payload JSONB column; the promoted columns exist for
// clear ownership and indexed user/project/status queries. The
// identityPostgresResource struct is the shared typed-table descriptor (named
// for its first adopter); see typedPostgresResourceFor.
var requestNotificationPostgresResources = map[string]identityPostgresResource{
	requestNotificationFormsResource: {
		resource: requestNotificationFormsResource,
		table:    "forms",
		insert:   requestNotificationFormInsertColumns,
		update:   requestNotificationFormUpdateColumns,
	},
	requestNotificationFormMessagesResource: {
		resource: requestNotificationFormMessagesResource,
		table:    "form_messages",
		insert:   requestNotificationFormMessageInsertColumns,
		update:   requestNotificationFormMessageUpdateColumns,
	},
	requestNotificationAnnouncementsResource: {
		resource: requestNotificationAnnouncementsResource,
		table:    "announcements",
		insert:   requestNotificationAnnouncementInsertColumns,
		update:   requestNotificationAnnouncementUpdateColumns,
	},
	requestNotificationAnnouncementReadsResource: {
		resource: requestNotificationAnnouncementReadsResource,
		table:    "announcement_reads",
		insert:   requestNotificationAnnouncementReadInsertColumns,
		update:   requestNotificationAnnouncementReadUpdateColumns,
	},
	requestNotificationNotificationsResource: {
		resource: requestNotificationNotificationsResource,
		table:    "notifications",
		insert:   requestNotificationNotificationInsertColumns,
		update:   requestNotificationNotificationUpdateColumns,
	},
}

func requestNotificationPostgresResourceFor(resource string) (identityPostgresResource, bool) {
	spec, ok := requestNotificationPostgresResources[resource]
	return spec, ok
}

func requestNotificationFormInsertColumns(data map[string]any, _ string, _ time.Time) []identityColumnValue {
	return []identityColumnValue{
		{"user_id", identityTextDefault(data, "", "user_id", "userId")},
		{"project_id", identityNullableText(data, "project_id", "projectId")},
		{"tag", identityTextDefault(data, "", "tag")},
		{"title", identityTextDefault(data, "", "title")},
		{"status", identityTextDefault(data, "Pending", "status")},
	}
}

func requestNotificationFormUpdateColumns(data map[string]any) []identityColumnValue {
	return identityColumnsFromData(data, []identityColumnReader{
		{"user_id", identityTextUpdate("user_id", "userId")},
		{"project_id", identityNullableTextUpdate("project_id", "projectId")},
		{"tag", identityTextUpdate("tag")},
		{"title", identityTextUpdate("title")},
		{"status", identityTextUpdate("status")},
	})
}

func requestNotificationFormMessageInsertColumns(data map[string]any, _ string, _ time.Time) []identityColumnValue {
	return []identityColumnValue{
		{"form_id", identityTextDefault(data, "", "form_id", "formId")},
		{"user_id", identityTextDefault(data, "", "user_id", "userId")},
	}
}

func requestNotificationFormMessageUpdateColumns(data map[string]any) []identityColumnValue {
	return identityColumnsFromData(data, []identityColumnReader{
		{"form_id", identityTextUpdate("form_id", "formId")},
		{"user_id", identityTextUpdate("user_id", "userId")},
	})
}

func requestNotificationAnnouncementInsertColumns(data map[string]any, _ string, _ time.Time) []identityColumnValue {
	return []identityColumnValue{
		{"priority", identityTextDefault(data, "info", "priority")},
		{"is_pinned", identityBoolDefault(data, false, "is_pinned", "isPinned")},
		{"published_at", identityNullableTime(data, "published_at", "publishedAt")},
		{"expires_at", identityNullableTime(data, "expires_at", "expiresAt")},
		{"created_by", identityTextDefault(data, "", "created_by", "createdBy")},
	}
}

func requestNotificationAnnouncementUpdateColumns(data map[string]any) []identityColumnValue {
	return identityColumnsFromData(data, []identityColumnReader{
		{"priority", identityTextUpdate("priority")},
		{"is_pinned", identityBoolUpdate("is_pinned", "isPinned")},
		{"published_at", identityNullableTimeUpdate("published_at", "publishedAt")},
		{"expires_at", identityNullableTimeUpdate("expires_at", "expiresAt")},
		{"created_by", identityTextUpdate("created_by", "createdBy")},
	})
}

func requestNotificationAnnouncementReadInsertColumns(data map[string]any, _ string, _ time.Time) []identityColumnValue {
	return []identityColumnValue{
		{"announcement_id", identityTextDefault(data, "", "announcement_id", "announcementId")},
		{"user_id", identityTextDefault(data, "", "user_id", "userId")},
	}
}

func requestNotificationAnnouncementReadUpdateColumns(data map[string]any) []identityColumnValue {
	return identityColumnsFromData(data, []identityColumnReader{
		{"announcement_id", identityTextUpdate("announcement_id", "announcementId")},
		{"user_id", identityTextUpdate("user_id", "userId")},
	})
}

func requestNotificationNotificationInsertColumns(data map[string]any, _ string, _ time.Time) []identityColumnValue {
	return []identityColumnValue{
		{"user_id", identityTextDefault(data, "", "user_id", "userId")},
		{"notification_id", identityTextDefault(data, "", "notification_id", "notificationId")},
		{"read", identityBoolDefault(data, false, "read")},
	}
}

func requestNotificationNotificationUpdateColumns(data map[string]any) []identityColumnValue {
	return identityColumnsFromData(data, []identityColumnReader{
		{"user_id", identityTextUpdate("user_id", "userId")},
		{"notification_id", identityTextUpdate("notification_id", "notificationId")},
		{"read", identityBoolUpdate("read")},
	})
}
