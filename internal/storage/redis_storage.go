package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/ddddao/ticker-service-v2/internal/config"
	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
)

// RedisStorage implements Storage interface using Redis
type RedisStorage struct {
	client *redis.Client
	ttl    time.Duration
}

// NewRedisStorage creates a new Redis storage
func NewRedisStorage(cfg config.RedisConfig) (*RedisStorage, error) {
	// If Redis is not configured, return nil
	if cfg.Addr == "" {
		return nil, fmt.Errorf("redis not configured")
	}

	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	logrus.WithField("app", "ticker-service-v2").Info("Connected to Redis successfully")
	
	return &RedisStorage{
		client: client,
		ttl:    30 * time.Second, // Default TTL for ticker data
	}, nil
}

// Publish stores ticker data in Redis and publishes to channel
func (r *RedisStorage) Publish(ctx context.Context, exchange, symbol string, data []byte) error {
	// Store in Redis with TTL
	key := fmt.Sprintf("ticker:%s:%s", exchange, symbol)
	if err := r.client.Set(ctx, key, data, r.ttl).Err(); err != nil {
		return fmt.Errorf("failed to store ticker data: %w", err)
	}

	// Publish to channel for real-time subscribers
	channel := fmt.Sprintf("ticker:%s:%s", exchange, symbol)
	if err := r.client.Publish(ctx, channel, data).Err(); err != nil {
		// Log but don't fail if publish fails
		logrus.WithError(err).Warnf("Failed to publish to channel %s", channel)
	}

	return nil
}

// Get retrieves ticker data from Redis
func (r *RedisStorage) Get(ctx context.Context, exchange, symbol string) ([]byte, error) {
	key := fmt.Sprintf("ticker:%s:%s", exchange, symbol)
	data, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, fmt.Errorf("ticker data not found")
		}
		return nil, fmt.Errorf("failed to get ticker data: %w", err)
	}
	return data, nil
}

// GetAll retrieves all ticker data for an exchange
func (r *RedisStorage) GetAll(ctx context.Context, exchange string) (map[string][]byte, error) {
	pattern := fmt.Sprintf("ticker:%s:*", exchange)
	keys, err := r.client.Keys(ctx, pattern).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get keys: %w", err)
	}

	result := make(map[string][]byte)
	for _, key := range keys {
		data, err := r.client.Get(ctx, key).Bytes()
		if err != nil {
			continue // Skip failed keys
		}
		// Extract symbol from key (ticker:exchange:symbol)
		symbol := key[len(fmt.Sprintf("ticker:%s:", exchange)):]
		result[symbol] = data
	}

	return result, nil
}

// Close closes the Redis connection
func (r *RedisStorage) Close() error {
	return r.client.Close()
}

// IsConnected returns true if Redis is connected
func (r *RedisStorage) IsConnected() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	
	err := r.client.Ping(ctx).Err()
	return err == nil
}