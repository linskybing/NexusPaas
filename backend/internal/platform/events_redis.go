package platform

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
)

const defaultEventStream = "events"

// RedisEventBus implements EventStream on Redis Streams so events are durable and
// shared across replicas (findings 4, 7). Publish appends without producer-side
// trimming so lagging projection consumers can replay from the shared stream.
// Consume records (consumer,event_id) in a Redis set for cross-replica
// idempotency; Outbox reads the stream back; Checkpoint/Lag track per-consumer
// progress against the stream length.
type RedisEventBus struct {
	rdb    *redis.Client
	stream string
}

var _ EventStream = (*RedisEventBus)(nil)

func NewRedisEventBus(rdb *redis.Client) *RedisEventBus {
	return &RedisEventBus{rdb: rdb, stream: defaultEventStream}
}

func (b *RedisEventBus) Publish(ctx context.Context, event contracts.Event) error {
	if event.Name == "" || event.Source == "" || event.EventID == "" || event.TraceID == "" || event.SchemaVersion == 0 {
		return errors.New("event metadata is incomplete")
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return b.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: b.stream,
		Values: map[string]any{"event": payload},
	}).Err()
}

func (b *RedisEventBus) Consume(ctx context.Context, consumer string, event contracts.Event) (bool, error) {
	if consumer == "" || event.EventID == "" {
		return false, errors.New("consumer and event_id are required")
	}
	added, err := b.rdb.SAdd(ctx, inboxKey(consumer), event.EventID).Result()
	if err != nil {
		return false, err
	}
	return added == 1, nil
}

func (b *RedisEventBus) ResetConsumer(consumer string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := b.rdb.Del(ctx, inboxKey(consumer), checkpointKey(consumer)).Err(); err != nil {
		slog.Error("redis reset consumer failed", "consumer", consumer, "error", err)
	}
}

func (b *RedisEventBus) Outbox() []contracts.Event {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	messages, err := b.rdb.XRange(ctx, b.stream, "-", "+").Result()
	if err != nil {
		slog.Error("redis outbox read failed", "error", err)
		return nil
	}
	events := make([]contracts.Event, 0, len(messages))
	for _, msg := range messages {
		raw, ok := msg.Values["event"].(string)
		if !ok {
			continue
		}
		var event contracts.Event
		if err := json.Unmarshal([]byte(raw), &event); err != nil {
			slog.Error("redis outbox unmarshal failed", "id", msg.ID, "error", err)
			continue
		}
		events = append(events, event)
	}
	return events
}

func (b *RedisEventBus) Checkpoint(consumer string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	length, err := b.rdb.XLen(ctx, b.stream).Result()
	if err != nil {
		slog.Error("redis checkpoint xlen failed", "consumer", consumer, "error", err)
		return
	}
	if err := b.rdb.Set(ctx, checkpointKey(consumer), length, 0).Err(); err != nil {
		slog.Error("redis checkpoint set failed", "consumer", consumer, "error", err)
	}
}

func (b *RedisEventBus) Lag(consumer string) int {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	length, err := b.rdb.XLen(ctx, b.stream).Result()
	if err != nil {
		slog.Error("redis lag xlen failed", "consumer", consumer, "error", err)
		return 0
	}
	checkpoint, err := b.rdb.Get(ctx, checkpointKey(consumer)).Int64()
	if err == redis.Nil {
		checkpoint = 0
	} else if err != nil {
		slog.Error("redis lag get failed", "consumer", consumer, "error", err)
		return 0
	}
	if checkpoint > length {
		return 0
	}
	return int(length - checkpoint)
}

func inboxKey(consumer string) string      { return "inbox:" + consumer }
func checkpointKey(consumer string) string { return "checkpoint:" + consumer }
