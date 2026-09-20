package cache

import (
	"context"
	"time"
)

type Store interface {
	Set(ctx context.Context, key, value string, expireTime time.Duration)
	Get(ctx context.Context, key string) string
	Has(ctx context.Context, key string) bool
	Forget(ctx context.Context, key string)
	Forever(ctx context.Context, key string, value any)
	Flush(ctx context.Context)
	IsAlive(ctx context.Context) error
	Increment(ctx context.Context, parameters ...any)
	Decrement(ctx context.Context, parameters ...any)
}
