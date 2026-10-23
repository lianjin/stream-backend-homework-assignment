package redis

import (
	"context"
	"fmt"

	"github.com/GetStream/stream-backend-homework-assignment/api"
	"github.com/redis/go-redis/v9"
)

// Redis provides caching in Redis.
type Redis struct {
	cli *redis.Client
}

// Connect connects to the Redis server and pings the server to ensure the
// connection is working.
func Connect(ctx context.Context, addr string) (*Redis, error) {
	cli := redis.NewClient(&redis.Options{
		Addr: addr,
	})
	if err := cli.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("ping redis: %w", err)
	}
	return &Redis{
		cli: cli,
	}, nil
}

const (
	messagePrefix = "messages"
	maxSize       = 10
)

func messageKey(id string) string {
	return fmt.Sprintf("%s:%s", messagePrefix, id)
}

// ListMessages returns a list of message from Redis. The messages are sorted
// by the timestamp in descending order.
func (r *Redis) ListMessages(ctx context.Context) ([]api.Message, error) {
	vals, err := r.cli.ZRevRange(ctx, messagePrefix, 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("zrange: %w", err)
	}

	out := make([]api.Message, 0, len(vals))
	for _, key := range vals {
		var msg message
		err = r.cli.HGetAll(ctx, key).Scan(&msg)
		if err != nil {
			return nil, fmt.Errorf("hgetall: %w", err)
		}
		out = append(out, msg.APIMessage())
	}

	return out, nil
}

// InsertMessage adds the message to Redis with the message:MESSAGE_ID as the key and adds the key to a sorted set.
func (r *Redis) InsertMessage(ctx context.Context, msg api.Message) error {
	m := fromAPIMessage(msg)
	key := messageKey(m.ID)

	err := r.cli.Watch(ctx, func(tx *redis.Tx) error {
		_, err := tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			addMessage(ctx, pipe, key, m)
			return nil
		})
		return err
	}, key)

	if err != nil {
		return fmt.Errorf("redis insert message: %w", err)
	}

	if err := r.evictOldest(ctx); err != nil {
		return fmt.Errorf("evict oldest: %w", err)
	}
	return nil
}

// ReplaceMessages rebuilds the latest-message cache to exactly msgs.
func (r *Redis) ReplaceMessages(ctx context.Context, msgs []api.Message) error {
	old, err := r.cli.ZRange(ctx, messagePrefix, 0, -1).Result()
	if err != nil {
		return fmt.Errorf("zrange: %w", err)
	}

	_, err = r.cli.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		if len(old) > 0 {
			pipe.Del(ctx, old...)
		}
		pipe.Del(ctx, messagePrefix)
		for _, msg := range msgs {
			m := fromAPIMessage(msg)
			addMessage(ctx, pipe, messageKey(m.ID), m)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("redis replace messages: %w", err)
	}
	return nil
}

func addMessage(ctx context.Context, pipe redis.Pipeliner, key string, m message) {
	pipe.HSet(ctx, key, m)
	pipe.ZAdd(ctx, messagePrefix, redis.Z{
		Score:  float64(m.CreatedAt.UnixNano()),
		Member: key,
	})
}

func (r *Redis) evictOldest(ctx context.Context) error {
	vals, err := r.cli.ZRange(ctx, messagePrefix, 0, int64(-maxSize-1)).Result()
	if err != nil {
		return fmt.Errorf("zrange: %w", err)
	}

	for _, key := range vals {
		_ = r.cli.ZRem(ctx, messagePrefix, key).Err()
		_ = r.cli.Del(ctx, key).Err()
	}

	return nil
}
