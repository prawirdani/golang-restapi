package redis

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/prawirdani/golang-restapi/internal/throttle"
	"github.com/redis/go-redis/v9"
)

const keyPrefix = "throttle:"

// Throttler implements [throttle.Throttler] on Redis.
type Throttler struct {
	client *redis.Client
}

func NewThrottler(client *redis.Client) *Throttler {
	return &Throttler{
		client: client,
	}
}

func (s *Throttler) TryAcquire(
	ctx context.Context,
	key string,
	ttl time.Duration,
) (throttle.Result, error) {
	if key == "" {
		return throttle.Result{}, errors.New("throttle key cannot be empty")
	}
	if ttl <= 0 {
		return throttle.Result{}, errors.New("throttle TTL must be greater than zero")
	}
	key = keyPrefix + key

	result, err := s.client.SetArgs(
		ctx,
		key,
		"L",
		redis.SetArgs{
			Mode: "NX",
			TTL:  ttl,
		},
	).Result()

	if err != nil && !errors.Is(err, redis.Nil) {
		return throttle.Result{}, fmt.Errorf("acquire throttle: %w", err)
	}

	now := time.Now()
	if err == nil && result == "OK" {
		// manual inject ttl on first timer success.
		return throttle.Result{Allowed: true, RetryAfter: now.Add(ttl).UTC()}, nil
	}

	retryAfter, err := s.client.TTL(ctx, key).Result()
	if err != nil {
		return throttle.Result{}, fmt.Errorf("get throttle TTL: %w", err)
	}

	return throttle.Result{
		Allowed:    false,
		RetryAfter: now.Add(retryAfter).UTC(),
	}, nil
}

func (s *Throttler) Release(
	ctx context.Context,
	key string,
) error {
	if key == "" {
		return errors.New("throttle key cannot be empty")
	}

	if err := s.client.Del(
		ctx,
		keyPrefix+key,
	).Err(); err != nil {
		return fmt.Errorf(
			"release throttle: %w",
			err,
		)
	}

	return nil
}
