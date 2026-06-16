# image-registry-service

Category: Supply Chain | Phase: 2

## 1. Overview

The container image supply-chain service. Responsible for image requests/review, project allow-lists, image builds (three sources: archive upload, storage, Dockerfile), build logs, catalog publish/sync, the Harbor admin API, and the secondary Harbor GPU23 lane. This service owns the Harbor API integration; the UI proxy belongs to integration-proxy-service.

## 2. Functional Requirements

| ID | Requirement | Acceptance/Notes |
| --- | --- | --- |
| FR-IMAGE-01 | Support listing, adding, and removing project allowed images. | /projects/{id}/images. |
| FR-IMAGE-02 | Support user-submitted image requests; admins can list globally, batch update, approve, or reject. | /image-requests. |
| FR-IMAGE-03 | Support image builds from uploaded archives, storage, or Dockerfiles. | /images/build, /from-storage, /dockerfile. |
| FR-IMAGE-04 | Provide build logs, project build lists, and project build deletion. | Build job names must be checked back against project permissions. |
| FR-IMAGE-05 | Support image catalog list, sync, publish, unpublish, delete, and sync status. | /image-catalog. |
| FR-IMAGE-06 | Maintain governance state: repository/tag/sync targets, artifact origin/status, delivery mode, allow lists. | Derived from domain/image and migrations. |
| FR-IMAGE-07 | Support Harbor reverse proxy, Harbor status/statistics/projects, and the secondary Harbor GPU23 lane. | /harbor, /harbor-gpu23 (UI proxy portion handed to integration-proxy). |
| FR-IMAGE-08 | Sync image access or pull-secret state when project or group membership changes. | Derived from imageService.SetGroupRepo, SetBuildAccessRepos. |

## 3. Owned Data

`container_repositories`, `container_tags`, `sync_targets`, `image_allow_lists`, `image_requests`, `image_build_jobs`.

## 4. Current Code/Route Mapping

- Handlers: `image.go`, `harbor.go` (API integration portion)
- Application: `application/image`
- Domain: `domain/image`
- Routes: `/api/v1/image-requests`, `/api/v1/images/*`, `/api/v1/image-catalog`, `/api/v1/projects/{id}/images`, `/api/v1/harbor-status`

## 5. Dependencies

| Dependency | Purpose |
| --- | --- |
| Harbor (primary lane + GPU23 lane) | Registry operations, status/statistics |
| k8s-control-service | Actual execution of image build jobs |
| org-project-service | Membership-driven sync of image access / pull secrets |
| storage-service | from-storage build sources |

## 6. Events

| Direction | Event | Counterpart | Purpose |
| --- | --- | --- | --- |
| Publish | ImageRequested / ImageApproved / ImageBuildStarted / ImageBuilt / ImagePublished / ImageSyncFailed | workload, audit, notification, org-project | Update allow lists and build status |
| Subscribe | GroupMembershipChanged / ProjectCreated/Deleted | org-project | Sync image access and pull-secret state |

## 7. Non-Functional Highlights

- Harbor calls need timeout/retry/circuit breaker; when Harbor is unavailable, build/sync shows clear degradation (NFR-RES-01/02).
- Image builds are long-running, using async status + build log streaming (NFR-PERF-01).
- Image build/publish must have saga state, compensation, and retry strategies (acceptance criterion).
- Allow-list read models must be quickly invalidated after membership changes (NFR-MTEN-02).

## 8. Decomposition Notes

Extracted in phase 2. The Harbor UI reverse proxy is handed to integration-proxy-service; this service keeps only the Harbor admin/API integration and governance state.
