package cache

import (
	"context"
	"log"
	"os"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// Cache defines the interface for storing and retrieving screenshot results.
type Cache interface {
	Get(key string) ([]byte, bool)
	Set(key string, value []byte, ttl time.Duration)
	Delete(key string)
}

// ───────────────────────────────────────────────────────────────────────────────
// Memory Cache (fallback)
// ───────────────────────────────────────────────────────────────────────────────

type memEntry struct {
	data      []byte
	expiresAt time.Time
}

// MemoryCache is an in-process cache backed by sync.Map with per-entry TTL.
type MemoryCache struct {
	m sync.Map
}

func (c *MemoryCache) Get(key string) ([]byte, bool) {
	v, ok := c.m.Load(key)
	if !ok {
		return nil, false
	}
	e := v.(memEntry)
	if time.Now().After(e.expiresAt) {
		c.m.Delete(key)
		return nil, false
	}
	return e.data, true
}

func (c *MemoryCache) Set(key string, value []byte, ttl time.Duration) {
	c.m.Store(key, memEntry{data: value, expiresAt: time.Now().Add(ttl)})
}

func (c *MemoryCache) Delete(key string) {
	c.m.Delete(key)
}

// ───────────────────────────────────────────────────────────────────────────────
// Redis Cache (optional)
// ───────────────────────────────────────────────────────────────────────────────

// RedisCache wraps go-redis for a distributed, persistent cache.
type RedisCache struct {
	client *redis.Client
	ctx    context.Context
}

func (c *RedisCache) Get(key string) ([]byte, bool) {
	data, err := c.client.Get(c.ctx, key).Bytes()
	if err != nil {
		return nil, false
	}
	return data, true
}

func (c *RedisCache) Set(key string, value []byte, ttl time.Duration) {
	if err := c.client.Set(c.ctx, key, value, ttl).Err(); err != nil {
		log.Printf("[Cache] Redis Set error: %v", err)
	}
}

func (c *RedisCache) Delete(key string) {
	c.client.Del(c.ctx, key)
}

// ───────────────────────────────────────────────────────────────────────────────
// Factory
// ───────────────────────────────────────────────────────────────────────────────

// NewCache creates a Cache backed by Redis if REDIS_URL is set and reachable.
// Falls back silently to an in-process MemoryCache.
func NewCache() Cache {
	redisURL := os.Getenv("REDIS_URL")
	if redisURL != "" {
		opt, err := redis.ParseURL(redisURL)
		if err == nil {
			client := redis.NewClient(opt)
			ctx := context.Background()
			if pingErr := client.Ping(ctx).Err(); pingErr == nil {
				log.Printf("[Cache] Usando Redis (%s)", redisURL)
				return &RedisCache{client: client, ctx: ctx}
			}
			log.Printf("[Cache] Redis no disponible, usando memoria")
		} else {
			log.Printf("[Cache] URL de Redis inválida: %v, usando memoria", err)
		}
	} else {
		log.Printf("[Cache] REDIS_URL no configurado, usando memoria")
	}
	return &MemoryCache{}
}
