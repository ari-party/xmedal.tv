package utils

import (
	"log"
	"sync"

	coreenv "github.com/caarlos0/env/v10"
)

type Config struct {
	NodeEnv  string `env:"NODE_ENV" envDefault:"development"`
	Port     int    `env:"PORT" envDefault:"3000"`
	RedisURL string `env:"REDIS_URL" envDefault:"redis://localhost:6379"`
}

var (
	cfg  Config
	once sync.Once
)

func LoadConfig() Config {
	once.Do(func() {
		if err := coreenv.Parse(&cfg); err != nil {
			log.Fatalf("failed to load environment: %v", err)
		}
	})

	return cfg
}
