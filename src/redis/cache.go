package redis

import (
	"context"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

const cacheTTL = 12 * time.Hour

func GetCachedContentURL(ctx context.Context, key string) (string, error) {
	result, err := Client().Get(ctx, key).Result()
	if err != nil {
		if err == goredis.Nil {
			return "", nil
		}

		return "", err
	}

	return result, nil
}

func SetCachedContentURL(ctx context.Context, key, value string) error {
	return Client().SetEx(ctx, key, value, cacheTTL).Err()
}
