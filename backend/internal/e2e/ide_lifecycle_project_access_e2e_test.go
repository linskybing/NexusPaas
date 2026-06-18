//go:build e2e

package e2e

import (
	"net/http"
	"testing"
)

const (
	ideIdentityUsersResource  = "ide-service:ide_identity_users"
	ideProjectsResource       = "ide-service:ide_projects"
	ideProjectMembersResource = "ide-service:ide_project_members"
	ideSessionsResource       = "ide-service:ide_sessions"
)

func TestIDELifecycleProjectAccessE2E(t *testing.T) {
	h := newHarness(t, identityService, ideService)
	ids := h.seedIdentityContracts()
	badUserID := "badide" + h.runID
	badToken := h.seedAPIUser(badUserID, "bad-ide-"+h.runID, false)
	h.seedIDEProjectAccess(ids.userID, "manager", false)

	memberImages := e2eDataRecords(t, h.doWithBearer(ideService, http.MethodGet, "/api/v1/ide/images?project_id="+h.projectID()+"&ide_type=jupyter", ids.apiToken, http.StatusOK))
	if !e2eRecordsContainDataValue(memberImages, "key", "jupyter-base") || e2eRecordsContainDataValue(memberImages, "key", "jupyter-base-root") {
		t.Fatalf("member IDE images = %#v, want base image without root profile", memberImages)
	}
	publicImages := e2eDataRecords(t, h.doJSON(ideService, http.MethodGet, "/api/v1/ide/images?ide_type=jupyter", nil, h.apiKey, http.StatusOK))
	if e2eRecordsContainDataValue(publicImages, "key", "jupyter-base-root") {
		t.Fatalf("public IDE images = %#v, want no root profiles", publicImages)
	}

	started := h.doJSONWithBearer(ideService, http.MethodPost, "/api/v1/ide/start?project_id="+h.projectID()+"&image_key=jupyter-base&gpu=0", map[string]any{
		"storage_ids":         []string{"storage-" + h.runID},
		"queue_name":          "queue-" + h.runID,
		"sm_percentage":       50,
		"pinned_memory_limit": "8Gi",
		"device_class_name":   "gpu.nvidia.com",
	}, ids.apiToken, http.StatusOK)
	podName := e2eLowerPodName(ids.userID, "jupyter")
	if data := started.dataMap(t); data["status"] != "started" || data["pod_name"] != podName {
		t.Fatalf("start IDE response = %#v, want started %s", data, podName)
	}
	session := h.getRecord(ideSessionsResource, podName)
	if session.Data["project_id"] != h.projectID() || session.Data["user_id"] != ids.userID || session.Data["status"] != "running" {
		t.Fatalf("IDE session = %#v, want running user project session", session.Data)
	}

	otherSessionID := "ide-other-" + e2eSuffix(h.runID)
	h.createRecord(ideSessionsResource, otherSessionID, map[string]any{
		"pod_name":   otherSessionID,
		"project_id": h.projectID(),
		"user_id":    "other-" + h.runID,
		"username":   "other-" + h.runID,
		"ide_type":   "jupyter",
		"status":     "running",
	})
	ownSessions := e2eDataRecords(t, h.doWithBearer(ideService, http.MethodGet, "/api/v1/ide", ids.apiToken, http.StatusOK))
	if !e2eRecordsContainDataValue(ownSessions, "pod_name", podName) || e2eRecordsContainDataValue(ownSessions, "pod_name", otherSessionID) {
		t.Fatalf("own IDE sessions = %#v, want only caller session", ownSessions)
	}
	projectSessions := e2eDataRecords(t, h.doWithBearer(ideService, http.MethodGet, "/api/v1/ide?project_id="+h.projectID(), ids.apiToken, http.StatusOK))
	if !e2eRecordsContainDataValue(projectSessions, "pod_name", podName) || !e2eRecordsContainDataValue(projectSessions, "pod_name", otherSessionID) {
		t.Fatalf("project IDE sessions = %#v, want manager to see project sessions", projectSessions)
	}

	h.assertIDEValidationAndNonMemberGuards(ids.apiToken, badToken, badUserID, podName)

	h.doWithBearer(ideService, http.MethodPost, "/api/v1/ide/stop?project_id="+h.projectID()+"&type=jupyter", ids.apiToken, http.StatusOK)
	if session := h.getRecord(ideSessionsResource, podName); session.Data["status"] != "stopped" {
		t.Fatalf("stopped IDE session = %#v, want stopped", session.Data)
	}
	h.doWithBearer(ideService, http.MethodPost, "/api/v1/ide/delete?project_id="+h.projectID()+"&type=jupyter", ids.apiToken, http.StatusOK)
	if session := h.getRecord(ideSessionsResource, podName); session.Data["status"] != "deleted" {
		t.Fatalf("deleted IDE session = %#v, want deleted", session.Data)
	}
}

func (h *e2eHarness) seedIDEProjectAccess(userID, role string, allowRoot bool) {
	h.t.Helper()
	h.createRecord(ideIdentityUsersResource, userID, map[string]any{
		"username": "ide-user-" + h.runID,
		"status":   "online",
	})
	h.createRecord(ideProjectsResource, h.projectID(), map[string]any{
		"p_id":              h.projectID(),
		"project_name":      "ide-project-" + h.runID,
		"allow_run_as_root": allowRoot,
	})
	h.createRecord(ideProjectMembersResource, h.projectID()+":"+userID, map[string]any{
		"project_id": h.projectID(),
		"user_id":    userID,
		"role":       role,
	})
}

func (h *e2eHarness) assertIDEValidationAndNonMemberGuards(memberToken, badToken, badUserID, protectedPodName string) {
	h.t.Helper()
	h.doWithBearer(ideService, http.MethodPost, "/api/v1/ide/start?project_id="+h.projectID()+"&type=terminal", memberToken, http.StatusBadRequest)
	h.doWithBearer(ideService, http.MethodPost, "/api/v1/ide/start?project_id="+h.projectID()+"&image_key=missing", memberToken, http.StatusBadRequest)
	h.doWithBearer(ideService, http.MethodPost, "/api/v1/ide/start?project_id="+h.projectID()+"&type=vscode&image_key=jupyter-base", memberToken, http.StatusBadRequest)
	h.doWithBearer(ideService, http.MethodPost, "/api/v1/ide/start?project_id="+h.projectID()+"&image_key=jupyter-base-root", memberToken, http.StatusForbidden)
	h.doWithBearer(ideService, http.MethodPost, "/api/v1/ide/start?project_id="+h.projectID()+"&image_key=jupyter-base&executor_type=local", memberToken, http.StatusForbidden)
	h.doJSONWithBearer(ideService, http.MethodPost, "/api/v1/ide/start?project_id="+h.projectID()+"&image_key=jupyter-base", map[string]any{"sm_percentage": 0}, memberToken, http.StatusBadRequest)
	h.doJSONWithBearer(ideService, http.MethodPost, "/api/v1/ide/start?project_id="+h.projectID()+"&image_key=jupyter-base", map[string]any{"device_class_name": "Bad_Device"}, memberToken, http.StatusBadRequest)

	h.doWithBearer(ideService, http.MethodGet, "/api/v1/ide/images?project_id="+h.projectID(), badToken, http.StatusForbidden)
	h.doWithBearer(ideService, http.MethodGet, "/api/v1/ide?project_id="+h.projectID(), badToken, http.StatusForbidden)
	h.doWithBearer(ideService, http.MethodPost, "/api/v1/ide/start?project_id="+h.projectID()+"&image_key=jupyter-base", badToken, http.StatusForbidden)
	protectedBefore := h.getRecord(ideSessionsResource, protectedPodName).Data["status"]
	h.doWithBearer(ideService, http.MethodPost, "/api/v1/ide/stop?project_id="+h.projectID()+"&type=jupyter", badToken, http.StatusForbidden)
	h.doWithBearer(ideService, http.MethodPost, "/api/v1/ide/delete?project_id="+h.projectID()+"&type=jupyter", badToken, http.StatusForbidden)
	if protectedAfter := h.getRecord(ideSessionsResource, protectedPodName).Data["status"]; protectedAfter != protectedBefore {
		h.t.Fatalf("non-member stop/delete mutated member IDE session: before=%v after=%v", protectedBefore, protectedAfter)
	}
	if _, found := h.store.Get(h.ctx, ideSessionsResource, e2eLowerPodName(badUserID, "jupyter")); found {
		h.t.Fatalf("non-member stop/delete created or mutated caller IDE session")
	}
}
