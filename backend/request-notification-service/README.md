# request-notification-service

Category: Collaboration | Phase: 1

## 1. Overview

The forms/requests, in-app notifications, and announcements service. Responsible for forms/requests, form messages, notifications, announcements, unread counts, and mark-read. Performs project access checks against org-project-service (event snapshots + synchronous queries for critical operations).

## 2. Functional Requirements

| ID | Requirement | Acceptance/Notes |
| --- | --- | --- |
| FR-COMM-01 | Support users creating forms/requests, listing my forms, and querying form details. | forms may carry an optional project_id and must validate project access. |
| FR-COMM-02 | Support admins listing all forms, batch status updates, and single-form status updates. | /forms/batch/status. |
| FR-COMM-03 | Support form message creation and queries, with visibility controlled by ownership/system role. | /forms/{id}/messages. |
| FR-COMM-04 | Support marking notifications as read, mark-all-read, and clear-all. | /notifications. |
| FR-COMM-05 | Support announcement creation, paginated listing, update, delete, active list, unread count, and mark read. | /announcements and /admin/announcements. |

## 3. Owned Data

`forms`, `form_messages`, `notifications`, `announcements`, `announcement_reads`.

## 4. Current Code/Route Mapping

- Handlers: `form.go`, `notification.go`, `announcement.go`
- Domain: `domain/form`, `domain/announcement`
- Routes: `/api/v1/forms`, `/api/v1/notifications`, `/api/v1/announcements`, `/api/v1/admin/announcements`

## 5. Events

| Direction | Event | Counterpart | Purpose |
| --- | --- | --- | --- |
| Subscribe | NotificationRequested | any service | Generate in-app notifications (job completed, image approved, storage transfer finished, etc.) |
| Publish | AnnouncementPublished | platform-gateway | Unread count push |
| Subscribe | UserCreated/Updated, ProjectDeleted | identity, org-project | Recipient snapshots and cleanup of form project references |

## 6. Dependencies

| Dependency | Purpose |
| --- | --- |
| org-project-service | Project access validation for forms |
| identity-service | Ownership/system-role visibility decisions (via the Authz SDK) |
| media-upload-service | Form image attachments (stores media IDs only, never raw files) |

## 7. Non-Functional Highlights

- Notification workers need horizontal sharding or mutual-exclusion locks (NFR-SCALE-02).
- Form status changes and announcement administration must produce AuditEvents (NFR-SEC-05).
- A non-core service that may degrade independently (NFR-AVAIL-01).

## 8. Decomposition Notes

Extracted in phase 1. media-upload-service is small and may initially co-deploy with this service while keeping code and data boundaries.
