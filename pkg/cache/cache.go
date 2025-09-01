package cache

import (
	"context"
	"fmt"
	"time"
)

type Cache interface {
	Get(ctx context.Context, key string, dest any) error
	Set(ctx context.Context, key string, value any, expiration time.Duration) error
	Delete(ctx context.Context, key string) error
	Clear(ctx context.Context) error
	Close() error
}

type CacheMiddleware struct {
	cache Cache
	ttl   time.Duration
}

func NewCacheMiddleware(cache Cache, ttl time.Duration) *CacheMiddleware {
	return &CacheMiddleware{
		cache: cache,
		ttl:   ttl,
	}
}

func GenerateKey(prefix string, params ...any) string {
	if len(params) == 0 {
		return prefix
	}
	return fmt.Sprintf("%s:%v", prefix, params)
}
