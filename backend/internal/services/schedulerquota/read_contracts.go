package schedulerquota

import (
	"context"

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
	Plan(ctx context.Context, planID string) (admissionRecord, bool)
	Queue(ctx context.Context, queueID string) (admissionRecord, bool)
	ListQueues(ctx context.Context) []admissionRecord
	ProjectMember(ctx context.Context, key string) (admissionRecord, bool)
	ListProjectMembers(ctx context.Context) []admissionRecord
	ListUserGroups(ctx context.Context) []admissionRecord
	UserQuota(ctx context.Context, key string) (admissionRecord, bool)
	ListUserQuotas(ctx context.Context) []admissionRecord
	ListWorkloadJobs(ctx context.Context) []admissionRecord
}

// storeAdmissionReader is the shared-store-backed implementation of
// admissionReader. It is the only place that maps the typed reads to concrete
// resource keys.
type storeAdmissionReader struct {
	store     platform.RecordStore
	scheduler schedulerQuotaRepository
}

// newAdmissionReader wraps a shared store as an admissionReader. A nil store
// yields a reader whose lookups all miss, matching the evaluator's
// fail-closed "store not configured" handling.
func newAdmissionReader(store platform.RecordStore) admissionReader {
	return storeAdmissionReader{store: store, scheduler: schedulerQuotaRepositoryFromStore(store)}
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
