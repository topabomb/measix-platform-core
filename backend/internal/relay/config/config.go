package config

import (
	"errors"
	"flag"
	"os"
	"strconv"
	"time"
)

type Config struct {
	PublicListenAddr    string
	InternalListenAddr  string
	SpoolPath           string
	HubUsageURL         string
	HubServiceTokenFile string
	UsageBatchSize      int
	UsageFlushInterval  time.Duration
	ShutdownGrace       time.Duration
}

func Load(args []string) (Config, error) {
	fs := flag.NewFlagSet("runtime-relay", flag.ContinueOnError)
	batchSize, err := envInt("RELAY_USAGE_BATCH_SIZE", 100)
	if err != nil {
		return Config{}, err
	}
	flushInterval, err := envDuration("RELAY_USAGE_FLUSH_INTERVAL", time.Second)
	if err != nil {
		return Config{}, err
	}
	shutdownGrace, err := envDuration("RELAY_SHUTDOWN_GRACE", 30*time.Second)
	if err != nil {
		return Config{}, err
	}
	cfg := Config{
		PublicListenAddr:    env("RELAY_PUBLIC_LISTEN_ADDR", ":8090"),
		InternalListenAddr:  env("RELAY_INTERNAL_LISTEN_ADDR", "127.0.0.1:8091"),
		SpoolPath:           env("RELAY_SPOOL_PATH", "relay-spool.db"),
		HubUsageURL:         env("HUB_USAGE_URL", ""),
		HubServiceTokenFile: env("RELAY_HUB_SERVICE_TOKEN_FILE", ""),
		UsageBatchSize:      batchSize,
		UsageFlushInterval:  flushInterval,
		ShutdownGrace:       shutdownGrace,
	}
	fs.StringVar(&cfg.PublicListenAddr, "public-listen", cfg.PublicListenAddr, "public listen")
	fs.StringVar(&cfg.InternalListenAddr, "internal-listen", cfg.InternalListenAddr, "internal listen")
	fs.StringVar(&cfg.SpoolPath, "spool", cfg.SpoolPath, "usage spool path")
	fs.StringVar(&cfg.HubUsageURL, "hub-usage-url", cfg.HubUsageURL, "Hub usage ingest URL")
	fs.StringVar(&cfg.HubServiceTokenFile, "hub-service-token-file", cfg.HubServiceTokenFile, "Hub internal service token file")
	fs.IntVar(&cfg.UsageBatchSize, "usage-batch-size", cfg.UsageBatchSize, "usage batch size")
	fs.DurationVar(&cfg.UsageFlushInterval, "usage-flush-interval", cfg.UsageFlushInterval, "usage flush interval")
	fs.DurationVar(&cfg.ShutdownGrace, "shutdown-grace", cfg.ShutdownGrace, "shutdown grace")
	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}
	if cfg.PublicListenAddr == cfg.InternalListenAddr {
		return Config{}, errors.New("public and internal listeners must differ")
	}
	if cfg.SpoolPath == "" || cfg.HubUsageURL == "" || cfg.HubServiceTokenFile == "" {
		return Config{}, errors.New("missing required relay configuration")
	}
	if cfg.UsageBatchSize < 1 || cfg.UsageBatchSize > 200 || cfg.UsageFlushInterval <= 0 || cfg.ShutdownGrace <= 0 {
		return Config{}, errors.New("invalid relay usage/shutdown configuration")
	}
	return cfg, nil
}

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func envInt(key string, fallback int) (int, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, err
	}
	return parsed, nil
}

func envDuration(key string, fallback time.Duration) (time.Duration, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}
	return time.ParseDuration(value)
}
