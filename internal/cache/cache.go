package cache

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

var ErrLockNotHeld = errors.New("lock not held")

type Cache struct {
	client *redis.Client
	prefix string
}

func New(cfg Config) (*Cache, error) {
	if strings.TrimSpace(cfg.Addr) == "" {
		return nil, fmt.Errorf("redis addr is required")
	}
	opts := &redis.Options{
		Addr:         cfg.Addr,
		Password:     cfg.Password,
		DB:           cfg.DB,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolSize:     10,
		MinIdleConns: 2,
	}
	if cfg.Username != "" {
		opts.Username = cfg.Username
	}
	if cfg.TLS != nil {
		opts.TLSConfig = cfg.TLS
	}

	client := redis.NewClient(opts)
	if err := client.Ping(context.Background()).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("redis ping: %w", err)
	}

	slog.Info("cache: connected to Redis",
		"addr", cfg.Addr,
		"db", cfg.DB,
		"prefix", cfg.KeyPrefix,
		"tls", cfg.TLS != nil,
	)
	return &Cache{client: client, prefix: cfg.KeyPrefix}, nil
}

func (c *Cache) Close() error {
	return c.client.Close()
}

func (c *Cache) Health(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}

func (c *Cache) key(k string) string {
	if c.prefix == "" {
		return k
	}
	return c.prefix + k
}

func (c *Cache) Get(ctx context.Context, key string) ([]byte, error) {
	data, err := c.client.Get(ctx, c.key(key)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("redis get %q: %w", key, err)
	}
	return data, nil
}

func (c *Cache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if err := c.client.Set(ctx, c.key(key), value, ttl).Err(); err != nil {
		return fmt.Errorf("redis set %q: %w", key, err)
	}
	return nil
}

func (c *Cache) Delete(ctx context.Context, keys ...string) (int64, error) {
	if len(keys) == 0 {
		return 0, nil
	}
	prefixed := make([]string, len(keys))
	for i, k := range keys {
		prefixed[i] = c.key(k)
	}
	n, err := c.client.Del(ctx, prefixed...).Result()
	if err != nil {
		return 0, fmt.Errorf("redis del %v: %w", keys, err)
	}
	return n, nil
}

func (c *Cache) Exists(ctx context.Context, key string) (bool, error) {
	n, err := c.client.Exists(ctx, c.key(key)).Result()
	if err != nil {
		return false, fmt.Errorf("redis exists %q: %w", key, err)
	}
	return n > 0, nil
}

func (c *Cache) Incr(ctx context.Context, key string, delta int64) (int64, error) {
	n, err := c.client.IncrBy(ctx, c.key(key), delta).Result()
	if err != nil {
		return 0, fmt.Errorf("redis incr %q: %w", key, err)
	}
	return n, nil
}

var casScript = redis.NewScript(`
local current = redis.call("GET", KEYS[1])
local expect_missing = ARGV[1]
local expected = ARGV[2]
local newval = ARGV[3]
if expect_missing == "1" then
	if current ~= false then
		return 0
	end
	redis.call("SET", KEYS[1], newval)
else
	if current ~= expected then
		return 0
	end
	local pttl = redis.call("PTTL", KEYS[1])
	if pttl > 0 then
		redis.call("SET", KEYS[1], newval, "PX", pttl)
	else
		redis.call("SET", KEYS[1], newval)
	end
end
return 1
`)

func (c *Cache) CompareAndSwap(ctx context.Context, key string, oldValue, newValue []byte) (bool, error) {
	expectMissing := "0"
	var expected any = oldValue
	if oldValue == nil {
		expectMissing = "1"
		expected = ""
	}
	n, err := casScript.Run(ctx, c.client, []string{c.key(key)}, expectMissing, expected, newValue).Int64()
	if err != nil {
		return false, fmt.Errorf("redis cas %q: %w", key, err)
	}
	return n == 1, nil
}

var unlockScript = redis.NewScript(`
	if redis.call("GET", KEYS[1]) == ARGV[1] then
		return redis.call("DEL", KEYS[1])
	else
		return 0
	end
`)

func encodeLockToken(redisKey, redisToken string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(redisKey)) + "." + redisToken
}

func decodeLockToken(token string) (redisKey, redisToken string, err error) {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid lock token")
	}
	keyBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", "", fmt.Errorf("invalid lock token key: %w", err)
	}
	return string(keyBytes), parts[1], nil
}

func (c *Cache) Lock(ctx context.Context, key string, ttl time.Duration) (string, error) {
	redisKey := c.key(key)
	randBytes := make([]byte, 16)
	if _, err := rand.Read(randBytes); err != nil {
		return "", fmt.Errorf("lock token: %w", err)
	}
	redisToken := hex.EncodeToString(randBytes)
	ok, err := c.client.SetNX(ctx, redisKey, redisToken, ttl).Result()
	if err != nil {
		return "", fmt.Errorf("redis lock %q: %w", key, err)
	}
	if !ok {
		return "", fmt.Errorf("redis lock %q: already locked", key)
	}
	return encodeLockToken(redisKey, redisToken), nil
}

func (c *Cache) UnlockByToken(ctx context.Context, token string) error {
	redisKey, redisToken, err := decodeLockToken(token)
	if err != nil {
		return err
	}
	result, err := unlockScript.Run(ctx, c.client, []string{redisKey}, redisToken).Result()
	if err != nil {
		return fmt.Errorf("redis unlock: %w", err)
	}
	if result.(int64) == 0 {
		return ErrLockNotHeld
	}
	return nil
}

func (c *Cache) Publish(ctx context.Context, channel string, msg []byte) error {
	if err := c.client.Publish(ctx, c.key(channel), msg).Err(); err != nil {
		return fmt.Errorf("redis publish %q: %w", channel, err)
	}
	return nil
}

func (c *Cache) Subscribe(ctx context.Context, channel string) (<-chan []byte, error) {
	pubsub := c.client.Subscribe(ctx, c.key(channel))
	ch := make(chan []byte, 256)
	go func() {
		defer close(ch)
		defer func() { _ = pubsub.Close() }()
		subCh := pubsub.Channel()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-subCh:
				if !ok {
					return
				}
				payload := []byte(msg.Payload)
				select {
				case ch <- payload:
				case <-ctx.Done():
					return
				default:
					slog.Warn("cache: subscribe channel full, dropping message",
						"channel", channel)
				}
			}
		}
	}()
	return ch, nil
}
