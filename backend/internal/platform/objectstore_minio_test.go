//go:build integration

package platform

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// TestMinioObjectStoreRoundTrip exercises the real S3-compatible client against a
// MinIO endpoint. Run with: go test -tags integration ./internal/platform/...
// and TEST_OBJECT_STORE_URL / TEST_OBJECT_STORE_ACCESS_KEY /
// TEST_OBJECT_STORE_SECRET_KEY set (bucket defaults to "media-test").
func TestMinioObjectStoreRoundTrip(t *testing.T) {
	ctx := context.Background()
	store := newIntegrationMinioStore(t, ctx)
	key := "2026/integration_roundtrip.png"
	payload := []byte{0x89, 'P', 'N', 'G', 0, 1, 2, 3}
	if err := store.Put(ctx, key, payload, "image/png"); err != nil {
		t.Fatalf("Put: %v", err)
	}
	t.Cleanup(func() { _ = store.Delete(ctx, key) })

	body, contentType := requireObject(t, store, ctx, key)
	if !bytes.Equal(body, payload) || contentType != "image/png" {
		t.Fatalf("Get = %v/%q, want payload/image/png", body, contentType)
	}
	requireObjectListed(t, store, ctx, key)

	if err := store.Delete(ctx, key); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, _, found, _ := store.Get(ctx, key); found {
		t.Fatalf("object still present after delete")
	}
}

func newIntegrationMinioStore(t *testing.T, ctx context.Context) ObjectStore {
	t.Helper()
	endpoint, accessKey, secretKey, bucket := integrationObjectStoreConfig(t)
	if _, err := EnsureObjectStoreBucket(ctx, endpoint, accessKey, secretKey, bucket); err != nil {
		t.Fatalf("EnsureObjectStoreBucket: %v", err)
	}
	store, err := NewMinioObjectStore(ctx, endpoint, accessKey, secretKey, bucket)
	if err != nil {
		t.Fatalf("NewMinioObjectStore: %v", err)
	}
	if err := store.HealthCheck(ctx); err != nil {
		t.Fatalf("HealthCheck: %v", err)
	}
	return store
}

func TestMinioObjectStoreRequiresExistingBucket(t *testing.T) {
	ctx := context.Background()
	endpoint, accessKey, secretKey, _ := integrationObjectStoreConfig(t)
	bucket := uniqueIntegrationBucket("media-missing")
	client, err := newMinioClient(endpoint, accessKey, secretKey)
	if err != nil {
		t.Fatalf("newMinioClient: %v", err)
	}

	store, err := NewMinioObjectStore(ctx, endpoint, accessKey, secretKey, bucket)
	if err == nil {
		_ = store.Delete(ctx, "unused")
		t.Fatalf("NewMinioObjectStore missing bucket error = nil, want error")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("NewMinioObjectStore missing bucket error = %v, want does not exist", err)
	}
	exists, err := client.BucketExists(ctx, bucket)
	if err != nil {
		t.Fatalf("BucketExists after serving constructor: %v", err)
	}
	if exists {
		t.Fatalf("serving constructor created bucket %q", bucket)
	}
}

func TestEnsureObjectStoreBucketIdempotent(t *testing.T) {
	ctx := context.Background()
	endpoint, accessKey, secretKey, _ := integrationObjectStoreConfig(t)
	bucket := uniqueIntegrationBucket("media-provision")
	client, err := newMinioClient(endpoint, accessKey, secretKey)
	if err != nil {
		t.Fatalf("newMinioClient: %v", err)
	}
	t.Cleanup(func() {
		_ = client.RemoveBucket(context.Background(), bucket)
	})

	created, err := EnsureObjectStoreBucket(ctx, endpoint, accessKey, secretKey, bucket)
	if err != nil {
		t.Fatalf("EnsureObjectStoreBucket first: %v", err)
	}
	if !created {
		t.Fatalf("EnsureObjectStoreBucket first created = false, want true")
	}
	created, err = EnsureObjectStoreBucket(ctx, endpoint, accessKey, secretKey, bucket)
	if err != nil {
		t.Fatalf("EnsureObjectStoreBucket second: %v", err)
	}
	if created {
		t.Fatalf("EnsureObjectStoreBucket second created = true, want false")
	}
}

func integrationObjectStoreConfig(t *testing.T) (string, string, string, string) {
	t.Helper()
	endpoint := os.Getenv("TEST_OBJECT_STORE_URL")
	if endpoint == "" {
		t.Skip("set TEST_OBJECT_STORE_URL to run the MinIO integration test")
	}
	bucket := os.Getenv("TEST_OBJECT_STORE_BUCKET")
	if bucket == "" {
		bucket = "media-test"
	}
	return endpoint, os.Getenv("TEST_OBJECT_STORE_ACCESS_KEY"), os.Getenv("TEST_OBJECT_STORE_SECRET_KEY"), bucket
}

func uniqueIntegrationBucket(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UTC().UnixNano())
}

func requireObject(t *testing.T, store ObjectStore, ctx context.Context, key string) ([]byte, string) {
	t.Helper()
	body, contentType, found, err := store.Get(ctx, key)
	if err != nil || !found {
		t.Fatalf("Get found=%v err=%v", found, err)
	}
	return body, contentType
}

func requireObjectListed(t *testing.T, store ObjectStore, ctx context.Context, key string) {
	t.Helper()
	infos, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, info := range infos {
		if info.Key == key {
			return
		}
	}
	t.Fatalf("List did not include %q", key)
}
