package config

import (
	"errors"
	"flag"
	"os"
)

type Config struct {
	PublicListenAddr   string
	InternalListenAddr string
	SpoolPath          string
	HubServiceTokenFile string
}

func Load(args []string) (Config, error) {
	fs := flag.NewFlagSet("runtime-relay", flag.ContinueOnError)
	cfg := Config{
		PublicListenAddr:    env("RELAY_PUBLIC_LISTEN_ADDR", ":8090"),
		InternalListenAddr:  env("RELAY_INTERNAL_LISTEN_ADDR", "127.0.0.1:8091"),
		SpoolPath:           env("RELAY_SPOOL_PATH", "relay-spool.db"),
		HubServiceTokenFile: env("RELAY_HUB_SERVICE_TOKEN_FILE", ""),
	}
	fs.StringVar(&cfg.PublicListenAddr, "public-listen", cfg.PublicListenAddr, "public listen")
	fs.StringVar(&cfg.InternalListenAddr, "internal-listen", cfg.InternalListenAddr, "internal listen")
	fs.StringVar(&cfg.SpoolPath, "spool", cfg.SpoolPath, "usage spool path")
	fs.StringVar(&cfg.HubServiceTokenFile, "hub-service-token-file", cfg.HubServiceTokenFile, "Hub internal service token file")
	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}
	if cfg.PublicListenAddr == cfg.InternalListenAddr {
		return Config{}, errors.New("public and internal listeners must differ")
	}
	if cfg.SpoolPath == "" || cfg.HubServiceTokenFile == "" {
		return Config{}, errors.New("missing required relay configuration")
	}
	return cfg, nil
}

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
