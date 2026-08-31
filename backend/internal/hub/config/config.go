package config

import (
	"errors"
	"flag"
	"net/url"
	"os"
	"time"
)

type Config struct {
	AdminAssetsDir        string
	ListenAddr            string
	InternalListenAddr    string
	DBPath                string
	MasterKeyFile         string
	JWTPrivateKeyFile     string
	RelayInternalURL      string
	RelayServiceTokenFile string
	AccessTokenTTL        time.Duration
	ReconcileInterval     time.Duration
}

func Load(args []string) (Config, error) {
	fs := flag.NewFlagSet("control-hub", flag.ContinueOnError)
	accessTTL, err := envDuration("HUB_ACCESS_TOKEN_TTL", 10*time.Minute)
	if err != nil {
		return Config{}, err
	}
	reconcileInterval, err := envDuration("HUB_RECONCILE_INTERVAL", 10*time.Second)
	if err != nil {
		return Config{}, err
	}
	cfg := Config{
		AdminAssetsDir:        env("HUB_ADMIN_ASSETS_DIR", ""),
		ListenAddr:            env("HUB_LISTEN_ADDR", ":8080"),
		InternalListenAddr:    env("HUB_INTERNAL_LISTEN_ADDR", "127.0.0.1:8081"),
		DBPath:                env("HUB_DB_PATH", ""),
		MasterKeyFile:         env("HUB_MASTER_KEY_FILE", ""),
		JWTPrivateKeyFile:     env("HUB_JWT_PRIVATE_KEY_FILE", ""),
		RelayInternalURL:      env("RELAY_INTERNAL_URL", ""),
		RelayServiceTokenFile: env("HUB_RELAY_SERVICE_TOKEN_FILE", ""),
		AccessTokenTTL:        accessTTL,
		ReconcileInterval:     reconcileInterval,
	}
	fs.StringVar(&cfg.AdminAssetsDir, "admin-assets-dir", cfg.AdminAssetsDir, "built Admin SPA directory (contains index.html)")
	fs.StringVar(&cfg.ListenAddr, "listen", cfg.ListenAddr, "public listen address")
	fs.StringVar(&cfg.InternalListenAddr, "internal-listen", cfg.InternalListenAddr, "internal (private) listen address for Relay→Hub service APIs")
	fs.StringVar(&cfg.DBPath, "db", cfg.DBPath, "SQLite database path")
	fs.StringVar(&cfg.MasterKeyFile, "master-key-file", cfg.MasterKeyFile, "AES-256 master key file")
	fs.StringVar(&cfg.JWTPrivateKeyFile, "jwt-private-key-file", cfg.JWTPrivateKeyFile, "Ed25519 private key file")
	fs.StringVar(&cfg.RelayInternalURL, "relay-internal-url", cfg.RelayInternalURL, "Runtime Relay internal URL")
	fs.StringVar(&cfg.RelayServiceTokenFile, "relay-service-token-file", cfg.RelayServiceTokenFile, "Hub-to-Relay service credential file")
	fs.DurationVar(&cfg.AccessTokenTTL, "access-token-ttl", cfg.AccessTokenTTL, "access token TTL")
	fs.DurationVar(&cfg.ReconcileInterval, "reconcile-interval", cfg.ReconcileInterval, "Relay reconcile interval")
	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}
	if cfg.DBPath == "" || cfg.MasterKeyFile == "" || cfg.JWTPrivateKeyFile == "" || cfg.RelayInternalURL == "" || cfg.RelayServiceTokenFile == "" {
		return Config{}, errors.New("missing required hub configuration")
	}
	for _, raw := range []string{cfg.RelayInternalURL} {
		parsed, err := url.Parse(raw)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return Config{}, errors.New("invalid hub URL configuration")
		}
	}
	if cfg.AccessTokenTTL <= 0 || cfg.AccessTokenTTL > 10*time.Minute || cfg.ReconcileInterval <= 0 {
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
