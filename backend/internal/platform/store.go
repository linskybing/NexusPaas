package platform

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"maps"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
)

type Store struct {
	mu   sync.RWMutex
	data map[string]map[string]contracts.Record[map[string]any]
	seq  map[string]int
}

var ErrCreateConflict = errors.New("store create conflict")

type CreateConflictError struct {
	Resource string
	ID       string
}

func (e CreateConflictError) Error() string {
	return ErrCreateConflict.Error() + ": " + e.Resource + "/" + e.ID
}

func (e CreateConflictError) Unwrap() error {
	return ErrCreateConflict
}

func IsCreateConflict(err error) bool {
	return errors.Is(err, ErrCreateConflict)
}

func NewStore() *Store {
	return &Store{
		data: map[string]map[string]contracts.Record[map[string]any]{},
		seq:  map[string]int{},
	}
}

// NextID returns a fresh, collision-free identifier for resource using the
// given prefix. The scan-and-reserve happens under a single lock so concurrent
// callers never receive the same id (finding 28). A monotonic high-water mark
// per resource+prefix is retained so an id is never reused after the highest
// record is deleted (finding 31). base is the first number to allocate; width>0
// zero-pads the numeric suffix to that many digits. The store is scanned each
// call so pre-seeded or externally created ids are also skipped.
func (s *Store) NextID(resource, prefix string, base, width int) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := resource + "|" + prefix
	maxN := base - 1
	for id := range s.data[resource] {
		if !strings.HasPrefix(id, prefix) {
			continue
		}
		var n int
		if _, err := fmt.Sscanf(strings.TrimPrefix(id, prefix), "%d", &n); err == nil && n > maxN {
			maxN = n
		}
	}
	if cached := s.seq[key]; cached > maxN {
		maxN = cached
	}
	for {
		maxN++
		var id string
		if width > 0 {
			id = fmt.Sprintf("%s%0*d", prefix, width, maxN)
		} else {
			id = fmt.Sprintf("%s%d", prefix, maxN)
		}
		if _, exists := s.data[resource][id]; !exists {
			s.seq[key] = maxN
			return id
		}
	}
}

func (s *Store) Create(_ context.Context, resource string, data map[string]any) (contracts.Record[map[string]any], error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data[resource] == nil {
		s.data[resource] = map[string]contracts.Record[map[string]any]{}
	}
	id, _ := data["id"].(string)
	if id == "" {
		id = newID()
		data["id"] = id
	}
	if _, exists := s.data[resource][id]; exists {
		return contracts.Record[map[string]any]{}, CreateConflictError{Resource: resource, ID: id}
	}
	now := time.Now().UTC()
	record := contracts.Record[map[string]any]{
		ID:        id,
		Data:      cloneMap(data),
		Version:   1,
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.data[resource][id] = record
	return cloneRecord(record), nil
}

func (s *Store) Get(_ context.Context, resource, id string) (contracts.Record[map[string]any], bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.data[resource][id]
	if !ok {
		return contracts.Record[map[string]any]{}, false
	}
	return cloneRecord(record), true
}

func (s *Store) List(_ context.Context, resource string) []contracts.Record[map[string]any] {
	s.mu.RLock()
	defer s.mu.RUnlock()
	records := make([]contracts.Record[map[string]any], 0, len(s.data[resource]))
	for _, record := range s.data[resource] {
		records = append(records, cloneRecord(record))
	}
	sort.Slice(records, func(i, j int) bool { return records[i].CreatedAt.Before(records[j].CreatedAt) })
	return records
}

func (s *Store) Update(_ context.Context, resource, id string, data map[string]any) (contracts.Record[map[string]any], bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.data[resource][id]
	if !ok {
		return contracts.Record[map[string]any]{}, false
	}
	nextData := cloneMap(current.Data)
	for key, value := range data {
		nextData[key] = value
	}
	next := contracts.Record[map[string]any]{
		ID:        current.ID,
		Data:      nextData,
		Version:   current.Version + 1,
		CreatedAt: current.CreatedAt,
		UpdatedAt: time.Now().UTC(),
	}
	s.data[resource][id] = next
	return cloneRecord(next), true
}

func (s *Store) Delete(_ context.Context, resource, id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.data[resource][id]; !ok {
		return false
	}
	delete(s.data[resource], id)
	return true
}

func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return time.Now().UTC().Format("20060102150405.000000000")
	}
	return hex.EncodeToString(b[:])
}

// NewUUID returns a canonical RFC 4122 version 4 UUID string. It is suitable for
// outbox event identifiers whose backing schema declares the column as UUID.
func NewUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Fall back to a time-seeded value; still UUID-shaped for schema compatibility.
		nanos := uint64(time.Now().UTC().UnixNano())
		for i := 0; i < 8; i++ {
			b[i] = byte(nanos >> (8 * i))
			b[i+8] = byte(nanos >> (8 * i))
		}
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return hex.EncodeToString(b[0:4]) + "-" +
		hex.EncodeToString(b[4:6]) + "-" +
		hex.EncodeToString(b[6:8]) + "-" +
		hex.EncodeToString(b[8:10]) + "-" +
		hex.EncodeToString(b[10:16])
}

func cloneMap(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	return maps.Clone(in)
}

func cloneRecord(record contracts.Record[map[string]any]) contracts.Record[map[string]any] {
	record.Data = cloneMap(record.Data)
	return record
}
