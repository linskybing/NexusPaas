package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

const (
	pathInternalFastTransferProgress = "/internal/storage/projects/{project_id}/transfers/{targetNamespace}/{name}/progress"

	fastTransferStatusQueued    = "queued"
	fastTransferStatusRunning   = "running"
	fastTransferStatusSucceeded = "succeeded"
	fastTransferStatusFailed    = "failed"
	fastTransferStatusCancelled = "cancelled"
	fastTransferStatusStaged    = "staged"

	fastTransferChangedEvent    = "FastTransferChanged"
	fastTransferQueuedEvent     = "FastTransferQueued"
	fastTransferProgressedEvent = "FastTransferProgressed"
	fastTransferCompletedEvent  = "FastTransferCompleted"
	fastTransferFailedEvent     = "FastTransferFailed"

	fastTransferIdempotencyHeader                    = "Idempotency-Key"
	internalFastTransferIdempotencyKeyHash           = "internal_idempotency_key_hash"
	internalFastTransferIdempotencyFingerprint       = "internal_idempotency_fingerprint_hash"
	defaultFastTransferProgressPct             int   = 0
	defaultFastTransferBytes                   int64 = 0
)

type fastTransferProgressPatch struct {
	status     string
	progress   int
	bytesDone  int64
	bytesTotal int64
	checksum   string
	resume     string
	errorText  string
}

func fastTransferRecord(projectID, userID string, payload map[string]any, repo *recordStoreStorageRepository, now time.Time) map[string]any {
	name := shared.FirstNonBlank(shared.TextValue(payload, "name"), repo.NextFastTransferName())
	namespace := shared.FirstNonBlank(shared.TextValue(payload, "target_namespace", "targetNamespace"), "project-"+projectID)
	record := shared.CloneMap(payload)
	record["id"] = fastTransferID(projectID, namespace, name)
	record["project_id"] = projectID
	record["target_namespace"] = namespace
	record["name"] = name
	record["status"] = fastTransferStatusQueued
	record["progress_pct"] = defaultFastTransferProgressPct
	record["bytes_total"] = fastTransferInt64(payload, "bytes_total", "bytesTotal")
	record["bytes_done"] = defaultFastTransferBytes
	record["checksum"] = shared.TextValue(payload, "checksum")
	record["resume_token"] = shared.TextValue(payload, "resume_token", "resumeToken")
	record["idempotency_key"] = fastTransferIdempotencyKey(nil, payload)
	record["error"] = ""
	record["created_by"] = userID
	record["created_at"] = now.UTC()
	record["updated_at"] = now.UTC()
	return record
}

func fastTransferIdempotencyHashes(r *http.Request, payload map[string]any, projectID, userID string) (string, string) {
	key := fastTransferIdempotencyKey(r, payload)
	if key == "" {
		return "", ""
	}
	fingerprint := shared.CloneMap(payload)
	delete(fingerprint, "idempotency_key")
	delete(fingerprint, "idempotencyKey")
	fingerprint["project_id"] = projectID
	fingerprint["user_id"] = userID
	raw, _ := json.Marshal(fingerprint)
	return fastTransferHash("fast_transfer\x00" + projectID + "\x00" + userID + "\x00" + key), fastTransferHash(string(raw))
}

func fastTransferIdempotencyKey(r *http.Request, payload map[string]any) string {
	key := shared.TextValue(payload, "idempotency_key", "idempotencyKey")
	if key == "" && r != nil {
		key = strings.TrimSpace(r.Header.Get(fastTransferIdempotencyHeader))
	}
	return key
}

func fastTransferHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func fastTransferEventPayload(record map[string]any, action string) map[string]any {
	payload := shared.CloneMap(record)
	payload["action"] = action
	payload["transfer_id"] = shared.TextValue(record, "id")
	return payload
}

func fastTransferCancelPatch(current map[string]any, now time.Time) (map[string]any, int, error) {
	status := normalizeFastTransferStatus(shared.TextValue(current, "status"))
	if !fastTransferCanTransition(status, fastTransferStatusCancelled) {
		return nil, http.StatusConflict, fmt.Errorf("fast transfer cannot transition from %s to cancelled", status)
	}
	return map[string]any{
		"status":     fastTransferStatusCancelled,
		"updated_at": now.UTC(),
	}, http.StatusOK, nil
}

func fastTransferProgressPatchFromPayload(current, payload map[string]any, now time.Time) (map[string]any, string, int, error) {
	patch := fastTransferProgressPatch{
		status:     shared.FirstNonBlank(shared.TextValue(payload, "status"), fastTransferStatusRunning),
		progress:   fastTransferInt(payload, current, "progress_pct", "progressPct"),
		bytesDone:  fastTransferInt64OrCurrent(payload, current, "bytes_done", "bytesDone"),
		bytesTotal: fastTransferInt64OrCurrent(payload, current, "bytes_total", "bytesTotal"),
		checksum:   shared.TextValue(payload, "checksum"),
		resume:     shared.TextValue(payload, "resume_token", "resumeToken"),
		errorText:  shared.TextValue(payload, "error"),
	}
	if normalizeFastTransferStatus(patch.status) == fastTransferStatusSucceeded && !fastTransferHasAny(payload, "progress_pct", "progressPct") {
		patch.progress = 100
	}
	return fastTransferProgressPatchMap(current, patch, now)
}

func fastTransferInt(payload, current map[string]any, keys ...string) int {
	for _, key := range keys {
		if _, ok := payload[key]; ok {
			return shared.IntValue(payload, keys...)
		}
	}
	return shared.IntValue(current, keys...)
}

func fastTransferInt64OrCurrent(payload, current map[string]any, keys ...string) int64 {
	for _, key := range keys {
		if _, ok := payload[key]; ok {
			return fastTransferInt64(payload, keys...)
		}
	}
	return fastTransferInt64(current, keys...)
}

func fastTransferHasAny(data map[string]any, keys ...string) bool {
	for _, key := range keys {
		if _, ok := data[key]; ok {
			return true
		}
	}
	return false
}

func fastTransferProgressPatchMap(current map[string]any, patch fastTransferProgressPatch, now time.Time) (map[string]any, string, int, error) {
	from := normalizeFastTransferStatus(shared.TextValue(current, "status"))
	to := normalizeFastTransferStatus(patch.status)
	if to == "" {
		return nil, "", http.StatusUnprocessableEntity, fmt.Errorf("status is required")
	}
	if !fastTransferCanTransition(from, to) && from != to {
		return nil, "", http.StatusConflict, fmt.Errorf("fast transfer cannot transition from %s to %s", from, to)
	}
	if patch.progress == 0 && to == fastTransferStatusSucceeded {
		patch.progress = 100
	}
	currentProgress := shared.IntValue(current, "progress_pct", "progressPct")
	if patch.progress < currentProgress {
		return nil, "", http.StatusConflict, fmt.Errorf("progress_pct cannot decrease")
	}
	currentBytesDone := fastTransferInt64(current, "bytes_done", "bytesDone")
	if patch.bytesDone < currentBytesDone {
		return nil, "", http.StatusConflict, fmt.Errorf("bytes_done cannot decrease")
	}
	next := map[string]any{
		"status":       to,
		"progress_pct": patch.progress,
		"bytes_done":   patch.bytesDone,
		"updated_at":   now.UTC(),
	}
	if patch.bytesTotal > 0 {
		next["bytes_total"] = patch.bytesTotal
	}
	if patch.checksum != "" {
		next["checksum"] = patch.checksum
	}
	if patch.resume != "" {
		next["resume_token"] = patch.resume
	}
	if patch.errorText != "" || to == fastTransferStatusFailed {
		next["error"] = patch.errorText
	}
	return next, fastTransferTransitionEvent(to), http.StatusOK, nil
}

func normalizeFastTransferStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "", fastTransferStatusQueued:
		return fastTransferStatusQueued
	case fastTransferStatusRunning:
		return fastTransferStatusRunning
	case fastTransferStatusSucceeded:
		return fastTransferStatusSucceeded
	case fastTransferStatusFailed:
		return fastTransferStatusFailed
	case fastTransferStatusCancelled:
		return fastTransferStatusCancelled
	case fastTransferStatusStaged:
		return fastTransferStatusStaged
	default:
		return strings.ToLower(strings.TrimSpace(status))
	}
}

func fastTransferCanTransition(from, to string) bool {
	from = normalizeFastTransferStatus(from)
	to = normalizeFastTransferStatus(to)
	if from == to && (to == fastTransferStatusRunning || to == fastTransferStatusQueued) {
		return true
	}
	switch from {
	case fastTransferStatusQueued:
		return to == fastTransferStatusRunning || to == fastTransferStatusCancelled
	case fastTransferStatusRunning:
		return to == fastTransferStatusSucceeded || to == fastTransferStatusFailed || to == fastTransferStatusCancelled
	case fastTransferStatusStaged:
		return to == fastTransferStatusCancelled
	default:
		return false
	}
}

func fastTransferTransitionEvent(status string) string {
	switch normalizeFastTransferStatus(status) {
	case fastTransferStatusRunning:
		return fastTransferProgressedEvent
	case fastTransferStatusSucceeded:
		return fastTransferCompletedEvent
	case fastTransferStatusFailed:
		return fastTransferFailedEvent
	default:
		return fastTransferChangedEvent
	}
}

func fastTransferInt64(data map[string]any, keys ...string) int64 {
	for _, key := range keys {
		value, ok := data[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case int:
			return int64(typed)
		case int64:
			return typed
		case float64:
			return int64(typed)
		case json.Number:
			n, _ := typed.Int64()
			return n
		}
	}
	return 0
}
