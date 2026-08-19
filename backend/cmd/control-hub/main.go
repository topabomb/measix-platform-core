package main

import (
	"context"
	"github.com/topabomb/measix-platform-core/backend/internal/common/server"
	"github.com/topabomb/measix-platform-core/backend/internal/hub/app"
	"github.com/topabomb/measix-platform-core/backend/internal/hub/config"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load(os.Args[1:])
	if err != nil {
		log.Error("configuration invalid", "code", "invalid_configuration", "error", err)
		os.Exit(2)
	}
	a := app.New(cfg)
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()
	log.Info("control hub starting", "listenAddr", cfg.ListenAddr)
	if err := server.New(cfg.ListenAddr, a.Router).Run(ctx, log); err != nil {
		log.Error("control hub stopped", "error", err)
		os.Exit(1)
	}
}
