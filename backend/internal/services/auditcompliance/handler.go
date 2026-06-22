package auditcompliance

import (
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

const (
	serviceName               = "audit-compliance-service"
	auditLogResource          = serviceName + ":audit_logs"
	projectMemberConsumer     = serviceName + ":project_member_projection"
	groupMemberConsumer       = serviceName + ":group_member_projection"
	projectReportMembers      = serviceName + ":project_report_members"
	groupReportMembers        = serviceName + ":group_report_members"
	orgProjectMembersResource = "org-project-service:project_members"
	orgUserGroupsResource     = "org-project-service:user_groups"
)

type AuditLog struct {
	ID            string         `json:"id"`
	UserID        string         `json:"user_id"`
	ProjectID     *string        `json:"project_id,omitempty"`
	GroupID       *string        `json:"group_id,omitempty"`
	Action        string         `json:"action"`
	ResourceType  string         `json:"resource_type"`
	ResourceID    string         `json:"resource_id"`
	OldData       map[string]any `json:"old_data,omitempty"`
	NewData       map[string]any `json:"new_data,omitempty"`
	Metadata      AuditMetadata  `json:"metadata"`
	CreatedAt     time.Time      `json:"created_at"`
	IntegrityHash string         `json:"integrity_hash"`
	PreviousHash  string         `json:"previous_hash,omitempty"`
}

type AuditMetadata struct {
	IPAddress   string `json:"ip_address"`
	UserAgent   string `json:"user_agent"`
	Description string `json:"description"`
}

type TopOffender struct {
	UserID       string    `json:"user_id"`
	Username     string    `json:"username"`
	FailureCount int64     `json:"failure_count"`
	LastSeen     time.Time `json:"last_seen"`
}

type offenderState struct {
	count int64
	last  time.Time
}

type SecurityPosture struct {
	AuthFailures24h   int64         `json:"auth_failures_24h"`
	RoleChanges7d     int64         `json:"role_changes_7d"`
	PolicyMutations7d int64         `json:"policy_mutations_7d"`
	TopOffenders      []TopOffender `json:"top_offenders"`
	RecentEvents      []AuditLog    `json:"recent_events"`
}

type queryParams struct {
	UserID       string
	ProjectID    string
	GroupID      string
	ResourceType string
	Action       string
	StartTime    *time.Time
	EndTime      *time.Time
	Limit        int
	Offset       int
	PageProvided bool
}

func Register(app *platform.App) {
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/audit/logs", getAuditLogs)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/audit/report", downloadProjectReport)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/admin/security/posture", getSecurityPosture)
	app.RegisterCustomHandler(http.MethodPost, "/api/v1/internal/audit/cleanup", cleanupAuditRetention)
	registerAuditRetention(app)
}

func getAuditLogs(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID := currentUserID(r)
	if userID == "" {
		return http.StatusUnauthorized, shared.ErrorData("unauthorized"), nil
	}
	params, status, data, ok := parseQueryParams(r)
	if !ok {
		return status, data, nil
	}
	if !canQueryAuditLogs(app, r, userID, &params) {
		if !hasScopedAuditQuery(params) {
			return http.StatusBadRequest, shared.ErrorData("project_id or group_id is required for scoped audit query"), nil
		}
		return http.StatusForbidden, shared.ErrorData("project or group admin access required"), nil
	}
	logs := filterLogs(auditLogs(app, r), params)
	total := len(logs)
	logs = pageLogs(logs, params.Limit, params.Offset)
	if paginationRequested(r) {
		page := 1
		if params.Limit > 0 {
			page = params.Offset/params.Limit + 1
		}
		return http.StatusOK, map[string]any{
			"list":      logs,
			"total":     int64(total),
			"page":      page,
			"page_size": params.Limit,
			"offset":    params.Offset,
		}, nil
	}
	return http.StatusOK, logs, nil
}

func downloadProjectReport(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID := currentUserID(r)
	if userID == "" {
		return http.StatusUnauthorized, shared.ErrorData("unauthorized"), nil
	}
	projectID := strings.TrimSpace(r.URL.Query().Get("project_id"))
	if projectID == "" {
		return http.StatusBadRequest, shared.ErrorData("project_id is required"), nil
	}
	start, err := time.Parse(time.RFC3339, r.URL.Query().Get("start"))
	if err != nil || start.IsZero() {
		return http.StatusBadRequest, shared.ErrorData("valid start time in RFC3339 format is required"), nil
	}
	end, _ := time.Parse(time.RFC3339, r.URL.Query().Get("end"))
	if end.IsZero() {
		end = time.Now().UTC()
	}
	if !canReadProject(app, r, userID, projectID, isAdmin(r)) {
		return http.StatusForbidden, shared.ErrorData("project member access required"), nil
	}

	params := queryParams{StartTime: &start, EndTime: &end, Limit: 1000}
	logs := filterLogs(auditLogs(app, r), params)
	filtered := logs[:0]
	for _, log := range logs {
		if log.ProjectID != nil && *log.ProjectID == projectID {
			filtered = append(filtered, log)
		}
	}
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	writeProjectReportCSV(writer, app.Config.EffectiveProductName(), projectID, filtered)
	writer.Flush()
	return http.StatusOK, platform.RawResponse{
		ContentType: "text/csv",
		Headers: map[string]string{
			"Content-Disposition": "attachment;filename=" + auditReportFilename(app.Config.EffectiveProductName(), projectID),
		},
		Body: buf.Bytes(),
	}, nil
}

func writeProjectReportCSV(writer *csv.Writer, productName, projectID string, logs []AuditLog) {
	_ = writer.Write([]string{"Product", productName})
	_ = writer.Write([]string{"Project", projectID})
	_ = writer.Write(nil)
	_ = writer.Write([]string{"Time", "User", "Action", "Resource Type", "Resource ID", "Description", "Integrity Hash", "Previous Hash"})
	for _, log := range logs {
		_ = writer.Write([]string{
			log.CreatedAt.Format(time.RFC3339),
			log.UserID,
			log.Action,
			log.ResourceType,
			log.ResourceID,
			log.Metadata.Description,
			log.IntegrityHash,
			log.PreviousHash,
		})
	}
}

func auditReportFilename(productName, projectID string) string {
	product := safeFilenameToken(productName, "nexuspaas")
	project := safeFilenameToken(projectID, "project")
	return product + "_audit_report_" + project + ".csv"
}

func safeFilenameToken(value, fallback string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r == ' ' || r == '-' || r == '_':
			if b.Len() > 0 && !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	token := strings.Trim(b.String(), "-")
	if token == "" {
		return fallback
	}
	return token
}

func getSecurityPosture(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID := currentUserID(r)
	if userID == "" {
		return http.StatusUnauthorized, shared.ErrorData("unauthorized"), nil
	}
	if !isAdmin(r) {
		return http.StatusForbidden, shared.ErrorData("admin access required"), nil
	}
	windowDays := 7
	if raw := r.URL.Query().Get("window_days"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 {
			return http.StatusBadRequest, shared.ErrorData("window_days must be a positive integer"), nil
		}
		windowDays = parsed
	}
	if windowDays > 90 {
		return http.StatusBadRequest, shared.ErrorData("window_days must be ≤ 90"), nil
	}
	return http.StatusOK, securityPosture(auditLogs(app, r), windowDays, time.Now().UTC()), nil
}

func parseQueryParams(r *http.Request) (queryParams, int, any, bool) {
	params := queryParams{Limit: 100, Offset: 0}
	query := r.URL.Query()
	params.UserID = strings.TrimSpace(query.Get("user_id"))
	params.ProjectID = strings.TrimSpace(query.Get("project_id"))
	params.GroupID = strings.TrimSpace(query.Get("group_id"))
	params.ResourceType = strings.TrimSpace(query.Get("resource_type"))
	params.Action = strings.TrimSpace(query.Get("action"))
	if parsed, status, data, ok := parseOptionalTime(query.Get("start_time"), "Invalid start_time"); !ok {
		return queryParams{}, status, data, false
	} else {
		params.StartTime = parsed
	}
	if parsed, status, data, ok := parseOptionalTime(query.Get("end_time"), "Invalid end_time"); !ok {
		return queryParams{}, status, data, false
	} else {
		params.EndTime = parsed
	}
	if value, status, data, ok := parseOptionalNonNegativeInt(query.Get("limit"), "invalid limit parameter"); !ok {
		return queryParams{}, status, data, false
	} else if value != nil {
		params.Limit = *value
	}
	if value, status, data, ok := parseOptionalNonNegativeInt(query.Get("offset"), "invalid offset parameter"); !ok {
		return queryParams{}, status, data, false
	} else if value != nil {
		params.Offset = *value
	}
	if params.Limit > 1000 {
		params.Limit = 1000
	}
	if page, status, data, ok := parseOptionalPositiveInt(query.Get("page"), "invalid page parameter"); !ok {
		return queryParams{}, status, data, false
	} else if page != nil {
		params.PageProvided = true
		params.Offset = (*page - 1) * params.Limit
	}
	return params, 0, nil, true
}

func parseOptionalTime(raw, message string) (*time.Time, int, any, bool) {
	if raw == "" {
		return nil, 0, nil, true
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, http.StatusBadRequest, shared.ErrorData(message), false
	}
	return &parsed, 0, nil, true
}

func parseOptionalNonNegativeInt(raw, message string) (*int, int, any, bool) {
	return parseOptionalInt(raw, message, 0)
}

func parseOptionalPositiveInt(raw, message string) (*int, int, any, bool) {
	return parseOptionalInt(raw, message, 1)
}

func parseOptionalInt(raw, message string, min int) (*int, int, any, bool) {
	if raw == "" {
		return nil, 0, nil, true
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed < min {
		return nil, http.StatusBadRequest, shared.ErrorData(message), false
	}
	return &parsed, 0, nil, true
}

// RecentAuditLogMaps returns audit logs composed from the event outbox and the
// stored audit_logs resource, newest first, as generic maps for cross-service
// read models such as the dashboard. It is the shared reader that keeps those
// views consistent with emitted audit events instead of reading a raw store key
// that the audit service never populates (finding 32).
func RecentAuditLogMaps(app *platform.App, r *http.Request, limit int) []map[string]any {
	logs := auditLogs(app, r) // merged (outbox + store) and sorted newest-first
	if limit > 0 && len(logs) > limit {
		logs = logs[:limit]
	}
	out := make([]map[string]any, 0, len(logs))
	for _, entry := range logs {
		row := map[string]any{
			"id":            entry.ID,
			"user_id":       entry.UserID,
			"action":        entry.Action,
			"resource_type": entry.ResourceType,
			"resource_id":   entry.ResourceID,
			"created_at":    entry.CreatedAt.Format(time.RFC3339),
		}
		if entry.ProjectID != nil {
			row["project_id"] = *entry.ProjectID
		}
		if entry.GroupID != nil {
			row["group_id"] = *entry.GroupID
		}
		out = append(out, row)
	}
	return out
}

func auditLogs(app *platform.App, r *http.Request) []AuditLog {
	logs := []AuditLog{}
	if app == nil {
		return logs
	}
	for _, event := range app.Events.Outbox() {
		if event.Name != "AuditEvent" {
			continue
		}
		logs = append(logs, AuditLog{
			ID:           auditEventLogID(event),
			UserID:       auditEventActorID(event.Data),
			ProjectID:    optionalString(shared.TextValue(event.Data, "project_id", "projectId")),
			GroupID:      optionalString(shared.TextValue(event.Data, "group_id", "groupId")),
			Action:       shared.TextValue(event.Data, "action"),
			ResourceType: shared.FirstNonEmpty(shared.TextValue(event.Data, "resource_type"), shared.TextValue(event.Data, "resource")),
			ResourceID:   shared.TextValue(event.Data, "resource_id"),
			Metadata: AuditMetadata{
				IPAddress:   shared.TextValue(event.Data, "source_ip", "ip_address"),
				UserAgent:   shared.TextValue(event.Data, "user_agent"),
				Description: shared.TextValue(event.Data, "description"),
			},
			CreatedAt: event.OccurredAt,
		})
	}
	if app.Store != nil {
		for _, record := range app.Store.List(r.Context(), auditLogResource) {
			logs = append(logs, logFromMap(record.ID, record.Data, record.CreatedAt))
		}
	}
	sortAuditLogs(logs)
	applyAuditIntegrityChain(logs)
	return logs
}

func sortAuditLogs(logs []AuditLog) {
	sort.Slice(logs, func(i, j int) bool {
		if !logs[i].CreatedAt.Equal(logs[j].CreatedAt) {
			return logs[i].CreatedAt.After(logs[j].CreatedAt)
		}
		return logs[i].ID > logs[j].ID
	})
}

func applyAuditIntegrityChain(logs []AuditLog) {
	previousHash := ""
	for i := len(logs) - 1; i >= 0; i-- {
		logs[i].PreviousHash = previousHash
		logs[i].IntegrityHash = auditLogIntegrityHash(previousHash, logs[i])
		previousHash = logs[i].IntegrityHash
	}
}

type auditLogIntegrityPayload struct {
	PreviousHash string         `json:"previous_hash"`
	ID           string         `json:"id"`
	UserID       string         `json:"user_id"`
	ProjectID    string         `json:"project_id,omitempty"`
	GroupID      string         `json:"group_id,omitempty"`
	Action       string         `json:"action"`
	ResourceType string         `json:"resource_type"`
	ResourceID   string         `json:"resource_id"`
	OldData      map[string]any `json:"old_data,omitempty"`
	NewData      map[string]any `json:"new_data,omitempty"`
	Metadata     AuditMetadata  `json:"metadata"`
	CreatedAt    string         `json:"created_at"`
}

func auditLogIntegrityHash(previousHash string, log AuditLog) string {
	projectID := ""
	if log.ProjectID != nil {
		projectID = *log.ProjectID
	}
	groupID := ""
	if log.GroupID != nil {
		groupID = *log.GroupID
	}
	payload := auditLogIntegrityPayload{
		PreviousHash: previousHash,
		ID:           log.ID,
		UserID:       log.UserID,
		ProjectID:    projectID,
		GroupID:      groupID,
		Action:       log.Action,
		ResourceType: log.ResourceType,
		ResourceID:   log.ResourceID,
		OldData:      log.OldData,
		NewData:      log.NewData,
		Metadata:     log.Metadata,
		CreatedAt:    log.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		raw = []byte(previousHash + log.ID + log.UserID + log.Action + log.ResourceType + log.ResourceID + log.CreatedAt.UTC().Format(time.RFC3339Nano))
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func auditEventLogID(event contracts.Event) string {
	return shared.FirstNonEmpty(shared.TextValue(event.Data, "audit_event_id", "auditEventID", "id"), event.EventID)
}

func auditEventActorID(data map[string]any) string {
	return shared.TextValue(data, "actor_user_id", "actorUserID", "user_id", "userId")
}

func logFromMap(id string, data map[string]any, fallback time.Time) AuditLog {
	projectID := optionalString(shared.TextValue(data, "project_id", "projectId"))
	groupID := optionalString(shared.TextValue(data, "group_id", "groupId"))
	createdAt := timeValue(data, "created_at", "createdAt")
	if createdAt.IsZero() {
		createdAt = fallback
	}
	return AuditLog{
		ID:           shared.FirstNonEmpty(shared.TextValue(data, "id"), id),
		UserID:       shared.TextValue(data, "user_id", "userId"),
		ProjectID:    projectID,
		GroupID:      groupID,
		Action:       shared.TextValue(data, "action"),
		ResourceType: shared.TextValue(data, "resource_type", "resourceType"),
		ResourceID:   shared.TextValue(data, "resource_id", "resourceId"),
		OldData:      mapValue(data, "old_data", "oldData"),
		NewData:      mapValue(data, "new_data", "newData"),
		Metadata: AuditMetadata{
			IPAddress:   shared.TextValue(data, "ip_address", "source_ip"),
			UserAgent:   shared.TextValue(data, "user_agent", "userAgent"),
			Description: shared.TextValue(data, "description"),
		},
		CreatedAt: createdAt,
	}
}

func filterLogs(logs []AuditLog, params queryParams) []AuditLog {
	filtered := make([]AuditLog, 0, len(logs))
	for _, log := range logs {
		if auditLogMatches(log, params) {
			filtered = append(filtered, log)
		}
	}
	return filtered
}

func auditLogMatches(log AuditLog, params queryParams) bool {
	return stringFilterMatches(params.UserID, log.UserID) &&
		optionalStringFilterMatches(params.ProjectID, log.ProjectID) &&
		optionalStringFilterMatches(params.GroupID, log.GroupID) &&
		stringFilterMatches(params.ResourceType, log.ResourceType) &&
		stringFilterMatches(params.Action, log.Action) &&
		timeWindowMatches(log.CreatedAt, params.StartTime, params.EndTime)
}

func stringFilterMatches(filter, value string) bool {
	return filter == "" || value == filter
}

func optionalStringFilterMatches(filter string, value *string) bool {
	return filter == "" || (value != nil && *value == filter)
}

func timeWindowMatches(value time.Time, start, end *time.Time) bool {
	if start != nil && value.Before(*start) {
		return false
	}
	if end != nil && value.After(*end) {
		return false
	}
	return true
}

func canQueryAuditLogs(app *platform.App, r *http.Request, userID string, params *queryParams) bool {
	if isAdmin(r) || isPlatformAuditor(r) {
		return true
	}
	if !hasScopedAuditQuery(*params) {
		return false
	}
	if params.ProjectID != "" && canQueryProjectAuditLogs(app, r, userID, params.ProjectID) {
		return true
	}
	if params.GroupID != "" && canQueryGroupAuditLogs(app, r, userID, params.GroupID) {
		return true
	}
	return false
}

func hasScopedAuditQuery(params queryParams) bool {
	return params.ProjectID != "" || params.GroupID != ""
}

func canQueryProjectAuditLogs(app *platform.App, r *http.Request, userID, projectID string) bool {
	if app == nil || app.Store == nil {
		return false
	}
	syncProjectMemberReadModel(app, r)
	for _, member := range projectMemberRecords(app, r) {
		if shared.TextValue(member, "project_id", "projectId") != projectID || shared.TextValue(member, "user_id", "userId") != userID {
			continue
		}
		switch shared.TextValue(member, "role") {
		case "admin", "project_admin":
			return true
		}
	}
	return false
}

func canQueryGroupAuditLogs(app *platform.App, r *http.Request, userID, groupID string) bool {
	if app == nil || app.Store == nil {
		return false
	}
	syncGroupMemberReadModel(app, r)
	for _, member := range groupMemberRecords(app, r) {
		if shared.TextValue(member, "group_id", "groupId", "gid", "g_id") != groupID ||
			shared.TextValue(member, "user_id", "userId", "uid", "u_id") != userID {
			continue
		}
		if isGroupAuditAdminRole(shared.TextValue(member, "role")) {
			return true
		}
	}
	return false
}

func isGroupAuditAdminRole(role string) bool {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "admin", "group_admin":
		return true
	default:
		return false
	}
}

func pageLogs(logs []AuditLog, limit, offset int) []AuditLog {
	if offset > len(logs) {
		offset = len(logs)
	}
	end := offset + limit
	if limit == 0 || end > len(logs) {
		end = len(logs)
	}
	return logs[offset:end]
}

func securityPosture(logs []AuditLog, windowDays int, now time.Time) SecurityPosture {
	yesterday := now.Add(-24 * time.Hour)
	week := now.AddDate(0, 0, -7)
	window := now.AddDate(0, 0, -windowDays)
	securityActions := map[string]bool{
		"login_failed":   true,
		"logout":         true,
		"policy_added":   true,
		"policy_removed": true,
		"role_changed":   true,
	}
	offenders := map[string]offenderState{}
	posture := SecurityPosture{TopOffenders: []TopOffender{}, RecentEvents: []AuditLog{}}
	for _, log := range logs {
		accumulateSecurityPosture(&posture, offenders, securityActions, log, yesterday, week, window)
	}
	for userID, state := range offenders {
		posture.TopOffenders = append(posture.TopOffenders, TopOffender{UserID: userID, FailureCount: state.count, LastSeen: state.last})
	}
	sort.Slice(posture.TopOffenders, func(i, j int) bool {
		if posture.TopOffenders[i].FailureCount == posture.TopOffenders[j].FailureCount {
			return posture.TopOffenders[i].LastSeen.After(posture.TopOffenders[j].LastSeen)
		}
		return posture.TopOffenders[i].FailureCount > posture.TopOffenders[j].FailureCount
	})
	if len(posture.TopOffenders) > 10 {
		posture.TopOffenders = posture.TopOffenders[:10]
	}
	return posture
}

func accumulateSecurityPosture(posture *SecurityPosture, offenders map[string]offenderState, securityActions map[string]bool, log AuditLog, yesterday, week, window time.Time) {
	switch {
	case log.Action == "login_failed" && !log.CreatedAt.Before(yesterday):
		posture.AuthFailures24h++
	case log.Action == "role_changed" && !log.CreatedAt.Before(week):
		posture.RoleChanges7d++
	case (log.Action == "policy_added" || log.Action == "policy_removed") && !log.CreatedAt.Before(week):
		posture.PolicyMutations7d++
	}
	recordOffender(offenders, log, window)
	if securityActions[log.Action] && len(posture.RecentEvents) < 50 {
		posture.RecentEvents = append(posture.RecentEvents, log)
	}
}

func recordOffender(offenders map[string]offenderState, log AuditLog, window time.Time) {
	if log.Action != "login_failed" || log.CreatedAt.Before(window) {
		return
	}
	state := offenders[log.UserID]
	state.count++
	if log.CreatedAt.After(state.last) {
		state.last = log.CreatedAt
	}
	offenders[log.UserID] = state
}

func paginationRequested(r *http.Request) bool {
	query := r.URL.Query()
	return query.Get("page") != "" || query.Get("page_size") != "" || query.Get("limit") != "" || query.Get("offset") != ""
}

func canReadProject(app *platform.App, r *http.Request, userID, projectID string, admin bool) bool {
	if admin {
		return true
	}
	if app == nil || app.Store == nil {
		return false
	}
	syncProjectMemberReadModel(app, r)
	for _, member := range projectMemberRecords(app, r) {
		if shared.TextValue(member, "project_id", "projectId") == projectID && shared.TextValue(member, "user_id", "userId") == userID {
			return true
		}
	}
	return false
}

func syncProjectMemberReadModel(app *platform.App, r *http.Request) {
	if app == nil || app.Store == nil || app.Events == nil {
		return
	}
	app.RunProjection(r.Context(), projectMemberConsumer, func(event contracts.Event) error {
		return projectMemberEvent(app, r, event)
	})
}

func projectMemberEvent(app *platform.App, r *http.Request, event contracts.Event) error {
	data, deleted, ok := projectMemberProjection(event)
	if !ok {
		return nil
	}
	if deleted {
		deleteProjectMemberReadModel(app, r, data)
		return nil
	}
	return upsertProjectMemberReadModel(app, r, data)
}

func projectMemberProjection(event contracts.Event) (map[string]any, bool, bool) {
	name := strings.ToLower(event.Name)
	switch name {
	case "project_membercreated", "project_memberupdated":
		return projectMemberEventData(event), false, true
	case "project_memberdeleted":
		return projectMemberEventData(event), true, true
	default:
		return nil, false, false
	}
}

func projectMemberEventData(event contracts.Event) map[string]any {
	if next, ok := event.Data["new"].(map[string]any); ok {
		return shared.CloneMap(next)
	}
	return shared.CloneMap(event.Data)
}

func upsertProjectMemberReadModel(app *platform.App, r *http.Request, data map[string]any) error {
	id := projectMemberReadModelID(data)
	if id == "" {
		return nil
	}
	data["id"] = id
	if _, ok := app.Store.Update(r.Context(), projectReportMembers, id, data); ok {
		return nil
	}
	if _, err := app.Store.Create(r.Context(), projectReportMembers, data); err != nil {
		if platform.IsCreateConflict(err) {
			if _, ok := app.Store.Update(r.Context(), projectReportMembers, id, data); !ok {
				return fmt.Errorf("audit project-member projection conflict update missed for %s", id)
			}
			return nil
		}
		return fmt.Errorf("audit project-member projection create failed for %s: %w", id, err)
	}
	return nil
}

func deleteProjectMemberReadModel(app *platform.App, r *http.Request, data map[string]any) {
	if deleted, ok := data["deleted"].(bool); ok && !deleted {
		return
	}
	if id := projectMemberReadModelID(data); id != "" {
		app.Store.Delete(r.Context(), projectReportMembers, id)
	}
}

func projectMemberReadModelID(data map[string]any) string {
	id := shared.TextValue(data, "id")
	projectID := shared.TextValue(data, "project_id", "projectId")
	userID := shared.TextValue(data, "user_id", "userId")
	if id == "" && projectID != "" && userID != "" {
		return projectID + ":" + userID
	}
	return shared.FirstNonEmpty(id, userID, projectID)
}

func projectMemberRecords(app *platform.App, r *http.Request) []map[string]any {
	local := recordMaps(app, r, projectReportMembers)
	if !projectMemberSourceCoHosted(app) {
		return local
	}
	source := recordMaps(app, r, orgProjectMembersResource)
	if len(local) == 0 {
		return source
	}
	return mergeProjectMemberRecords(source, local)
}

func mergeProjectMemberRecords(source, local []map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(source)+len(local))
	seen := map[string]bool{}
	for _, record := range local {
		if id := projectMemberReadModelID(record); id != "" {
			seen[id] = true
		}
		out = append(out, record)
	}
	for _, record := range source {
		id := projectMemberReadModelID(record)
		if id != "" && seen[id] {
			continue
		}
		out = append(out, record)
	}
	return out
}

func syncGroupMemberReadModel(app *platform.App, r *http.Request) {
	if app == nil || app.Store == nil || app.Events == nil {
		return
	}
	app.RunProjection(r.Context(), groupMemberConsumer, func(event contracts.Event) error {
		return groupMemberEvent(app, r, event)
	})
}

func groupMemberEvent(app *platform.App, r *http.Request, event contracts.Event) error {
	data, deleted, ok := groupMemberProjection(event)
	if !ok {
		return nil
	}
	if deleted {
		deleteGroupMemberReadModel(app, r, data)
		return nil
	}
	return upsertGroupMemberReadModel(app, r, data)
}

func groupMemberProjection(event contracts.Event) (map[string]any, bool, bool) {
	if strings.ToLower(event.Name) != "groupmembershipchanged" {
		return nil, false, false
	}
	if next, ok := event.Data["new"].(map[string]any); ok {
		return shared.CloneMap(next), false, true
	}
	data := shared.CloneMap(event.Data)
	deleted := strings.ToLower(shared.TextValue(data, "action")) == "delete"
	if deleted {
		return data, true, true
	}
	return data, false, true
}

func upsertGroupMemberReadModel(app *platform.App, r *http.Request, data map[string]any) error {
	id := groupMemberReadModelID(data)
	if id == "" {
		return nil
	}
	data["id"] = id
	if _, ok := app.Store.Update(r.Context(), groupReportMembers, id, data); ok {
		return nil
	}
	if _, err := app.Store.Create(r.Context(), groupReportMembers, data); err != nil {
		if platform.IsCreateConflict(err) {
			if _, ok := app.Store.Update(r.Context(), groupReportMembers, id, data); !ok {
				return fmt.Errorf("audit group-member projection conflict update missed for %s", id)
			}
			return nil
		}
		return fmt.Errorf("audit group-member projection create failed for %s: %w", id, err)
	}
	return nil
}

func deleteGroupMemberReadModel(app *platform.App, r *http.Request, data map[string]any) {
	if deleted, ok := data["deleted"].(bool); ok && !deleted {
		return
	}
	if id := groupMemberReadModelID(data); id != "" {
		app.Store.Delete(r.Context(), groupReportMembers, id)
	}
}

func groupMemberReadModelID(data map[string]any) string {
	id := shared.TextValue(data, "id")
	groupID := shared.TextValue(data, "group_id", "groupId", "gid", "g_id")
	userID := shared.TextValue(data, "user_id", "userId", "uid", "u_id")
	if groupID != "" && userID != "" {
		return groupID + ":" + userID
	}
	return shared.FirstNonEmpty(id, userID, groupID)
}

func groupMemberRecords(app *platform.App, r *http.Request) []map[string]any {
	local := recordMaps(app, r, groupReportMembers)
	if !groupMemberSourceCoHosted(app) {
		return local
	}
	source := recordMaps(app, r, orgUserGroupsResource)
	if len(local) == 0 {
		return source
	}
	return mergeGroupMemberRecords(source, local)
}

func mergeGroupMemberRecords(source, local []map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(source)+len(local))
	seen := map[string]bool{}
	for _, record := range local {
		if id := groupMemberReadModelID(record); id != "" {
			seen[id] = true
		}
		out = append(out, record)
	}
	for _, record := range source {
		id := groupMemberReadModelID(record)
		if id != "" && seen[id] {
			continue
		}
		out = append(out, record)
	}
	return out
}

func recordMaps(app *platform.App, r *http.Request, resource string) []map[string]any {
	if app == nil || app.Store == nil {
		return nil
	}
	records := app.Store.List(r.Context(), resource)
	out := make([]map[string]any, 0, len(records))
	for _, record := range records {
		out = append(out, shared.CloneMap(record.Data))
	}
	return out
}

func projectMemberSourceCoHosted(app *platform.App) bool {
	return app != nil && app.Config.AllowsService("org-project-service")
}

func groupMemberSourceCoHosted(app *platform.App) bool {
	return app != nil && app.Config.AllowsService("org-project-service")
}

func currentUserID(r *http.Request) string {
	return strings.TrimSpace(r.Header.Get("X-User-ID"))
}

func isAdmin(r *http.Request) bool {
	role := strings.ToLower(r.Header.Get("X-User-Role"))
	return role == "admin" || role == "super-admin"
}

func isPlatformAuditor(r *http.Request) bool {
	return strings.TrimSpace(r.Header.Get("X-User-Role")) == "platform_auditor"
}

func mapValue(data map[string]any, keys ...string) map[string]any {
	for _, key := range keys {
		if value, ok := data[key].(map[string]any); ok {
			return value
		}
	}
	return nil
}

func timeValue(data map[string]any, keys ...string) time.Time {
	for _, key := range keys {
		switch value := data[key].(type) {
		case time.Time:
			return value
		case string:
			if parsed, err := time.Parse(time.RFC3339, value); err == nil {
				return parsed
			}
		}
	}
	return time.Time{}
}

func optionalString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}
