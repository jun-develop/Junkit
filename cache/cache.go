// Package cache provides a generic key-value cache supporting Redis, in-memory,
// and no-op backends.
package cache

import (
	"context"
	"errors"
	"time"
)

// ErrNotInteger is returned by Incr on a non-integer cache.
var ErrNotInteger = errors.New("cache: Incr requires Cache[int64] or Cache[any]")

// Logger defines a simple interface for logging non-fatal cache errors.
type Logger interface {
	Error(msg string, keysAndValues ...any)
}

// Cache is a typed key-value store implemented by RedisCache, MemoryCache and NoopCache.
// Get reports whether key was found (a miss returns the zero value); Set stores a value for ttl
// (0 = no expiry); Incr atomically adds 1 and returns the new value, applying ttl only when it
// creates the key (a fixed window); GetOrSet calls fetchFn on a miss and stores the result;
// Close releases underlying resources such as connections.
type Cache[T any] interface {
	Get(ctx context.Context, key string) (T, bool)
	Set(ctx context.Context, key string, value T, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
	Incr(ctx context.Context, key string, ttl time.Duration) (int64, error)
	GetOrSet(ctx context.Context, key string, ttl time.Duration, fetchFn func() (T, error)) (T, error)
	Close() error
}
