//go:build e2e

package e2e

import (
	"net/http"
	"strings"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
)

const (
	workloadConfigVersionsResource   = "workload-service:configfiles:versions"
	workloadInstanceCommandsResource = "workload-service:instances:commands"
	workloadJobLogsResource          = "workload-service:job_logs"
	workloadJobGPUUsageResource      = "workload-service:job_gpu_usage"
	workloadJobCommandsResource      = "workload-service:jobs:commands"
)

func TestWorkloadConfigFileLifecycleE2E(t *testing.T) {
	h := newHarnessWithPeers(t, map[string][]string{
		schedulerQuotaService: {orgProjectService, workloadService},
		workloadService:       {identityService, orgProjectService, schedulerQuotaService},
	}, identityService, orgProjectService, schedulerQuotaService, workloadService)
	ids := h.seedIdentityContracts()
	h.seedSchedulerAdmissionData(ids.userID)
	badUserID := "badcfg" + h.runID
	badToken := h.seedAPIUser(badUserID, "bad-cfg-"+h.runID, false)

	suffix := e2eSuffix(h.runID)
	configID := "cfg-life-" + suffix
	commitID := "cfg-commit-" + suffix
	jobID := "cfg-job-" + suffix
	deniedVersionID := "cfg-deny-version-" + suffix
	deniedJobID := "cfg-deny-job-" + suffix

	lifecycle := workloadConfigLifecycleIDs{
		configID:        configID,
		commitID:        commitID,
		jobID:           jobID,
		deniedVersionID: deniedVersionID,
		deniedJobID:     deniedJobID,
		logID:           "log-" + suffix,
		gpuID:           "gpu-" + suffix,
	}
	h.createAndCommitConfigFile(ids.apiToken, lifecycle)
	h.assertConfigFileReadbacks(ids.apiToken, lifecycle)
	h.exerciseConfigInstanceCommands(ids.apiToken, configID)
	h.submitConfigCommitJob(ids, lifecycle)
	h.assertJobReadCancel(ids.apiToken, lifecycle)
	h.assertWorkloadNonMemberGuards(badToken, configID, commitID, deniedVersionID, jobID, deniedJobID)
	h.doWithBearer(workloadService, http.MethodDelete, "/api/v1/configfiles/"+configID, ids.apiToken, http.StatusOK)
	if _, found := h.store.Get(h.ctx, workloadConfigsResource, configID); found {
		t.Fatalf("deleted config %s still exists", configID)
	}
	h.requireWorkloadLifecycleEvents(lifecycle)
}

type workloadConfigLifecycleIDs struct {
	configID        string
	commitID        string
	jobID           string
	deniedVersionID string
	deniedJobID     string
	logID           string
	gpuID           string
}

func (h *e2eHarness) createAndCommitConfigFile(token string, ids workloadConfigLifecycleIDs) {
	h.t.Helper()
	created := h.doJSONWithBearer(workloadService, http.MethodPost, "/api/v1/configfiles?project_id="+h.projectID(), map[string]any{
		"id":         ids.configID,
		"name":       "train.yaml",
		"path":       "jobs/train.yaml",
		"content":    "apiVersion: batch/v1\nkind: Job\nmetadata:\n  name: train\n",
		"message":    "initial",
		"e2e_run_id": h.runID,
	}, token, http.StatusCreated)
	h.requireEnvelopeCorrelation(created)
	if data := e2eResponseRecordData(h.t, created); data["project_id"] != h.projectID() || data["name"] != "train.yaml" {
		h.t.Fatalf("created config = %#v, want project/name", data)
	}
	updated := h.doJSONWithBearer(workloadService, http.MethodPut, "/api/v1/configfiles/"+ids.configID, map[string]any{
		"content":    "apiVersion: batch/v1\nkind: Job\nmetadata:\n  name: train-updated\n",
		"message":    "updated",
		"e2e_run_id": h.runID,
	}, token, http.StatusOK)
	h.requireEnvelopeCorrelation(updated)
	if data := e2eResponseRecordData(h.t, updated); !strings.Contains(textE2E(data["content"]), "train-updated") {
		h.t.Fatalf("updated config = %#v, want updated content", data)
	}
	committed := h.doJSONWithBearer(workloadService, http.MethodPost, "/api/v1/configfiles/"+ids.configID+"/versions", map[string]any{
		"id":         ids.commitID,
		"content":    "apiVersion: batch/v1\nkind: Job\nmetadata:\n  name: train-committed\n",
		"message":    "release",
		"e2e_run_id": h.runID,
	}, token, http.StatusCreated)
	h.requireEnvelopeCorrelation(committed)
	if data := e2eResponseRecordData(h.t, committed); data["config_id"] != ids.configID || data["immutable"] != true || data["sha256"] == "" {
		h.t.Fatalf("committed version = %#v, want immutable version for config", data)
	}
}

func (h *e2eHarness) assertConfigFileReadbacks(token string, ids workloadConfigLifecycleIDs) {
	h.t.Helper()
	versions := e2eDataRecords(h.t, h.doWithBearer(workloadService, http.MethodGet, "/api/v1/configfiles/"+ids.configID+"/versions", token, http.StatusOK))
	if !e2eRecordsContainID(versions, ids.commitID) || len(versions) < 3 {
		h.t.Fatalf("versions = %#v, want initial/update/commit including %s", versions, ids.commitID)
	}
	tree := h.doWithBearer(workloadService, http.MethodGet, "/api/v1/configfiles/tree", token, http.StatusOK).dataMap(h.t)
	if !e2eTreeContainsID(tree, ids.configID) {
		h.t.Fatalf("global config tree = %#v, want config %s", tree, ids.configID)
	}
	projectTree := h.doWithBearer(workloadService, http.MethodGet, "/api/v1/configfiles/project/"+h.projectID()+"/tree", token, http.StatusOK).dataMap(h.t)
	if projectTree["project_id"] != h.projectID() || !e2eTreeContainsID(projectTree, ids.configID) {
		h.t.Fatalf("project config tree = %#v, want config %s", projectTree, ids.configID)
	}
	projectConfigs := e2eDataRecords(h.t, h.doWithBearer(workloadService, http.MethodGet, "/api/v1/projects/"+h.projectID()+"/config-files", token, http.StatusOK))
	if !e2eRecordsContainID(projectConfigs, ids.configID) {
		h.t.Fatalf("project config files = %#v, want %s", projectConfigs, ids.configID)
	}
	history := e2eDataRecords(h.t, h.doWithBearer(workloadService, http.MethodGet, "/api/v1/configfiles/project/"+h.projectID()+"/history", token, http.StatusOK))
	if !e2eRecordsContainID(history, ids.commitID) {
		h.t.Fatalf("project history = %#v, want committed version %s", history, ids.commitID)
	}
}

func (h *e2eHarness) exerciseConfigInstanceCommands(token, configID string) {
	h.t.Helper()
	h.doJSONWithBearer(workloadService, http.MethodPost, "/api/v1/configfiles/"+configID+"/instance", map[string]any{
		"reason":     "smoke",
		"e2e_run_id": h.runID,
	}, token, http.StatusAccepted)
	h.doWithBearer(workloadService, http.MethodGet, "/api/v1/configfiles/"+configID+"/instance/pods", token, http.StatusOK)
	h.doJSONWithBearer(workloadService, http.MethodDelete, "/api/v1/configfiles/"+configID+"/instance", map[string]any{
		"reason":     "smoke",
		"e2e_run_id": h.runID,
	}, token, http.StatusAccepted)
}

func (h *e2eHarness) submitConfigCommitJob(identity identityIDs, ids workloadConfigLifecycleIDs) {
	h.t.Helper()
	submitted := h.doJSONWithBearer(workloadService, http.MethodPost, "/api/v1/jobs", map[string]any{
		"job_id":           ids.jobID,
		"config_commit_id": ids.commitID,
		"queue_name":       h.queueName(),
		"required_cpu":     0.1,
		"required_memory":  64,
		"e2e_run_id":       h.runID,
	}, identity.apiToken, http.StatusCreated)
	h.requireEnvelopeCorrelation(submitted)
	jobData := e2eResponseRecordData(h.t, submitted)
	if jobData["project_id"] != h.projectID() || jobData["config_id"] != ids.configID || jobData["config_commit_id"] != ids.commitID || jobData["user_id"] != identity.userID {
		h.t.Fatalf("submitted job = %#v, want config commit derived project/config/user", jobData)
	}
	h.updateRecord(schedulerAdmissionsResource, h.projectID()+"/"+identity.userID+"/"+h.queueName(), map[string]any{})
}

func (h *e2eHarness) assertJobReadCancel(token string, ids workloadConfigLifecycleIDs) {
	h.t.Helper()
	h.createRecord(workloadJobLogsResource, ids.logID, map[string]any{"job_id": ids.jobID, "line": "queued"})
	h.createRecord(workloadJobGPUUsageResource, ids.gpuID, map[string]any{"job_id": ids.jobID, "gpu_utilization": 42})
	logs := e2eDataRecords(h.t, h.doWithBearer(workloadService, http.MethodGet, "/api/v1/jobs/"+ids.jobID+"/logs", token, http.StatusOK))
	if !e2eRecordsContainID(logs, ids.logID) {
		h.t.Fatalf("job logs = %#v, want seeded log", logs)
	}
	gpu := e2eDataRecords(h.t, h.doWithBearer(workloadService, http.MethodGet, "/api/v1/jobs/"+ids.jobID+"/gpu-summary", token, http.StatusOK))
	if !e2eRecordsContainID(gpu, ids.gpuID) {
		h.t.Fatalf("job GPU usage = %#v, want seeded usage", gpu)
	}
	beforeCommands := len(h.listRecords(workloadJobCommandsResource))
	h.doJSONWithBearer(workloadService, http.MethodPost, "/api/v1/jobs/"+ids.jobID+"/cancel", map[string]any{
		"reason":     "user requested",
		"e2e_run_id": h.runID,
	}, token, http.StatusAccepted)
	if afterCommands := len(h.listRecords(workloadJobCommandsResource)); afterCommands != beforeCommands+1 {
		h.t.Fatalf("job cancel command count = %d, want %d", afterCommands, beforeCommands+1)
	}
}

func (h *e2eHarness) requireWorkloadLifecycleEvents(ids workloadConfigLifecycleIDs) {
	h.t.Helper()
	h.requireCorrelatedEvent("ConfigFileChanged", func(event contracts.Event) bool {
		return event.Source == workloadService && event.Data["id"] == ids.configID && event.Data["action"] == "created"
	})
	h.requireCorrelatedEvent("ConfigCommitted", func(event contracts.Event) bool {
		return event.Source == workloadService && event.Data["id"] == ids.commitID
	})
	h.requireCorrelatedEvent("SubmitAdmissionReviewed", func(event contracts.Event) bool {
		return event.Source == schedulerQuotaService && event.Data["project_id"] == h.projectID()
	})
	h.requireCorrelatedEvent("JobSubmitted", func(event contracts.Event) bool {
		return event.Source == workloadService && event.Data["job_id"] == ids.jobID
	})
	h.requireCorrelatedEvent("JobCancelRequested", func(event contracts.Event) bool {
		return event.Source == workloadService && event.Data["job_id"] == ids.jobID
	})
}

func (h *e2eHarness) assertWorkloadNonMemberGuards(badToken, configID, commitID, deniedVersionID, jobID, deniedJobID string) {
	h.t.Helper()
	e2eRequireNoRecords(h.t, e2eDataRecords(h.t, h.doWithBearer(workloadService, http.MethodGet, "/api/v1/configfiles", badToken, http.StatusOK)), "non-member config list")
	e2eRequireNoRecords(h.t, e2eDataRecords(h.t, h.doWithBearer(workloadService, http.MethodGet, "/api/v1/jobs", badToken, http.StatusOK)), "non-member job list")
	h.doWithBearer(workloadService, http.MethodGet, "/api/v1/configfiles/"+configID, badToken, http.StatusForbidden)
	h.doWithBearer(workloadService, http.MethodGet, "/api/v1/configfiles/"+configID+"/versions", badToken, http.StatusForbidden)
	h.doWithBearer(workloadService, http.MethodGet, "/api/v1/configfiles/project/"+h.projectID(), badToken, http.StatusForbidden)
	h.doWithBearer(workloadService, http.MethodGet, "/api/v1/projects/"+h.projectID()+"/config-files", badToken, http.StatusForbidden)
	originalConfig := h.getRecord(workloadConfigsResource, configID)
	originalContent := textE2E(originalConfig.Data["content"])
	h.doJSONWithBearer(workloadService, http.MethodPut, "/api/v1/configfiles/"+configID, map[string]any{"content": "forged"}, badToken, http.StatusForbidden)
	h.doJSONWithBearer(workloadService, http.MethodPatch, "/api/v1/configfiles/"+configID, map[string]any{"content": "forged-patch"}, badToken, http.StatusForbidden)
	h.doWithBearer(workloadService, http.MethodDelete, "/api/v1/configfiles/"+configID, badToken, http.StatusForbidden)
	if config := h.getRecord(workloadConfigsResource, configID); textE2E(config.Data["content"]) != originalContent {
		h.t.Fatalf("non-member config write/delete mutated config: before=%q after=%#v", originalContent, config.Data)
	}
	h.doJSONWithBearer(workloadService, http.MethodPost, "/api/v1/configfiles/"+configID+"/versions", map[string]any{
		"id":      deniedVersionID,
		"content": "forged",
	}, badToken, http.StatusForbidden)
	if _, found := h.store.Get(h.ctx, workloadConfigVersionsResource, deniedVersionID); found {
		h.t.Fatalf("non-member version commit persisted %s", deniedVersionID)
	}
	h.doWithBearer(workloadService, http.MethodGet, "/api/v1/configfiles/project/"+h.projectID()+"/tree", badToken, http.StatusForbidden)
	h.doWithBearer(workloadService, http.MethodGet, "/api/v1/configfiles/project/"+h.projectID()+"/history", badToken, http.StatusForbidden)
	beforeInstanceCommands := len(h.listRecords(workloadInstanceCommandsResource))
	h.doJSONWithBearer(workloadService, http.MethodPost, "/api/v1/configfiles/"+configID+"/instance", map[string]any{"reason": "forged"}, badToken, http.StatusForbidden)
	h.doJSONWithBearer(workloadService, http.MethodDelete, "/api/v1/configfiles/"+configID+"/instance", map[string]any{"reason": "forged"}, badToken, http.StatusForbidden)
	h.doWithBearer(workloadService, http.MethodGet, "/api/v1/configfiles/"+configID+"/instance/pods", badToken, http.StatusForbidden)
	if afterInstanceCommands := len(h.listRecords(workloadInstanceCommandsResource)); afterInstanceCommands != beforeInstanceCommands {
		h.t.Fatalf("non-member instance commands = %d, want unchanged %d", afterInstanceCommands, beforeInstanceCommands)
	}
	beforeAdmissions := len(h.listRecords(schedulerAdmissionsResource))
	h.doJSONWithBearer(workloadService, http.MethodPost, "/api/v1/jobs", map[string]any{
		"job_id":           deniedJobID,
		"config_commit_id": commitID,
		"queue_name":       h.queueName(),
		"required_cpu":     0.1,
		"required_memory":  64,
	}, badToken, http.StatusForbidden)
	if _, found := h.store.Get(h.ctx, workloadJobsResource, deniedJobID); found {
		h.t.Fatalf("non-member submit persisted job %s", deniedJobID)
	}
	if afterAdmissions := len(h.listRecords(schedulerAdmissionsResource)); afterAdmissions != beforeAdmissions {
		h.t.Fatalf("non-member submit admissions = %d, want unchanged %d", afterAdmissions, beforeAdmissions)
	}
	h.doWithBearer(workloadService, http.MethodGet, "/api/v1/jobs/"+jobID, badToken, http.StatusForbidden)
	beforeCommands := len(h.listRecords(workloadJobCommandsResource))
	h.doJSONWithBearer(workloadService, http.MethodPost, "/api/v1/jobs/"+jobID+"/cancel", map[string]any{"reason": "forged"}, badToken, http.StatusForbidden)
	if afterCommands := len(h.listRecords(workloadJobCommandsResource)); afterCommands != beforeCommands {
		h.t.Fatalf("non-member cancel commands = %d, want unchanged %d", afterCommands, beforeCommands)
	}
	for _, path := range []string{"/logs", "/gpu-summary", "/gpu-timeline", "/gpu-breakdown"} {
		h.doWithBearer(workloadService, http.MethodGet, "/api/v1/jobs/"+jobID+path, badToken, http.StatusForbidden)
	}
}

func e2eTreeContainsID(tree map[string]any, id string) bool {
	nodes, _ := tree["nodes"].([]any)
	for _, item := range nodes {
		node, _ := item.(map[string]any)
		if textE2E(node["id"]) == id {
			return true
		}
	}
	return false
}
