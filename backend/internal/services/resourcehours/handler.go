package resourcehours

import (
	"net/http"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

const (
	serviceName        = "usage-observability-service"
	resourceName       = serviceName + ":request_usage"
	podRecordsResource = serviceName + ":request_usage_pods"
	dateLayout         = "2006-01-02"
)

type UserResourceHours struct {
	UserID         string
	Username       string
	GroupID        string
	GroupName      string
	ProjectID      string
	ProjectName    string
	JobID          string
	CPUHours       float64
	GPUHours       float64
	MemoryGBHours  float64
	PeriodStart    *time.Time
	PeriodEnd      *time.Time
	IsFinalized    bool
	LastComputedAt time.Time
}

type AdminUsageSummary struct {
	UniqueUsers        int     `json:"uniqueUsers"`
	UniqueGroups       int     `json:"uniqueGroups"`
	UniqueProjects     int     `json:"uniqueProjects"`
	TotalCPUHours      float64 `json:"totalCPUHours"`
	TotalGPUHours      float64 `json:"totalGPUHours"`
	TotalMemoryGBHours float64 `json:"totalMemoryGBHours"`
}

type AdminUsageResponse struct {
	Rows     []UserResourceHours `json:"rows"`
	Summary  AdminUsageSummary   `json:"summary"`
	Since    string              `json:"since"`
	Filters  map[string]string   `json:"filters"`
	RowCount int                 `json:"rowCount"`
}

type adminUsageFilters struct {
	UserID          string
	Username        string
	GroupID         string
	ProjectID       string
	FinalizedRaw    string
	FilterFinalized bool
	Finalized       bool
}

func Register(app *platform.App) {
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/me/request-usage", getMyUsage)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/admin/request-usage", getAdminUsage)
	registerResourceHoursCollector(app)
}

func getMyUsage(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID := currentUserID(r)
	if userID == "" {
		return http.StatusUnauthorized, map[string]any{"message": "unauthorized"}, nil
	}
	since := resolveSince(r.URL.Query().Get("since"))
	rows := usageRows(app, r, since)
	out := make([]UserResourceHours, 0, len(rows))
	for _, row := range rows {
		if row.UserID == userID {
			out = append(out, row)
		}
	}
	return http.StatusOK, out, nil
}

func getAdminUsage(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID := currentUserID(r)
	if userID == "" {
		return http.StatusUnauthorized, map[string]any{"message": "unauthorized"}, nil
	}
	if !isAdmin(r) {
		return http.StatusForbidden, map[string]any{"message": "admin access required"}, nil
	}

	since := resolveSince(r.URL.Query().Get("since"))
	filters := adminUsageFiltersFromRequest(r)
	filtered, summary := filterAdminUsageRows(usageRows(app, r, since), filters)
	return http.StatusOK, AdminUsageResponse{
		Rows:     filtered,
		Summary:  summary,
		Since:    since.Format(dateLayout),
		Filters:  filters.values(),
		RowCount: len(filtered),
	}, nil
}

func adminUsageFiltersFromRequest(r *http.Request) adminUsageFilters {
	finalizedRaw := strings.TrimSpace(r.URL.Query().Get("finalized"))
	return adminUsageFilters{
		UserID:          strings.TrimSpace(r.URL.Query().Get("user_id")),
		Username:        strings.TrimSpace(r.URL.Query().Get("username")),
		GroupID:         strings.TrimSpace(r.URL.Query().Get("group_id")),
		ProjectID:       strings.TrimSpace(r.URL.Query().Get("project_id")),
		FinalizedRaw:    finalizedRaw,
		FilterFinalized: finalizedRaw != "",
		Finalized:       strings.EqualFold(finalizedRaw, "true"),
	}
}

func filterAdminUsageRows(rows []UserResourceHours, filters adminUsageFilters) ([]UserResourceHours, AdminUsageSummary) {
	filtered := make([]UserResourceHours, 0, len(rows))
	for _, row := range rows {
		if filters.matches(row) {
			filtered = append(filtered, row)
		}
	}
	return filtered, summarizeAdminUsageRows(filtered)
}

func (filters adminUsageFilters) matches(row UserResourceHours) bool {
	if filters.UserID != "" && row.UserID != filters.UserID {
		return false
	}
	if filters.Username != "" && !strings.EqualFold(row.Username, filters.Username) {
		return false
	}
	if filters.GroupID != "" && row.GroupID != filters.GroupID {
		return false
	}
	if filters.ProjectID != "" && row.ProjectID != filters.ProjectID {
		return false
	}
	if filters.FilterFinalized && row.IsFinalized != filters.Finalized {
		return false
	}
	return true
}

func summarizeAdminUsageRows(rows []UserResourceHours) AdminUsageSummary {
	userSet := map[string]struct{}{}
	groupSet := map[string]struct{}{}
	projectSet := map[string]struct{}{}
	summary := AdminUsageSummary{}
	for _, row := range rows {
		userSet[row.UserID] = struct{}{}
		if row.GroupID != "" {
			groupSet[row.GroupID] = struct{}{}
		}
		projectSet[row.ProjectID] = struct{}{}
		summary.TotalCPUHours += row.CPUHours
		summary.TotalGPUHours += row.GPUHours
		summary.TotalMemoryGBHours += row.MemoryGBHours
	}
	summary.UniqueUsers = len(userSet)
	summary.UniqueGroups = len(groupSet)
	summary.UniqueProjects = len(projectSet)
	return summary
}

func (filters adminUsageFilters) values() map[string]string {
	return map[string]string{
		"user_id":    filters.UserID,
		"username":   filters.Username,
		"group_id":   filters.GroupID,
		"project_id": filters.ProjectID,
		"finalized":  filters.FinalizedRaw,
	}
}

func usageRows(app *platform.App, r *http.Request, since time.Time) []UserResourceHours {
	if app == nil || app.Store == nil {
		return nil
	}
	records := app.Store.List(r.Context(), resourceName)
	rows := make([]UserResourceHours, 0, len(records))
	for _, record := range records {
		row := rowFromMap(record.Data)
		if row.PeriodStart != nil && row.PeriodStart.Before(since) {
			continue
		}
		rows = append(rows, row)
	}
	return rows
}

func rowFromMap(data map[string]any) UserResourceHours {
	return UserResourceHours{
		UserID:         textValue(data, "user_id", "userId", "UserID"),
		Username:       textValue(data, "username", "Username"),
		GroupID:        textValue(data, "group_id", "groupId", "GroupID"),
		GroupName:      textValue(data, "group_name", "groupName", "GroupName"),
		ProjectID:      textValue(data, "project_id", "projectId", "ProjectID"),
		ProjectName:    textValue(data, "project_name", "projectName", "ProjectName"),
		JobID:          textValue(data, "job_id", "jobId", "JobID"),
		CPUHours:       floatValue(data, "cpu_hours", "cpuHours", "CPUHours"),
		GPUHours:       floatValue(data, "gpu_hours", "gpuHours", "GPUHours"),
		MemoryGBHours:  floatValue(data, "memory_gb_hours", "memoryGBHours", "MemoryGBHours"),
		PeriodStart:    timeValue(data, "period_start", "periodStart", "PeriodStart"),
		PeriodEnd:      timeValue(data, "period_end", "periodEnd", "PeriodEnd"),
		IsFinalized:    boolValue(data, "is_finalized", "isFinalized", "IsFinalized"),
		LastComputedAt: derefTime(timeValue(data, "last_computed_at", "lastComputedAt", "LastComputedAt")),
	}
}

func resolveSince(raw string) time.Time {
	now := time.Now()
	firstOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	if raw == "" {
		return firstOfMonth
	}
	if parsed, err := time.Parse(dateLayout, raw); err == nil {
		return parsed
	}
	return firstOfMonth
}

func currentUserID(r *http.Request) string {
	return strings.TrimSpace(r.Header.Get("X-User-ID"))
}

func isAdmin(r *http.Request) bool {
	role := strings.ToLower(r.Header.Get("X-User-Role"))
	return role == "admin" || role == "super-admin"
}

func textValue(data map[string]any, keys ...string) string {
	for _, key := range keys {
		value, _ := data[key].(string)
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func floatValue(data map[string]any, keys ...string) float64 {
	for _, key := range keys {
		switch value := data[key].(type) {
		case float64:
			return value
		case float32:
			return float64(value)
		case int:
			return float64(value)
		case int64:
			return float64(value)
		}
	}
	return 0
}

func boolValue(data map[string]any, keys ...string) bool {
	for _, key := range keys {
		value, _ := data[key].(bool)
		if value {
			return true
		}
	}
	return false
}

func timeValue(data map[string]any, keys ...string) *time.Time {
	for _, key := range keys {
		switch value := data[key].(type) {
		case time.Time:
			return &value
		case string:
			if parsed, err := time.Parse(time.RFC3339, value); err == nil {
				return &parsed
			}
			if parsed, err := time.Parse(dateLayout, value); err == nil {
				return &parsed
			}
		}
	}
	return nil
}

func derefTime(value *time.Time) time.Time {
	if value == nil {
		return time.Time{}
	}
	return *value
}
