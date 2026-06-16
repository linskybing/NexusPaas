package platform

import (
	"context"
	"sync"
	"time"
)

type WorkerLeases struct {
	mu     sync.Mutex
	leases map[string]lease
}

type lease struct {
	worker    string
	expiresAt time.Time
}

func NewWorkerLeases() *WorkerLeases {
	return &WorkerLeases{leases: map[string]lease{}}
}

func (w *WorkerLeases) Acquire(_ context.Context, worker, shard string, ttl time.Duration) (bool, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	now := time.Now().UTC()
	current, ok := w.leases[shard]
	if ok && current.expiresAt.After(now) && current.worker != worker {
		return false, nil
	}
	w.leases[shard] = lease{worker: worker, expiresAt: now.Add(ttl)}
	return true, nil
}

func (w *WorkerLeases) Release(_ context.Context, worker, shard string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	current, ok := w.leases[shard]
	if ok && current.worker == worker {
		delete(w.leases, shard)
	}
	return nil
}
