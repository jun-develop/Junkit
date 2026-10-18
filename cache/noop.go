package cache

import (
	"context"
	"time"
)

// NoopCache is a Cache that disables caching: every read misses and writes are ignored.
type NoopCache[T any] struct{}

// NewNoopCache creates a new NoopCache instance.
func NewNoopCache[T any]() *NoopCache[T] {
	return &NoopCache[T]{}
}

// Get implements Cache; it always reports a miss.
func (c *NoopCache[T]) Get(context.Context, string) (T, bool) {
	var empty T
	return empty, false
}

// Set implements Cache; it is a no-op that always succeeds.
func (c *NoopCache[T]) Set(context.Context, string, T, time.Duration) error {
	return nil
}

// Incr implements Cache; it always returns 0.
func (c *NoopCache[T]) Incr(context.Context, string, time.Duration) (int64, error) {
	return 0, nil
}

// Delete implements Cache; it is a no-op that always succeeds.
func (c *NoopCache[T]) Delete(context.Context, string) error {
	return nil
}

// Exists implements Cache; it always reports false.
func (c *NoopCache[T]) Exists(context.Context, string) (bool, error) {
	return false, nil
}

// GetOrSet implements Cache; it always calls fetchFn and discards the result.
func (c *NoopCache[T]) GetOrSet(_ context.Context, _ string, _ time.Duration, fetchFn func() (T, error)) (T, error) {
	return fetchFn()
}

// Close implements Cache; it is a no-op.
func (c *NoopCache[T]) Close() error {
	return nil
}
