package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/prawirdani/golang-restapi/internal/infrastructure/messaging"
	"github.com/prawirdani/golang-restapi/pkg/log"
	"github.com/redis/go-redis/v9"
)

type ConsumerConfig struct {
	Stream      string
	Group       string
	Consumer    string
	DLQStream   string
	BatchSize   int64
	Concurrency int
	Block       time.Duration
	MinIdle     time.Duration // idle threshold before XAUTOCLAIM reclaims
	MaxRetries  int64
}

type StreamConsumer[T any] struct {
	rdb     *redis.Client
	cfg     ConsumerConfig
	handler messaging.Handler[T]
}

func NewStreamConsumer[T any](rdb *redis.Client, cfg ConsumerConfig, h messaging.Handler[T]) *StreamConsumer[T] {
	return &StreamConsumer[T]{rdb: rdb, cfg: cfg, handler: h}
}

func (c *StreamConsumer[T]) Start(ctx context.Context) error {
	if err := c.ensureGroup(ctx); err != nil {
		return err
	}

	// buffered semaphore
	sem := make(chan struct{}, c.cfg.Concurrency)

	ctx = log.WithContext(ctx, "stream", c.cfg.Stream)
	log.InfoCtx(ctx, "Consumer started",
		"group", c.cfg.Group,
		"consumer", c.cfg.Consumer,
	)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		c.claimPending(ctx, sem)

		streams, err := c.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    c.cfg.Group,
			Consumer: c.cfg.Consumer,
			Streams:  []string{c.cfg.Stream, ">"},
			Count:    c.cfg.BatchSize,
			Block:    c.cfg.Block,
		}).Result()
		if err != nil {
			if err != redis.Nil {
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

	env, err := decodeMessage[T](m)
	if err != nil {
		log.ErrorCtx(ctx, "Failed to decode message, moving to DLQ", err)
		c.ack(ctx, m.ID)
		c.toDLQ(ctx, m, "decode_error", err.Error())
		return
	}

	ctx = log.WithContext(ctx, "iid", env.ID)

	if err = c.handler(ctx, env); err == nil {
		log.DebugCtx(ctx, "Message handled")
		c.ack(ctx, m.ID)
		return
	}

	// Handler failed — check delivery count before deciding.
	deliveries := c.deliveryCount(ctx, m.ID)
	log.ErrorCtx(ctx, "Failed to handle message", err, "deliveries", deliveries)

	if deliveries >= c.cfg.MaxRetries {
		log.WarnCtx(ctx, "Handler max retries exceeded, moving to DLQ")
		c.ack(ctx, m.ID)
		c.toDLQ(ctx, m, "max_retries", err.Error())
		return
	}

	// No ACK — stays in PEL, XAUTOCLAIM will reclaim after MinIdle.
	log.DebugCtx(ctx, "Message will retry via XAUTOCLAIM",
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
			sem <- struct{}{}
			go c.handle(ctx, m, sem)
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
	if err := c.rdb.XAck(ctx, c.cfg.Stream, c.cfg.Group, id).Err(); err != nil {
		log.ErrorCtx(ctx, "ACK failed", err, "id", id)
	}
}

// toDLQ writes to the DLQ stream, preserving original payload plus metadata.
// Called AFTER ack — duplicates in DLQ are acceptable; lost messages are not.
func (c *StreamConsumer[T]) toDLQ(ctx context.Context, m redis.XMessage, reason, errMsg string) {
	if c.cfg.DLQStream == "" {
		return
	}

	values := make(map[string]any, len(m.Values)+3)
	maps.Copy(values, m.Values)

	values["_original_id"] = m.ID
	values["_reason"] = reason
	values["_error"] = errMsg
	values["_failed_at"] = time.Now().UTC().Format(time.RFC3339)

	if err := c.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: c.cfg.DLQStream,
		Values: values,
	}).Err(); err != nil {
		log.ErrorCtx(ctx, "DLQ write failed", err, "id", m.ID)
	}
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

// // Idempotent wraps a handler with SetNX dedup on envelope ID.
// // The handler never sees duplicate deliveries.
// func Idempotent[T any](rdb *redis.Client, ttl time.Duration, h messaging.Handler[Envelope[T]])
// messaging.Handler[Envelope[T]] {
//     return func(ctx context.Context, env Envelope[T]) error {
//         key := "processed:" + env.ID
//         set, err := rdb.SetNX(ctx, key, 1, ttl).Result()
//         if err != nil {
//             return err
//         }
//         if !set {
//             log.DebugCtx(ctx, "Duplicate message, skipping", "envelope_id", env.ID)
//             return nil // nil = consumer will ACK it
//         }
//         return h(ctx, env)
//     }
// }
