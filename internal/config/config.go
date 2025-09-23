package config

import (
	"os"
)

type Config struct {
	Token        string
	DatabasePath string
}

func FromEnv() Config {
	cfg := Config{
		Token:        os.Getenv("TELEGRAM_BOT_TOKEN"),
		DatabasePath: os.Getenv("DATABASE_PATH"),
	}
	if cfg.DatabasePath == "" {
		cfg.DatabasePath = "./data/coffeetrix.db"
	}
	return cfg
}
