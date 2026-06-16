package platform

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// ObjectStore is the binary-blob persistence port. Media blobs are too large to
// keep inline in the JSON RecordStore at scale, so they live in a dedicated
// object store (MinIO/S3 in production) while their metadata stays in the
// RecordStore. The interface is intentionally small (Interface Segregation) and
// is injected via WithObjectStore so the platform core never depends on a
// concrete client (Dependency Inversion). When no object store is configured the
// App leaves this nil and callers fall back to RecordStore-inline storage.
type ObjectStore interface {
	// Put writes body under key, overwriting any existing object.
	Put(ctx context.Context, key string, body []byte, contentType string) error
	// Get returns the object body and content type. found is false when the key
	// does not exist; err is only non-nil on a transport/backend failure.
	Get(ctx context.Context, key string) (body []byte, contentType string, found bool, err error)
	// Delete removes key. Deleting a missing key is not an error.
	Delete(ctx context.Context, key string) error
	// List returns metadata for every stored object (used for bounded eviction).
	List(ctx context.Context) ([]ObjectInfo, error)
	// HealthCheck verifies the backend is reachable and the bucket exists. It is
	// used by readiness for a protocol-level (not just TCP) check.
	HealthCheck(ctx context.Context) error
}

// ObjectInfo is the metadata returned when listing an ObjectStore.
type ObjectInfo struct {
	Key         string
	Size        int64
	ContentType string
}

// MemoryObjectStore is an in-process ObjectStore used by tests and local
// no-dependency runs. It is safe for concurrent use.
type MemoryObjectStore struct {
	mu      sync.RWMutex
	objects map[string]memoryObject
}

type memoryObject struct {
	body        []byte
	contentType string
}

// NewMemoryObjectStore returns an empty in-memory ObjectStore.
func NewMemoryObjectStore() *MemoryObjectStore {
	return &MemoryObjectStore{objects: map[string]memoryObject{}}
}

func (s *MemoryObjectStore) Put(_ context.Context, key string, body []byte, contentType string) error {
	if key == "" {
		return fmt.Errorf("object key is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.objects[key] = memoryObject{body: append([]byte(nil), body...), contentType: contentType}
	return nil
}

func (s *MemoryObjectStore) Get(_ context.Context, key string) ([]byte, string, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	obj, ok := s.objects[key]
	if !ok {
		return nil, "", false, nil
	}
	return append([]byte(nil), obj.body...), obj.contentType, true, nil
}

func (s *MemoryObjectStore) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.objects, key)
	return nil
}

func (s *MemoryObjectStore) List(_ context.Context) ([]ObjectInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	infos := make([]ObjectInfo, 0, len(s.objects))
	for key, obj := range s.objects {
		infos = append(infos, ObjectInfo{Key: key, Size: int64(len(obj.body)), ContentType: obj.contentType})
	}
	sort.Slice(infos, func(i, j int) bool { return infos[i].Key < infos[j].Key })
	return infos, nil
}

func (s *MemoryObjectStore) HealthCheck(_ context.Context) error { return nil }

var _ ObjectStore = (*MemoryObjectStore)(nil)
