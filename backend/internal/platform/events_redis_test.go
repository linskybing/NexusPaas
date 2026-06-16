package platform

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"

	"github.com/redis/go-redis/v9"
)

func sampleEvent(id string) contracts.Event {
	return contracts.Event{
		EventID:       id,
		Name:          "UserCreated",
		Source:        "identity-service",
		TraceID:       "trace-1",
		SchemaVersion: 1,
		Data:          map[string]any{"user_id": "US1"},
	}
}

func TestRedisEventBusPublishAndOutbox(t *testing.T) {
	_, client := newTestRedisClient(t)
	bus := NewRedisEventBus(client)
	ctx := context.Background()

	if err := bus.Publish(ctx, sampleEvent("e1")); err != nil {
		t.Fatalf("publish e1: %v", err)
	}
	if err := bus.Publish(ctx, sampleEvent("e2")); err != nil {
		t.Fatalf("publish e2: %v", err)
	}

	out := bus.Outbox()
	if len(out) != 2 {
		t.Fatalf("outbox len = %d, want 2", len(out))
	}
	if out[0].EventID != "e1" || out[1].EventID != "e2" {
		t.Fatalf("outbox order = %s,%s", out[0].EventID, out[1].EventID)
	}
	if out[0].Data["user_id"] != "US1" {
		t.Fatalf("event data not round-tripped: %#v", out[0].Data)
	}
}

func TestRedisEventBusPublishXAddArgsDoNotRequestTrim(t *testing.T) {
	_, client := newTestRedisClient(t)
	hook := &redisCommandCaptureHook{}
	client.AddHook(hook)
	bus := NewRedisEventBus(client)

	if err := bus.Publish(context.Background(), sampleEvent("e1")); err != nil {
		t.Fatalf("publish e1: %v", err)
	}
	args := hook.commandArgs("xadd")
	if len(args) == 0 {
		t.Fatal("did not capture xadd command")
	}
	requireRedisArg(t, args, "xadd")
	requireRedisArg(t, args, defaultEventStream)
	requireRedisArg(t, args, "*")
	requireRedisArg(t, args, "event")
	for _, forbidden := range []string{"maxlen", "minid", "~", "="} {
		if redisArgsContain(args, forbidden) {
			t.Fatalf("xadd args = %#v, must not contain trim token %q", args, forbidden)
		}
	}
}

func TestRedisEventBusPublishDoesNotTrimStream(t *testing.T) {
	_, client := newTestRedisClient(t)
	bus := NewRedisEventBus(client)
	ctx := context.Background()
	const total = 1005

	for i := range total {
		if err := bus.Publish(ctx, sampleEvent(fmt.Sprintf("event-%04d", i))); err != nil {
			t.Fatalf("publish event %d: %v", i, err)
		}
	}
	if got, err := client.XLen(ctx, bus.stream).Result(); err != nil || got != total {
		t.Fatalf("stream length = %d err=%v, want %d", got, err, total)
	}
	out := bus.Outbox()
	if len(out) != total {
		t.Fatalf("outbox length = %d, want %d", len(out), total)
	}
	if out[0].EventID != "event-0000" || out[len(out)-1].EventID != "event-1004" {
		t.Fatalf("outbox first/last = %q/%q, want event-0000/event-1004", out[0].EventID, out[len(out)-1].EventID)
	}
}

func TestRedisEventBusPublishRejectsIncompleteMetadata(t *testing.T) {
	_, client := newTestRedisClient(t)
	bus := NewRedisEventBus(client)
	if err := bus.Publish(context.Background(), contracts.Event{Name: "X"}); err == nil {
		t.Fatal("expected incomplete-metadata error")
	}
}

func TestRedisEventBusConsumeIsIdempotentPerConsumer(t *testing.T) {
	_, client := newTestRedisClient(t)
	bus := NewRedisEventBus(client)
	ctx := context.Background()
	event := sampleEvent("e1")

	first, err := bus.Consume(ctx, "dashboard", event)
	if err != nil || !first {
		t.Fatalf("first consume = %v err=%v, want true", first, err)
	}
	again, err := bus.Consume(ctx, "dashboard", event)
	if err != nil || again {
		t.Fatalf("duplicate consume = %v err=%v, want false", again, err)
	}
	// A different consumer processes the same event independently.
	other, err := bus.Consume(ctx, "audit", event)
	if err != nil || !other {
		t.Fatalf("other-consumer consume = %v err=%v, want true", other, err)
	}
}

func TestRedisEventBusCheckpointAndLag(t *testing.T) {
	_, client := newTestRedisClient(t)
	bus := NewRedisEventBus(client)
	ctx := context.Background()

	_ = bus.Publish(ctx, sampleEvent("e1"))
	_ = bus.Publish(ctx, sampleEvent("e2"))
	if lag := bus.Lag("dashboard"); lag != 2 {
		t.Fatalf("lag before checkpoint = %d, want 2", lag)
	}
	bus.Checkpoint("dashboard")
	if lag := bus.Lag("dashboard"); lag != 0 {
		t.Fatalf("lag after checkpoint = %d, want 0", lag)
	}
	_ = bus.Publish(ctx, sampleEvent("e3"))
	if lag := bus.Lag("dashboard"); lag != 1 {
		t.Fatalf("lag after new event = %d, want 1", lag)
	}
}

type redisCommandCaptureHook struct {
	mu       sync.Mutex
	commands [][]any
}

func (h *redisCommandCaptureHook) DialHook(next redis.DialHook) redis.DialHook {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		return next(ctx, network, addr)
	}
}

func (h *redisCommandCaptureHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		args := append([]any(nil), cmd.Args()...)
		h.mu.Lock()
		h.commands = append(h.commands, args)
		h.mu.Unlock()
		return next(ctx, cmd)
	}
}

func (h *redisCommandCaptureHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		return next(ctx, cmds)
	}
}

func (h *redisCommandCaptureHook) commandArgs(name string) []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	name = strings.ToLower(name)
	for _, command := range h.commands {
		if len(command) == 0 || strings.ToLower(fmt.Sprint(command[0])) != name {
			continue
		}
		out := make([]string, 0, len(command))
		for _, arg := range command {
			out = append(out, strings.ToLower(fmt.Sprint(arg)))
		}
		return out
	}
	return nil
}

func requireRedisArg(t *testing.T, args []string, want string) {
	t.Helper()
	if !redisArgsContain(args, want) {
		t.Fatalf("xadd args = %#v, want token %q", args, want)
	}
}

func redisArgsContain(args []string, want string) bool {
	want = strings.ToLower(want)
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}
