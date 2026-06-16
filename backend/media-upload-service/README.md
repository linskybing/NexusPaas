# media-upload-service

Category: Support | Phase: 1

## 1. Overview

The media upload service. Responsible for image uploads, image serving (JWT-only routes), a service-owned media storage boundary, and form/preview media management. A small service that may initially co-deploy with request-notification-service while keeping code and data boundaries.

## 2. Functional Requirements

| ID | Requirement | Acceptance/Notes |
| --- | --- | --- |
| FR-COMM-06 | Support image uploads and JWT-only image-serving routes for browser scenarios such as forms/previews. | /uploads/images. |

## 3. Owned Data

`uploaded_media` metadata, object keys, checksums, owner references, and the current small-image blob payloads stored through the shared `RecordStore` port.

## 4. Current Code/Route Mapping

- Handlers: `internal/services/mediaupload/handler.go`
- Application: `mediaupload.Service`
- Repository: service-owned `RecordStore` resource `media-upload-service:uploaded_media`
- Routes: `/api/v1/uploads/images/*`

## 5. Dependencies

| Dependency | Purpose |
| --- | --- |
| `RecordStore` / Postgres | Durable uploaded media metadata and current small-image blob payloads |
| `OBJECT_STORE_*` / MinIO or S3 | Production blob persistence for uploaded media objects; provision the bucket with `ADMIN_TASK=ensure-object-store-bucket` before serving startup |
| authorization-policy-service | Access decisions for image-serving routes (owner/domain) |

## 6. Design Notes

- Upload validation: file-type allow-list, size limits, checksum computation and storage (input validation per the security NFRs).
- Image serving is a JWT-only route (browser `<img>` tags cannot send custom headers) but must still pass RBAC (NFR-SEC-04).
- Media is linked via owner references (user_id/form_id); other services store only media IDs and never access the media storage boundary directly.
- A dedicated MinIO/S3 bucket remains the intended production hardening path when upload volume, object size, lifecycle policies, or CDN integration outgrow JSONB-backed small-image storage.
- Orphaned media cleanup: subscribe to ProjectDeleted/form-deletion events for retention cleanup.

## 7. Non-Functional Highlights

- Future MinIO/S3 calls need timeout/retry/circuit breaker (NFR-RES-01).
- Future object-storage secrets are managed via Kubernetes Secret and must be rotatable (NFR-SEC-03).
- A support service that may degrade independently (NFR-AVAIL-01).

## 8. Decomposition Notes

Extracted in phase 1 (or co-deployed with request-notification-service). The lowest-risk pilot for validating Gateway forwarding, JWT-only routes, and the independent service deployment pipeline.
