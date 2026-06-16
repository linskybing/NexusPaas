package workload

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestConfigRepositoryConfigLifecycleCloneAndOrdering(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: serviceName})
	configs := configRepository(app)
	ctx := context.Background()

	if got := configs.NextConfigID(); got != "CFG2600001" {
		t.Fatalf("first config id = %q, want CFG2600001", got)
	}
	if got := configs.NextConfigID(); got != "CFG2600002" {
		t.Fatalf("second config id = %q, want CFG2600002", got)
	}

	input := map[string]any{"id": "cfg-b", "project_id": "P1", "name": "b.yaml", "path": "jobs/b.yaml"}
	created, err := configs.CreateConfig(ctx, input)
	if err != nil {
		t.Fatalf("create config: %v", err)
	}
	input["name"] = "mutated-input"
	created.Data["name"] = "mutated-return"

	stored, found := configs.GetConfig(ctx, "cfg-b")
	if !found || stored.Data["name"] != "b.yaml" {
		t.Fatalf("stored config = %#v found=%v, want original name", stored.Data, found)
	}

	if _, err := configs.CreateConfig(ctx, map[string]any{"id": "cfg-a", "project_id": "P1", "path": "jobs/a.yaml"}); err != nil {
		t.Fatalf("create cfg-a: %v", err)
	}
	if _, err := configs.CreateConfig(ctx, map[string]any{"id": "cfg-other", "project_id": "P2", "path": "jobs/other.yaml"}); err != nil {
		t.Fatalf("create cfg-other: %v", err)
	}

	got := configs.ListConfigsByProject(ctx, "P1")
	if ids := workloadConfigRecordIDs(got); strings.Join(ids, ",") != "cfg-a,cfg-b" {
		t.Fatalf("project configs = %v, want cfg-a,cfg-b sorted by path", ids)
	}

	updated, ok := configs.UpdateConfig(ctx, "cfg-b", map[string]any{"content": "kind: Pod"})
	if !ok || updated.Data["content"] != "kind: Pod" {
		t.Fatalf("updated config = %#v ok=%v, want content", updated.Data, ok)
	}
	if !configs.DeleteConfig(ctx, "cfg-other") {
		t.Fatal("delete cfg-other returned false")
	}
	if _, found := configs.GetConfig(ctx, "cfg-other"); found {
		t.Fatal("cfg-other still exists after delete")
	}
}

func TestConfigRepositoryVersionHistoryAndHashing(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: serviceName})
	configs := configRepository(app)
	ctx := context.Background()
	now := time.Date(2026, 6, 16, 8, 30, 0, 0, time.UTC)

	version, err := configs.CreateVersion(ctx, "cfg1", map[string]any{
		"content": "kind: Job",
		"message": "initial",
	}, "created", now)
	if err != nil {
		t.Fatalf("create version: %v", err)
	}
	sum := sha256.Sum256([]byte("kind: Job"))
	if version.ID != "VER2600001" ||
		version.Data["config_id"] != "cfg1" ||
		version.Data["sha256"] != hex.EncodeToString(sum[:]) ||
		version.Data["message"] != "initial" ||
		version.Data["created_at"] != now.Format(time.RFC3339) {
		t.Fatalf("version = %#v, want hashed cfg1 version metadata", version)
	}

	fallback, err := configs.CreateVersion(ctx, "cfg2", map[string]any{"content": "other"}, "updated", now)
	if err != nil {
		t.Fatalf("create fallback version: %v", err)
	}
	if fallback.Data["message"] != "updated" {
		t.Fatalf("fallback message = %#v, want reason", fallback.Data["message"])
	}

	history := configs.ListVersionsForConfigs(ctx, map[string]bool{"cfg1": true})
	if len(history) != 1 || history[0].ID != "VER2600001" {
		t.Fatalf("cfg1 history = %#v, want first version only", history)
	}

	got, found := configs.GetVersion(ctx, version.ID)
	if !found {
		t.Fatal("expected stored version")
	}
	got.Data["message"] = "mutated-return"
	reread, _ := configs.GetVersion(ctx, version.ID)
	if reread.Data["message"] != "initial" {
		t.Fatalf("version clone leaked mutation: %#v", reread.Data)
	}
}

func TestConfigRepositoryInstancesCommandsAndUnavailableStore(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: serviceName})
	configs := configRepository(app)
	ctx := context.Background()
	now := time.Date(2026, 6, 16, 9, 0, 0, 0, time.UTC)

	createWorkloadRecord(t, app, instancesResource, map[string]any{"id": "pod1", "config_id": "cfg1", "pod": "train"})
	createWorkloadRecord(t, app, instancesResource, map[string]any{"id": "pod2", "config_id": "cfg2", "pod": "other"})

	pods := configs.ListInstancesByConfig(ctx, "cfg1")
	if len(pods) != 1 || pods[0].ID != "pod1" {
		t.Fatalf("pods = %#v, want pod1 only", pods)
	}
	pods[0].Data["pod"] = "mutated-return"
	rereadPods := configs.ListInstancesByConfig(ctx, "cfg1")
	if rereadPods[0].Data["pod"] != "train" {
		t.Fatalf("pod clone leaked mutation: %#v", rereadPods[0].Data)
	}

	payload := map[string]any{"namespace": "proj-p1", "id": "caller-id"}
	command, err := configs.CreateInstanceCommand(ctx, "cfg1", "start", payload, now)
	if err != nil {
		t.Fatalf("create command: %v", err)
	}
	if command.ID != "CMD2600001" ||
		command.Data["id"] != "CMD2600001" ||
		command.Data["config_id"] != "cfg1" ||
		command.Data["action"] != "start" ||
		command.Data["status"] != "accepted" ||
		command.Data["requested_at"] != now.Format(time.RFC3339) {
		t.Fatalf("command = %#v, want accepted start command metadata", command)
	}
	if _, mutated := payload["action"]; mutated || payload["id"] != "caller-id" {
		t.Fatalf("command creation mutated caller payload: %#v", payload)
	}

	unavailable := configRepositoryFromStore(nil)
	if got := unavailable.NextConfigID(); got != "" {
		t.Fatalf("nil-store config id = %q, want empty", got)
	}
	if _, err := unavailable.CreateConfig(ctx, map[string]any{"id": "cfg"}); !errors.Is(err, errWorkloadConfigRepositoryUnavailable) {
		t.Fatalf("nil-store create err = %v, want repository unavailable", err)
	}
	if _, found := unavailable.GetConfig(ctx, "cfg"); found {
		t.Fatal("nil-store get returned found")
	}
	if got := unavailable.ListConfigsByProject(ctx, "P1"); len(got) != 0 {
		t.Fatalf("nil-store list = %#v, want empty", got)
	}
	if unavailable.DeleteConfig(ctx, "cfg") {
		t.Fatal("nil-store delete returned true")
	}
	if _, err := unavailable.CreateVersion(ctx, "cfg", map[string]any{}, "created", now); !errors.Is(err, errWorkloadConfigRepositoryUnavailable) {
		t.Fatalf("nil-store version err = %v, want repository unavailable", err)
	}
	if _, err := unavailable.CreateInstanceCommand(ctx, "cfg", "start", map[string]any{}, now); !errors.Is(err, errWorkloadConfigRepositoryUnavailable) {
		t.Fatalf("nil-store command err = %v, want repository unavailable", err)
	}
}

func TestConfigRepositorySourceGuardOwnsConfigResourceStoreAccess(t *testing.T) {
	violations, err := scanWorkloadConfigResourceGuard(".")
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) > 0 {
		t.Fatalf("workload config resources must be owned by config_repository.go:\n%s", strings.Join(violations, "\n"))
	}
}

func scanWorkloadConfigResourceGuard(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var violations []string
	for _, entry := range entries {
		if !shouldScanWorkloadConfigGuardFile(entry) {
			continue
		}
		name := entry.Name()
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		violations = append(violations, workloadConfigFileGuardViolations(name, string(raw))...)
	}
	return violations, nil
}

func shouldScanWorkloadConfigGuardFile(entry os.DirEntry) bool {
	name := entry.Name()
	return !entry.IsDir() &&
		filepath.Ext(name) == ".go" &&
		!strings.HasSuffix(name, "_test.go") &&
		name != "config_repository.go"
}

func workloadConfigFileGuardViolations(name, raw string) []string {
	violations := []string{}
	for i, line := range strings.Split(raw, "\n") {
		if violation, ok := workloadConfigLineGuardViolation(name, i+1, line); ok {
			violations = append(violations, violation)
		}
	}
	return violations
}

func workloadConfigLineGuardViolation(name string, lineNumber int, line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "//") {
		return "", false
	}
	term := firstGuardedWorkloadConfigTerm(trimmed, guardedWorkloadConfigTerms())
	if term == "" {
		return "", false
	}
	return name + ":" + strconv.Itoa(lineNumber) + ": " + workloadConfigGuardReason(trimmed) + " uses " + term + ": " + trimmed, true
}

func workloadConfigGuardReason(line string) string {
	if workloadConfigStoreCallPattern.MatchString(line) {
		return "direct store call"
	}
	return "resource key"
}

func workloadConfigRecordIDs(records []contractsRecordForConfigTest) []string {
	ids := make([]string, 0, len(records))
	for _, record := range records {
		ids = append(ids, record.ID)
	}
	return ids
}

func firstGuardedWorkloadConfigTerm(line string, terms []string) string {
	for _, term := range terms {
		if strings.Contains(line, term) {
			return term
		}
	}
	return ""
}

func guardedWorkloadConfigTerms() []string {
	return []string{
		"configsResource",
		"versionsResource",
		"instancesResource",
		"commandsResource",
		"workload-service:configfiles",
		"workload-service:configfiles:versions",
		"workload-service:instances",
		"workload-service:instances:commands",
		"\":configfiles\"",
		"\":configfiles:versions\"",
		"\":instances\"",
		"\":instances:commands\"",
	}
}

var workloadConfigStoreCallPattern = regexp.MustCompile(`\b(?:Store|store)\.(?:Get|List|Create|Update|Delete|NextID)\(`)

type contractsRecordForConfigTest = contracts.Record[map[string]any]
