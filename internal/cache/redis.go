package cache

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

type Cache struct {
	client *redis.Client
}

func New(addr string, password string, db int) *Cache {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	return &Cache{
		client: client,
	}
}

// Close the Redis connection
func (c *Cache) Close() error {
	return c.client.Close()
}

// ========== Generic Cache Methods ==========

// Get retrieves a value from cache by key
func (c *Cache) Get(key string) ([]byte, bool) {
	ctx := context.Background()
	data, err := c.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, false
		}
		return nil, false
	}
	return []byte(data), true
}

// Set stores a value in cache with TTL
func (c *Cache) Set(key string, value []byte, ttl time.Duration) {
	ctx := context.Background()
	c.client.Set(ctx, key, value, ttl)
}

// Delete removes a key from cache
func (c *Cache) Delete(key string) {
	ctx := context.Background()
	c.client.Del(ctx, key)
}

