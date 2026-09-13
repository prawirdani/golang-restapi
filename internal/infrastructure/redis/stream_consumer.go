package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"runtime/debug"
	"strings"
	"time"

	"github.com/prawirdani/golang-restapi/internal/ports/messaging"
	"github.com/prawirdani/golang-restapi/pkg/log"
	"github.com/redis/go-redis/v9"
)

type ConsumerStreamConfig struct {
	Stream      string
	Group       string
	Consumer    string
	UseDLQ      bool
	BatchSize   int64
	Concurrency int
	Block       time.Duration
	MinIdle     time.Duration // idle threshold before XAUTOCLAIM reclaims
	MaxRetries  int64
}

// dedupTTL bounds how long a processed envelope ID stays deduplicated. It must
// outlive the longest redelivery window (retries + a XAck lost during
// shutdown); 7d covers even a multi-day outage.
const dedupTTL = 7 * 24 * time.Hour

// dedupKey scopes the idempotency claim to one stream so equal envelope IDs
// produced for different streams don't collide.
func dedupKey(stream, id string) string {
	return "dedup:" + stream + ":" + id
}

type Consumer interface {
	Start(ctx context.Context) error
}

type StreamConsumer[T any] struct {
	rdb     *redis.Client
	cfg     ConsumerStreamConfig
	handler messaging.Handler[T]
}

func NewStreamConsumer[T any](rdb *redis.Client, cfg ConsumerStreamConfig, h messaging.Handler[T]) *StreamConsumer[T] {
	return &StreamConsumer[T]{rdb: rdb, cfg: cfg, handler: h}
}

func (c *StreamConsumer[T]) Start(ctx context.Context) error {
	if err := c.ensureGroup(ctx); err != nil {
		return err
	}

	// buffered semaphore
	sem := make(chan struct{}, c.cfg.Concurrency)

	// claimPending scans the full PEL cursor — too expensive to run before every
	// XReadGroup. ponytail: reclaim at most once every 30s; retries in the PEL
	// wait one extra tick (plus MinIdle) before being picked up again.
	claimTicker := time.NewTicker(30 * time.Second)
	defer claimTicker.Stop()

	ctx = log.WithContext(ctx, "stream", c.cfg.Stream)
	log.InfoCtx(
		ctx, "Consumer started",
		"group", c.cfg.Group,
		"consumer", c.cfg.Consumer,
	)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-claimTicker.C:
			c.claimPending(ctx, sem)
		default:
		}

		streams, err := c.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    c.cfg.Group,
			Consumer: c.cfg.Consumer,
			Streams:  []string{c.cfg.Stream, ">"},
			Count:    c.cfg.BatchSize,
			Block:    c.cfg.Block,
		}).Result()
		if err != nil {
			if !errors.Is(err, redis.Nil) {
				log.ErrorCtx(ctx, "XReadGroup error", err)
				sleep(ctx, 100*time.Millisecond)
			}
			continue
		}

		for _, s := range streams {
			for _, m := range s.Messages {
				select {
				case sem <- struct{}{}:
					go c.handle(ctx, m, sem)
				case <-ctx.Done():
					// drain the semaphore — wait for running goroutines to finish
					for i := 0; i < c.cfg.Concurrency; i++ {
						sem <- struct{}{}
					}
					return ctx.Err()
				}
			}
		}
	}
}

func (c *StreamConsumer[T]) handle(ctx context.Context, m redis.XMessage, sem chan struct{}) {
	defer func() { <-sem }() // pop one out when done

	ctx = log.WithContext(ctx, "id", m.ID)

	var env messaging.Envelope[T]

	// A panic in message handling (template Execute, mailer.Send, nil deref)
	// must not crash the whole worker: recover, log with stack, and route the
	// message to the DLQ (or leave it in the PEL) so it is not lost.
	defer func() {
		if r := recover(); r != nil {
			err := fmt.Errorf("panic handling message: %v", r)
			log.ErrorCtx(ctx, "Recovered panic, moving to DLQ", err, "stack", string(debug.Stack()))
			// Release the dedup claim so a redelivery re-processes instead of
			// being skipped as a duplicate.
			if env.ID != "" {
				if rerr := c.rdb.Del(ctx, dedupKey(c.cfg.Stream, env.ID)).Err(); rerr != nil {
					log.WarnCtx(ctx, "Failed to release dedup claim after panic", rerr, "id", env.ID)
				}
			}
			if dlqErr := c.toDLQ(ctx, m, "panic", err.Error()); dlqErr != nil {
				// Do not ACK: leave in PEL so XAUTOCLAIM retries rather than losing it.
				return
			}
			c.ack(ctx, m.ID)
		}
	}()

	env, err := decodeMessage[T](m)
	if err != nil {
		log.ErrorCtx(ctx, "Failed to decode message, moving to DLQ", err)
		if dlqErr := c.toDLQ(ctx, m, "decode_error", err.Error()); dlqErr != nil {
			// Do not ACK: leave in PEL so XAUTOCLAIM retries rather than losing it.
			return
		}
		c.ack(ctx, m.ID)
		return
	}

	ctx = log.WithContext(ctx, "iid", env.ID)

	// Idempotency: claim the envelope ID before processing so a redelivery
	// (e.g. a XAck lost during shutdown after a successful send) is skipped
	// instead of sending duplicate mail. The claim is released on failure so
	// the retry/DLQ path below re-processes on redelivery.
	key := dedupKey(c.cfg.Stream, env.ID)
	// SetNX is deprecated in go-redis v9 — use SET ... NX via SetArgs instead.
	// redis.Nil means the key already existed (duplicate delivery); any other
	// error is a real failure (process anyway — at-least-once semantics).
	if _, err := c.rdb.SetArgs(ctx, key, "1", redis.SetArgs{Mode: "NX", TTL: dedupTTL}).Result(); errors.Is(err, redis.Nil) {
		log.DebugCtx(ctx, "Duplicate delivery, already processed, acking", "id", env.ID)
		c.ack(ctx, m.ID)
		return
	} else if err != nil {
		log.WarnCtx(ctx, "Dedup check failed, processing anyway", err, "id", env.ID)
	}

	if err = c.handler(ctx, env); err == nil {
		log.DebugCtx(ctx, "Message handled")
		c.ack(ctx, m.ID)
		return
	}

	// Handler failed — release the dedup claim so the retry/DLQ decision below
	// can re-process on redelivery instead of being skipped as a duplicate.
	if rerr := c.rdb.Del(ctx, key).Err(); rerr != nil {
		log.WarnCtx(ctx, "Failed to release dedup claim", rerr, "id", env.ID)
	}

	// Handler failed — check delivery count before deciding.
	deliveries := c.deliveryCount(ctx, m.ID)
	log.ErrorCtx(ctx, "Failed to handle message", err, "deliveries", deliveries)

	if deliveries >= c.cfg.MaxRetries {
		log.WarnCtx(ctx, "Handler max retries exceeded, moving to DLQ")
		if dlqErr := c.toDLQ(ctx, m, "max_retries", err.Error()); dlqErr != nil {
			// Do not ACK: leave in PEL so XAUTOCLAIM retries rather than losing it.
			return
		}
		c.ack(ctx, m.ID)
		return
	}

	// No ACK — stays in PEL, XAUTOCLAIM will reclaim after MinIdle.
	log.DebugCtx(
		ctx, "Message will retry via XAUTOCLAIM",
		"deliveries", deliveries,
		"next_retry_after", c.cfg.MinIdle,
	)
}

func (c *StreamConsumer[T]) claimPending(ctx context.Context, sem chan struct{}) {
	cursor := "0-0"
	for {
		res, next, err := c.rdb.XAutoClaim(ctx, &redis.XAutoClaimArgs{
			Stream:   c.cfg.Stream,
			Group:    c.cfg.Group,
			Consumer: c.cfg.Consumer,
			MinIdle:  c.cfg.MinIdle,
			Start:    cursor,
			Count:    c.cfg.BatchSize,
		}).Result()
		if err != nil {
			log.ErrorCtx(ctx, "XAUTOCLAIM error", err)
			return
		}
		if len(res) > 0 {
			log.DebugCtx(ctx, "Reclaimed pending messages", "count", len(res))
		}
		for _, m := range res {
			select {
			case sem <- struct{}{}:
				go c.handle(ctx, m, sem)
			case <-ctx.Done():
				// Saturated semaphore during shutdown: leave the rest in the
				// PEL for the next reclaim instead of blocking forever.
				return
			}
		}
		if next == "0-0" || next == "0" {
			return
		}
		cursor = next
	}
}

// deliveryCount reads Redis' native delivery counter from the PEL.
// XAUTOCLAIM increments this automatically — no extra writes needed.
func (c *StreamConsumer[T]) deliveryCount(ctx context.Context, id string) int64 {
	res, err := c.rdb.XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream: c.cfg.Stream,
		Group:  c.cfg.Group,
		Start:  id,
		End:    id,
		Count:  1,
	}).Result()
	if err != nil || len(res) == 0 {
		return 1
	}
	return res[0].RetryCount
}

func (c *StreamConsumer[T]) ack(ctx context.Context, id string) {
	// Ack with a non-cancellable ctx: if we got here the work is done, so the
	// ack must still land during graceful shutdown or the message redelivers
	// (duplicate side effects). go-redis' own read/write timeouts bound it.
	ctx = context.WithoutCancel(ctx)
	if err := c.rdb.XAck(ctx, c.cfg.Stream, c.cfg.Group, id).Err(); err != nil {
		log.ErrorCtx(ctx, "ACK failed", err, "id", id)
	}
}

// toDLQ writes to the DLQ stream, preserving original payload plus metadata.
// It MUST be called BEFORE ack: the caller only acks once this returns nil, so a
// failed DLQ write leaves the message in the PEL for XAUTOCLAIM to retry
// (duplicates in the DLQ are acceptable; lost messages are not).
//
// When DLQ is disabled it returns nil so the caller acks and drops the message,
// preserving the previous behavior.
func (c *StreamConsumer[T]) toDLQ(ctx context.Context, m redis.XMessage, reason, errMsg string) error {
	// Same reasoning as ack: a completed DLQ write must survive graceful
	// shutdown. go-redis' own read/write timeouts bound it.
	ctx = context.WithoutCancel(ctx)
	if !c.cfg.UseDLQ {
		return nil
	}

	values := make(map[string]any, len(m.Values)+3)
	maps.Copy(values, m.Values)

	values["_original_id"] = m.ID
	values["_reason"] = reason
	values["_error"] = errMsg
	values["_failed_at"] = time.Now().UTC().Format(time.RFC3339)

	if err := c.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: c.cfg.Stream + ".dlq",
		Values: values,
	}).Err(); err != nil {
		log.ErrorCtx(ctx, "DLQ write failed", err, "id", m.ID)
		return err
	}
	return nil
}

func (c *StreamConsumer[T]) ensureGroup(ctx context.Context) error {
	err := c.rdb.XGroupCreateMkStream(ctx, c.cfg.Stream, c.cfg.Group, "$").Err()
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		return fmt.Errorf("create consumer group: %w", err)
	}
	return nil
}

func sleep(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}

func decodeMessage[T any](m redis.XMessage) (messaging.Envelope[T], error) {
	var env messaging.Envelope[T]
	raw, ok := m.Values["payload"].(string)
	if !ok {
		return env, fmt.Errorf("missing payload")
	}
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		return env, err
	}
	return env, nil
}
