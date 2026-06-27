package schedulerquota

import (
	"context"
	"log/slog"
	"strings"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

// Cross-service resource keys the submit-admission evaluator depends on. They
// are owned by other services (workload-service, org-project-service); the
// scheduler may only *read* them. Keeping the keys in this one file localizes
// the cross-service data coupling (problem.md §3 / §4 DIP) to a single typed
// seam instead of scattering raw shared-store string keys through the admission
// logic. Full data-ownership moves to typed per-aggregate repositories under
// GAP-4; until then this interface is the boundary other services break against
// if their schema changes.
const (
	workloadJobsResource   = "workload-service:jobs"
	userQuotasResource     = "org-project-service:user_quotas"
	projectMembersResource = "org-project-service:project_members"
	userGroupsResource     = "org-project-service:user_groups"
)

type admissionRecord = contracts.Record[map[string]any]

// admissionReader exposes the records the submit-admission evaluator reads as
// named, typed lookups. The evaluator depends on this interface rather than on
// platform.RecordStore + cross-service string keys, so the coupling is explicit
// and substitutable (e.g. by service-owned read clients once typed repos land).
type admissionReader interface {
	Project(ctx context.Context, projectID string) (admissionRecord, bool)
	ListProjects(ctx context.Context) []admissionRecord
	Plan(ctx context.Context, planID string) (admissionRecord, bool)
	Queue(ctx context.Context, queueID string) (admissionRecord, bool)
	ListQueues(ctx context.Context) []admissionRecord
	ListNetworkProfiles(ctx context.Context) []admissionRecord
	ProjectMember(ctx context.Context, key string) (admissionRecord, bool)
	ListProjectMembers(ctx context.Context) []admissionRecord
	ListUserGroups(ctx context.Context) []admissionRecord
	UserQuota(ctx context.Context, key string) (admissionRecord, bool)
	ListUserQuotas(ctx context.Context) []admissionRecord
	ListWorkloadJobs(ctx context.Context) []admissionRecord
}

// storeAdmissionReader is the co-hosted/local implementation of admissionReader.
// It is the only place that maps the typed reads to concrete resource keys.
type storeAdmissionReader struct {
	store     platform.RecordStore
	scheduler *recordStoreSchedulerQuotaRepository
}

// newAdmissionReader wraps a local store as an admissionReader. A nil store
// yields a reader whose lookups all miss, matching the evaluator's fail-closed
// "store not configured" handling. Isolated production handlers should use
// newAdmissionReaderForApp so foreign resources resolve through owner contracts.
func newAdmissionReader(store platform.RecordStore) admissionReader {
	return storeAdmissionReader{store: store, scheduler: schedulerQuotaRepositoryFromStore(store)}
}

func newAdmissionReaderForApp(app *platform.App) admissionReader {
	if app == nil {
		return newAdmissionReader(nil)
	}
	local := storeAdmissionReader{store: app.Store, scheduler: schedulerQuotaRepositoryForApp(app)}
	if app.Config.ServiceName == "" || app.Config.ServiceName == "all" {
		return local
	}
	return ownerReadAdmissionReader{
		local: local,
		cfg:   app.Config,
		owner: platform.NewRemoteServiceReader(app.Config),
	}
}

func (rdr storeAdmissionReader) get(ctx context.Context, resource, id string) (admissionRecord, bool) {
	if rdr.store == nil {
		return admissionRecord{}, false
	}
	return rdr.store.Get(ctx, resource, id)
}

func (rdr storeAdmissionReader) list(ctx context.Context, resource string) []admissionRecord {
	if rdr.store == nil {
		return nil
	}
	return rdr.store.List(ctx, resource)
}

func (rdr storeAdmissionReader) Project(ctx context.Context, projectID string) (admissionRecord, bool) {
	return rdr.get(ctx, projectsResource, projectID)
}

func (rdr storeAdmissionReader) ListProjects(ctx context.Context) []admissionRecord {
	return rdr.list(ctx, projectsResource)
}

func (rdr storeAdmissionReader) Plan(ctx context.Context, planID string) (admissionRecord, bool) {
	if rdr.scheduler == nil {
		return admissionRecord{}, false
	}
	return rdr.scheduler.GetPlan(ctx, planID)
}

func (rdr storeAdmissionReader) Queue(ctx context.Context, queueID string) (admissionRecord, bool) {
	if rdr.scheduler == nil {
		return admissionRecord{}, false
	}
	return rdr.scheduler.GetQueue(ctx, queueID)
}

func (rdr storeAdmissionReader) ListQueues(ctx context.Context) []admissionRecord {
	if rdr.scheduler == nil {
		return nil
	}
	return rdr.scheduler.ListQueues(ctx)
}

func (rdr storeAdmissionReader) ListNetworkProfiles(ctx context.Context) []admissionRecord {
	return rdr.list(ctx, networkProfilesResource)
}

func (rdr storeAdmissionReader) ProjectMember(ctx context.Context, key string) (admissionRecord, bool) {
	return rdr.get(ctx, projectMembersResource, key)
}

func (rdr storeAdmissionReader) ListProjectMembers(ctx context.Context) []admissionRecord {
	return rdr.list(ctx, projectMembersResource)
}

func (rdr storeAdmissionReader) ListUserGroups(ctx context.Context) []admissionRecord {
	return rdr.list(ctx, userGroupsResource)
}

func (rdr storeAdmissionReader) UserQuota(ctx context.Context, key string) (admissionRecord, bool) {
	return rdr.get(ctx, userQuotasResource, key)
}

func (rdr storeAdmissionReader) ListUserQuotas(ctx context.Context) []admissionRecord {
	return rdr.list(ctx, userQuotasResource)
}

func (rdr storeAdmissionReader) ListWorkloadJobs(ctx context.Context) []admissionRecord {
	return rdr.list(ctx, workloadJobsResource)
}

type ownerReadAdmissionReader struct {
	local storeAdmissionReader
	cfg   platform.Config
	owner platform.CrossServiceReader
}

func (rdr ownerReadAdmissionReader) Project(ctx context.Context, projectID string) (admissionRecord, bool) {
	return rdr.getOwner(ctx, projectsResource, projectID)
}

func (rdr ownerReadAdmissionReader) ListProjects(ctx context.Context) []admissionRecord {
	return rdr.listOwner(ctx, projectsResource)
}

func (rdr ownerReadAdmissionReader) Plan(ctx context.Context, planID string) (admissionRecord, bool) {
	return rdr.local.Plan(ctx, planID)
}

func (rdr ownerReadAdmissionReader) Queue(ctx context.Context, queueID string) (admissionRecord, bool) {
	return rdr.local.Queue(ctx, queueID)
}

func (rdr ownerReadAdmissionReader) ListQueues(ctx context.Context) []admissionRecord {
	return rdr.local.ListQueues(ctx)
}

func (rdr ownerReadAdmissionReader) ListNetworkProfiles(ctx context.Context) []admissionRecord {
	return rdr.local.ListNetworkProfiles(ctx)
}

func (rdr ownerReadAdmissionReader) ProjectMember(ctx context.Context, key string) (admissionRecord, bool) {
	return rdr.getOwner(ctx, projectMembersResource, key)
}

func (rdr ownerReadAdmissionReader) ListProjectMembers(ctx context.Context) []admissionRecord {
	return rdr.listOwner(ctx, projectMembersResource)
}

func (rdr ownerReadAdmissionReader) ListUserGroups(ctx context.Context) []admissionRecord {
	return rdr.listOwner(ctx, userGroupsResource)
}

func (rdr ownerReadAdmissionReader) UserQuota(ctx context.Context, key string) (admissionRecord, bool) {
	return rdr.getOwner(ctx, userQuotasResource, key)
}

func (rdr ownerReadAdmissionReader) ListUserQuotas(ctx context.Context) []admissionRecord {
	return rdr.listOwner(ctx, userQuotasResource)
}

func (rdr ownerReadAdmissionReader) ListWorkloadJobs(ctx context.Context) []admissionRecord {
	return rdr.listOwner(ctx, workloadJobsResource)
}

func (rdr ownerReadAdmissionReader) getOwner(ctx context.Context, resource, id string) (admissionRecord, bool) {
	if rdr.isLocalOwner(resource) {
		return rdr.local.get(ctx, resource, id)
	}
	if rdr.owner == nil {
		return admissionRecord{}, false
	}
	record, found, err := rdr.owner.Get(ctx, resource, id)
	if err != nil {
		slog.Error("scheduler quota owner read get failed", "resource", resource, "id", id, "error", err)
		return admissionRecord{}, false
	}
	return record, found
}

func (rdr ownerReadAdmissionReader) listOwner(ctx context.Context, resource string) []admissionRecord {
	if rdr.isLocalOwner(resource) {
		return rdr.local.list(ctx, resource)
	}
	if rdr.owner == nil {
		return nil
	}
	records, err := rdr.owner.List(ctx, resource)
	if err != nil {
		slog.Error("scheduler quota owner read list failed", "resource", resource, "error", err)
		return nil
	}
	return records
}

func (rdr ownerReadAdmissionReader) isLocalOwner(resource string) bool {
	owner, _, found := strings.Cut(resource, ":")
	return !found || rdr.cfg.AllowsService(owner)
}
