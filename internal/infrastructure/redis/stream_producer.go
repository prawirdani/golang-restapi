package redis

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/prawirdani/golang-restapi/internal/messaging"
	"github.com/redis/go-redis/v9"
)

func produceStream[T any](ctx context.Context, rdb *redis.Client, stream string, env messaging.Envelope[T]) error {
	b, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("marshal envelope: %w", err)
	}

	return rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: stream,
		Values: map[string]any{"payload": string(b)},
	}).Err()
}
