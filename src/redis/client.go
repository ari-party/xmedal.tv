package redis

import (
	"context"
	"sync"

	goredis "github.com/redis/go-redis/v9"

	"xmedaltv/src/utils"
)

var (
	client *goredis.Client
	once   sync.Once
)

func Client() *goredis.Client {
	once.Do(func() {
		cfg := utils.LoadConfig()
		log := utils.Logger()

		opts, err := goredis.ParseURL(cfg.RedisURL)
		if err != nil {
			log.Error("failed to parse redis url", "error", err)
			panic(err)
		}

		client = goredis.NewClient(opts)

		if err := client.Ping(context.Background()).Err(); err != nil {
			log.Error("unable to connect to redis", "error", err)
		} else {
			log.Info("connected to redis")
		}
	})

	return client
}

func Ping(ctx context.Context) error {
	return Client().Ping(ctx).Err()
}
