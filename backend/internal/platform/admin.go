package platform

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

func RunAdminTask(task string, cfg Config) error {
	switch task {
	case "validate-config":
		return cfg.Validate()
	case "apply-migrations":
		return ApplyMigrations(context.Background(), cfg.DatabaseURL)
	case "ensure-object-store-bucket":
		return runEnsureObjectStoreBucketTask(context.Background(), cfg)
	case "validate-migrations":
		files, err := validateServiceMigrations()
		if err != nil {
			return err
		}
		fmt.Printf("validated %d additive service migration files\n", len(files))
		return nil
	case "replay-outbox":
		fmt.Println("outbox replay dry-run: idempotency uses event_id and consumer inbox checkpoint")
		return nil
	case "compensate":
		fmt.Println("compensation dry-run: idempotency keys and service-owned command/status records required")
		return nil
	default:
		return fmt.Errorf("unknown admin task %q", task)
	}
}

func runEnsureObjectStoreBucketTask(ctx context.Context, cfg Config) error {
	if !cfg.RequiresObjectStore() {
		return fmt.Errorf("ADMIN_TASK=ensure-object-store-bucket requires SERVICE_NAME=%s or SERVICE_NAME=all", mediaUploadServiceName)
	}
	if missing := missingObjectStoreConfig(cfg); len(missing) > 0 {
		return errors.New(strings.Join(missing, ", ") + " must be set for ADMIN_TASK=ensure-object-store-bucket")
	}
	bucket := strings.TrimSpace(cfg.ObjectStoreBucket)
	created, err := EnsureObjectStoreBucket(ctx, cfg.ObjectStoreURL, cfg.ObjectStoreAccessKey, cfg.ObjectStoreSecretKey, bucket)
	if err != nil {
		return err
	}
	if created {
		fmt.Printf("object store bucket %q created\n", bucket)
		return nil
	}
	fmt.Printf("object store bucket %q already exists\n", bucket)
	return nil
}

func missingObjectStoreConfig(cfg Config) []string {
	values := map[string]string{
		envObjectStoreURL:       cfg.ObjectStoreURL,
		envObjectStoreAccessKey: cfg.ObjectStoreAccessKey,
		envObjectStoreSecretKey: cfg.ObjectStoreSecretKey,
		envObjectStoreBucket:    cfg.ObjectStoreBucket,
	}
	missing := []string{}
	for name, value := range values {
		if strings.TrimSpace(value) == "" {
			missing = append(missing, name)
		}
	}
	return missing
}
