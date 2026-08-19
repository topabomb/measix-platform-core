package config

import (
	"errors"
	"flag"
	"os"
)

type Config struct{ ListenAddr, PublicBaseURL, DBPath string }

func Load(args []string) (Config, error) {
	fs := flag.NewFlagSet("control-hub", flag.ContinueOnError)
	cfg := Config{ListenAddr: env("HUB_LISTEN_ADDR", ":8080"), PublicBaseURL: env("HUB_PUBLIC_BASE_URL", "https://localhost"), DBPath: env("HUB_DB_PATH", "hub.db")}
	fs.StringVar(&cfg.ListenAddr, "listen", cfg.ListenAddr, "public listen address")
	fs.StringVar(&cfg.PublicBaseURL, "public-base-url", cfg.PublicBaseURL, "public platform URL")
	fs.StringVar(&cfg.DBPath, "db", cfg.DBPath, "SQLite database path")
	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}
	if cfg.PublicBaseURL == "" || cfg.DBPath == "" {
		return Config{}, errors.New("missing required hub configuration")
	}
	return cfg, nil
}
func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
