package messaging

import "context"

type Handler[T any] func(ctx context.Context, env Envelope[T]) error
