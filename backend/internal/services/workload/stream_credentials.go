package workload

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

func streamCredentials(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	payload, err := platform.DecodeMapWithError(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(msgInvalidBody), nil
	}
	jobID := shared.TextValue(payload, "job_id", "jobId", "id")
	if jobID == "" {
		return http.StatusBadRequest, shared.ErrorData("job_id is required"), nil
	}
	record, status, data, ok := streamCredentialJob(app, r, jobID)
	if !ok {
		return status, data, nil
	}
	if status, data, ok := requireProjectAccess(app, r, jobProjectID(record)); !ok {
		return status, data, nil
	}
	if !shared.BoolValue(record.Data, "streaming_session", "streamingSession", "StreamingSession") {
		return http.StatusConflict, shared.ErrorData("job is not a streaming session"), nil
	}
	if !streamJobActive(record) {
		return http.StatusConflict, shared.ErrorData("streaming session is not active"), nil
	}
	if len(app.Config.StreamTURNURIs) == 0 || strings.TrimSpace(app.Config.StreamTURNSharedSecret) == "" {
		return http.StatusServiceUnavailable, shared.ErrorData("stream TURN credentials are not configured"), nil
	}

	ttl := streamCredentialTTL(app.Config, shared.IntValue(payload, "ttl_seconds", "ttlSeconds"))
	expires := time.Now().UTC().Add(ttl)
	username := fmt.Sprintf("%d:%s", expires.Unix(), streamCredentialUser(r, payload))
	password := streamTURNPassword(app.Config.StreamTURNSharedSecret, username)
	return http.StatusOK, map[string]any{
		"job_id": jobID,
		"turn": map[string]any{
			"uris":        append([]string{}, app.Config.StreamTURNURIs...),
			"username":    username,
			"password":    password,
			"ttl_seconds": int(ttl.Seconds()),
			"expires_at":  expires.Format(time.RFC3339),
		},
	}, nil
}

func streamCredentialJob(app *platform.App, r *http.Request, jobID string) (contracts.Record[map[string]any], int, any, bool) {
	jobs := jobRepository(app)
	if jobs == nil {
		return contracts.Record[map[string]any]{}, http.StatusInternalServerError, shared.ErrorData("job repository unavailable"), false
	}
	record, found := jobs.FindJob(r.Context(), jobID)
	if !found {
		return contracts.Record[map[string]any]{}, http.StatusNotFound, shared.ErrorData("job not found"), false
	}
	return record, 0, nil, true
}

func streamCredentialTTL(cfg platform.Config, requestedSeconds int) time.Duration {
	maxTTL := cfg.StreamTURNCredentialTTL
	if maxTTL <= 0 {
		maxTTL = 8 * time.Hour
	}
	ttl := maxTTL
	if requestedSeconds > 0 {
		ttl = time.Duration(requestedSeconds) * time.Second
	}
	if ttl > maxTTL {
		return maxTTL
	}
	return ttl
}

func streamCredentialUser(r *http.Request, payload map[string]any) string {
	user := shared.FirstNonEmpty(
		strings.TrimSpace(r.Header.Get("X-User-ID")),
		strings.TrimSpace(r.Header.Get("X-Username")),
		"stream",
	)
	if session := shared.TextValue(payload, "session_id", "sessionId"); session != "" {
		user += "-" + session
	}
	return streamCredentialNamePart(user)
}

func streamCredentialNamePart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "stream"
	}
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	if out := strings.Trim(b.String(), "-"); out != "" {
		return out
	}
	return "stream"
}

func streamTURNPassword(secret, username string) string {
	mac := hmac.New(sha1.New, []byte(secret))
	mac.Write([]byte(username))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func streamJobActive(record contracts.Record[map[string]any]) bool {
	switch currentJobStatus(record.Data) {
	case jobStatusSubmitted, jobStatusWaitingInfra, jobStatusQueued, jobStatusRunning:
		return true
	default:
		return false
	}
}
