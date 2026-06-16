package authorizationpolicy

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

func decodeRawPolicy(r *http.Request) ([]string, error) {
	raw, err := readRequestBody(r)
	if err != nil {
		return nil, err
	}
	var policy []string
	if err := json.Unmarshal(raw, &policy); err != nil {
		return nil, fmt.Errorf(msgInvalidPolicyFormat)
	}
	return normalizePolicy(policy), nil
}

func decodeRawPolicyUpdate(r *http.Request) ([]string, []string, error) {
	raw, err := readRequestBody(r)
	if err != nil {
		return nil, nil, err
	}
	var payload struct {
		Old []string `json:"old"`
		New []string `json:"new"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, nil, fmt.Errorf(msgInvalidPolicyFormat)
	}
	oldPolicy := normalizePolicy(payload.Old)
	newPolicy := normalizePolicy(payload.New)
	if len(oldPolicy) < 4 || len(newPolicy) < 4 {
		return nil, nil, fmt.Errorf(msgInvalidPolicyFormat)
	}
	return oldPolicy, newPolicy, nil
}

func readRequestBody(r *http.Request) ([]byte, error) {
	defer r.Body.Close()
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf(errReadBodyFmt, err)
	}
	return raw, nil
}

func decodePermissionOperations(r *http.Request) ([]map[string]string, error) {
	payload, _, err := decodePayload(r)
	if err != nil {
		return nil, fmt.Errorf(msgInvalidBatchRequest)
	}
	items, ok := payload["operations"].([]any)
	if !ok || len(items) == 0 {
		return nil, fmt.Errorf(msgInvalidBatchRequest)
	}
	operations := make([]map[string]string, 0, len(items))
	for i, item := range items {
		raw, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("operations[%d] must be an object", i)
		}
		op := map[string]string{
			"type":       shared.TextValue(raw, "type"),
			"action":     shared.TextValue(raw, "action"),
			"project_id": shared.TextValue(raw, "project_id", "projectId"),
			"group_id":   shared.TextValue(raw, "group_id", "groupId"),
			"user_id":    shared.TextValue(raw, "user_id", "userId"),
			"role":       shared.TextValue(raw, "role"),
		}
		if op["type"] == "" || op["action"] == "" || op["user_id"] == "" {
			return nil, fmt.Errorf(msgInvalidBatchRequest)
		}
		operations = append(operations, op)
	}
	return operations, nil
}

func normalizePolicy(policy []string) []string {
	out := make([]string, 0, len(policy))
	for _, value := range policy {
		out = append(out, strings.TrimSpace(value))
	}
	return out
}

func rawPolicyRecord(policy []string) map[string]any {
	record := map[string]any{
		"id":     rawPolicyID(policy),
		"policy": append([]string{}, policy...),
	}
	for i, value := range policy {
		record[fmt.Sprintf("v%d", i)] = value
	}
	if len(policy) >= 4 {
		record["sub"] = policy[0]
		record["dom"] = policy[1]
		record["obj"] = policy[2]
		record["act"] = policy[3]
	}
	return record
}

func rawPolicyID(policy []string) string {
	return strings.Join(policy, "\x1f")
}

func policySlice(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string{}, typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			out = append(out, strings.TrimSpace(fmt.Sprint(item)))
		}
		return out
	default:
		return nil
	}
}
