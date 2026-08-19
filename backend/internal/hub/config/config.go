package config

import (
	"errors"
	"flag"
	"net/url"
	"os"
	"time"
)

type Config struct {
	ListenAddr            string
	PublicBaseURL         string
	RuntimeAPIBase        string
	DBPath                string
	MasterKeyFile         string
	JWTPrivateKeyFile     string
	RelayInternalURL      string
	RelayServiceTokenFile string
	AccessTokenTTL        time.Duration
	RefreshTokenTTL       time.Duration
	ReconcileInterval     time.Duration
}

func Load(args []string) (Config, error) {
	fs := flag.NewFlagSet("control-hub", flag.ContinueOnError)
	accessTTL, err := envDuration("HUB_ACCESS_TOKEN_TTL", 10*time.Minute)
	if err != nil {
		return Config{}, err
	}
	refreshTTL, err := envDuration("HUB_REFRESH_TOKEN_TTL", 30*24*time.Hour)
	if err != nil {
		return Config{}, err
	}
	reconcileInterval, err := envDuration("HUB_RECONCILE_INTERVAL", 10*time.Second)
	if err != nil {
		return Config{}, err
	}
	cfg := Config{
		ListenAddr:            env("HUB_LISTEN_ADDR", ":8080"),
		PublicBaseURL:         env("HUB_PUBLIC_BASE_URL", ""),
		RuntimeAPIBase:        env("HUB_RUNTIME_API_BASE", ""),
		DBPath:                env("HUB_DB_PATH", ""),
		MasterKeyFile:         env("HUB_MASTER_KEY_FILE", ""),
		JWTPrivateKeyFile:     env("HUB_JWT_PRIVATE_KEY_FILE", ""),
		RelayInternalURL:      env("RELAY_INTERNAL_URL", ""),
		RelayServiceTokenFile: env("HUB_RELAY_SERVICE_TOKEN_FILE", ""),
		AccessTokenTTL:        accessTTL,
		RefreshTokenTTL:       refreshTTL,
		ReconcileInterval:     reconcileInterval,
	}
	fs.StringVar(&cfg.ListenAddr, "listen", cfg.ListenAddr, "public listen address")
	fs.StringVar(&cfg.PublicBaseURL, "public-base-url", cfg.PublicBaseURL, "public platform URL")
	fs.StringVar(&cfg.RuntimeAPIBase, "runtime-api-base", cfg.RuntimeAPIBase, "Runtime Relay public base URL")
	fs.StringVar(&cfg.DBPath, "db", cfg.DBPath, "SQLite database path")
	fs.StringVar(&cfg.MasterKeyFile, "master-key-file", cfg.MasterKeyFile, "AES-256 master key file")
	fs.StringVar(&cfg.JWTPrivateKeyFile, "jwt-private-key-file", cfg.JWTPrivateKeyFile, "Ed25519 private key file")
	fs.StringVar(&cfg.RelayInternalURL, "relay-internal-url", cfg.RelayInternalURL, "Runtime Relay internal URL")
	fs.StringVar(&cfg.RelayServiceTokenFile, "relay-service-token-file", cfg.RelayServiceTokenFile, "Hub-to-Relay service credential file")
	fs.DurationVar(&cfg.AccessTokenTTL, "access-token-ttl", cfg.AccessTokenTTL, "access token TTL")
	fs.DurationVar(&cfg.RefreshTokenTTL, "refresh-token-ttl", cfg.RefreshTokenTTL, "refresh credential TTL")
	fs.DurationVar(&cfg.ReconcileInterval, "reconcile-interval", cfg.ReconcileInterval, "Relay reconcile interval")
	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}
	if cfg.PublicBaseURL == "" || cfg.RuntimeAPIBase == "" || cfg.DBPath == "" || cfg.MasterKeyFile == "" || cfg.JWTPrivateKeyFile == "" || cfg.RelayInternalURL == "" || cfg.RelayServiceTokenFile == "" {
		return Config{}, errors.New("missing required hub configuration")
	}
	for _, raw := range []string{cfg.PublicBaseURL, cfg.RuntimeAPIBase, cfg.RelayInternalURL} {
		parsed, err := url.Parse(raw)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return Config{}, errors.New("invalid hub URL configuration")
		}
	}
	if cfg.AccessTokenTTL <= 0 || cfg.AccessTokenTTL > 10*time.Minute || cfg.RefreshTokenTTL <= 0 || cfg.ReconcileInterval <= 0 {
		return Config{}, errors.New("invalid hub TTL/reconcile configuration")
	}
	return cfg, nil
}

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func envDuration(key string, fallback time.Duration) (time.Duration, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}
	return time.ParseDuration(value)
}
