package cache

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

var ErrLockNotHeld = errors.New("lock not held")

type Cache struct {
	client *redis.Client
}

func New(addr, password string, db int) (*Cache, error) {
	client := redis.NewClient(&redis.Options{
		Addr:            addr,
		Password:        password,
		DB:              db,
		DialTimeout:     5 * time.Second,
		ReadTimeout:     3 * time.Second,
		WriteTimeout:    3 * time.Second,
		MaxRetries:      3,
		MinRetryBackoff: 100 * time.Millisecond,
		MaxRetryBackoff: 2 * time.Second,
		PoolSize:        10,
		MinIdleConns:    2,
		PoolTimeout:     4 * time.Second,
	})

	if err := client.Ping(context.Background()).Err(); err != nil {
		return nil, fmt.Errorf("redis ping: %w", err)
	}

	slog.Info("cache: connected to Redis", "addr", addr, "db", db)
	return &Cache{client: client}, nil
}

func (c *Cache) Close() error {
	return c.client.Close()
}

func (c *Cache) Health(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}

func (c *Cache) Get(ctx context.Context, key string) ([]byte, error) {
	data, err := c.client.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("redis get %q: %w", key, err)
	}
	return data, nil
}

func (c *Cache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if err := c.client.Set(ctx, key, value, ttl).Err(); err != nil {
		return fmt.Errorf("redis set %q: %w", key, err)
	}
	return nil
}

func (c *Cache) Delete(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	if err := c.client.Del(ctx, keys...).Err(); err != nil {
		return fmt.Errorf("redis del %v: %w", keys, err)
	}
	return nil
}

func (c *Cache) Exists(ctx context.Context, key string) (bool, error) {
	n, err := c.client.Exists(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("redis exists %q: %w", key, err)
	}
	return n > 0, nil
}

func (c *Cache) Incr(ctx context.Context, key string, delta int64) (int64, error) {
	n, err := c.client.IncrBy(ctx, key, delta).Result()
	if err != nil {
		return 0, fmt.Errorf("redis incr %q: %w", key, err)
	}
	return n, nil
}

func (c *Cache) CompareAndSwap(ctx context.Context, key string, oldValue, newValue []byte) (bool, error) {
	var script *redis.Script
	var args []interface{}
	if oldValue == nil {
		script = redis.NewScript(`
			local exists = redis.call("EXISTS", KEYS[1])
			if exists == 1 then
				return 0
			end
			redis.call("SET", KEYS[1], ARGV[1])
			return 1
		`)
		args = []interface{}{newValue}
	} else {
		script = redis.NewScript(`
			local current = redis.call("GET", KEYS[1])
			if current ~= ARGV[1] then
				return 0
			end
			redis.call("SET", KEYS[1], ARGV[2])
			return 1
		`)
		args = []interface{}{oldValue, newValue}
	}
	result, err := script.Run(ctx, c.client, []string{key}, args...).Result()
	if err != nil {
		return false, fmt.Errorf("redis cas %q: %w", key, err)
	}
	return result.(int64) == 1, nil
}

type Lock struct {
	client *redis.Client
	key    string
	token  string
}

func (l *Lock) Unlock(ctx context.Context) error {
	script := redis.NewScript(`
		if redis.call("GET", KEYS[1]) == ARGV[1] then
			return redis.call("DEL", KEYS[1])
		else
			return 0
		end
	`)
	result, err := script.Run(ctx, l.client, []string{l.key}, l.token).Result()
	if err != nil {
		return fmt.Errorf("redis unlock %q: %w", l.key, err)
	}
	if result.(int64) == 0 {
		return ErrLockNotHeld
	}
	return nil
}

func (c *Cache) Lock(ctx context.Context, key string, ttl time.Duration) (*Lock, error) {
	token := fmt.Sprintf("%x", time.Now().UnixNano())
	ok, err := c.client.SetNX(ctx, key, token, ttl).Result()
	if err != nil {
		return nil, fmt.Errorf("redis lock %q: %w", key, err)
	}
	if !ok {
		return nil, fmt.Errorf("redis lock %q: already locked", key)
	}
	return &Lock{client: c.client, key: key, token: token}, nil
}

func (c *Cache) Publish(ctx context.Context, channel string, msg []byte) error {
	if err := c.client.Publish(ctx, channel, msg).Err(); err != nil {
		return fmt.Errorf("redis publish %q: %w", channel, err)
	}
	return nil
}

func (c *Cache) Subscribe(ctx context.Context, channel string) (<-chan []byte, error) {
	pubsub := c.client.Subscribe(ctx, channel)

	ch := make(chan []byte, 256)
	go func() {
		defer pubsub.Close()
		for msg := range pubsub.Channel() {
			select {
			case ch <- []byte(msg.Payload):
			default:
				slog.Warn("cache: subscribe channel full, dropping message",
					"channel", channel)
			}
		}
	}()

	return ch, nil
}
