package platform

import (
	"context"
	"testing"
)

func TestMemoryObjectStorePutGetDelete(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryObjectStore()

	if err := store.Put(ctx, "a/b.png", []byte("hello"), "image/png"); err != nil {
		t.Fatalf("Put: %v", err)
	}
	body, contentType, found, err := store.Get(ctx, "a/b.png")
	if err != nil || !found {
		t.Fatalf("Get found=%v err=%v, want found", found, err)
	}
	if string(body) != "hello" || contentType != "image/png" {
		t.Fatalf("Get = %q/%q, want hello/image/png", body, contentType)
	}

	// Stored bytes must be copied, not aliased to the caller's slice.
	body[0] = 'x'
	again, _, _, _ := store.Get(ctx, "a/b.png")
	if string(again) != "hello" {
		t.Fatalf("object store returned aliased body: %q", again)
	}

	if err := store.Delete(ctx, "a/b.png"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, _, found, _ := store.Get(ctx, "a/b.png"); found {
		t.Fatalf("object still present after delete")
	}
}

func TestMemoryObjectStoreListAndHealth(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryObjectStore()
	_ = store.Put(ctx, "k2", []byte("22"), "text/plain")
	_ = store.Put(ctx, "k1", []byte("1"), "text/plain")

	infos, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(infos) != 2 || infos[0].Key != "k1" || infos[0].Size != 1 || infos[1].Size != 2 {
		t.Fatalf("List = %#v, want sorted k1(1),k2(2)", infos)
	}
	if err := store.HealthCheck(ctx); err != nil {
		t.Fatalf("HealthCheck: %v", err)
	}
}

func TestObjectStoreBackingDependencyAddedWhenConfigured(t *testing.T) {
	with := Config{ServiceName: mediaUploadServiceName, DatabaseURL: "postgres://h:5432", RedisURL: "redis://h:6379", EventBusURL: "redis://h:6379", ObjectStoreURL: "http://minio:9000"}
	if got := len(with.BackingDependencies()); got != 4 {
		t.Fatalf("BackingDependencies with object store = %d, want 4", got)
	}
	stale := Config{ServiceName: "identity-service", DatabaseURL: "postgres://h:5432", RedisURL: "redis://h:6379", EventBusURL: "redis://h:6379", ObjectStoreURL: "http://minio:9000"}
	for _, dep := range stale.BackingDependencies() {
		if dep.Name == envObjectStoreURL {
			t.Fatalf("object store dependency present for non-blob service")
		}
	}
	without := Config{DatabaseURL: "postgres://h:5432"}
	for _, dep := range without.BackingDependencies() {
		if dep.Name == envObjectStoreURL {
			t.Fatalf("object store dependency present without OBJECT_STORE_URL")
		}
	}
}

func TestValidateProductionObjectStore(t *testing.T) {
	base := withRuntimeDefaults(Config{
		ServiceName: mediaUploadServiceName, Production: true, RequireAuth: true, APIKeys: map[string]bool{"k": true},
		APIKeyPrincipals:       map[string]APIKeyPrincipal{"k": {ID: "svc", Role: "service"}},
		AuthorizationPolicyURL: "https://pdp", AuthorizationPolicyAPIKey: "x",
		ServiceIdentityName:      mediaUploadServiceName,
		ServiceIdentityKey:       "scoped-key",
		ServiceTrustedIdentities: map[string]ServiceTrustedIdentity{"iam-unit": {Key: "iam-key", Audiences: []string{mediaUploadServiceName}}},
		DatabaseURL:              "postgres://h:5432", RedisURL: "redis://h:6379", EventBusURL: "redis://h:6379",
	})
	// Missing object store URL fails in production.
	if err := base.Validate(); err == nil {
		t.Fatalf("Validate without OBJECT_STORE_URL should fail in production")
	}
	// URL set but credentials missing fails.
	base.ObjectStoreURL = "http://minio:9000"
	if err := base.Validate(); err == nil {
		t.Fatalf("Validate without object store credentials should fail")
	}
	// Fully configured passes.
	base.ObjectStoreAccessKey = "access"
	base.ObjectStoreSecretKey = "secret"
	if err := base.Validate(); err != nil {
		t.Fatalf("Validate fully configured object store: %v", err)
	}

	isolated := base
	isolated.ServiceName = "identity-service"
	isolated.ObjectStoreURL = "not a url"
	isolated.ObjectStoreAccessKey = ""
	isolated.ObjectStoreSecretKey = ""
	if err := isolated.Validate(); err != nil {
		t.Fatalf("Validate isolated non-blob service with stale object store config: %v", err)
	}

	cohosted := isolated
	cohosted.ServiceName = "all"
	if err := cohosted.Validate(); err == nil {
		t.Fatalf("Validate co-hosted all without object store credentials should fail")
	}
}
