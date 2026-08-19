package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/topabomb/measix-platform-core/backend/internal/common/server"
	"github.com/topabomb/measix-platform-core/backend/internal/relay/app"
	"github.com/topabomb/measix-platform-core/backend/internal/relay/config"
	"golang.org/x/sync/errgroup"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load(os.Args[1:])
	if err != nil {
		log.Error("configuration invalid", "error", err)
		os.Exit(2)
	}
	credential, err := os.ReadFile(cfg.HubServiceTokenFile)
	if err != nil {
		log.Error("service credential unavailable", "error", err)
		os.Exit(2)
	}
	serviceToken := strings.TrimSpace(string(credential))
	if serviceToken == "" {
		log.Error("service credential unavailable", "error", "empty credential")
		os.Exit(2)
	}

	a := app.New(serviceToken)
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error { return server.New(cfg.PublicListenAddr, a.Public).Run(ctx, log) })
	g.Go(func() error { return server.New(cfg.InternalListenAddr, a.Internal).Run(ctx, log) })
	if err := g.Wait(); err != nil {
		log.Error("runtime relay stopped", "error", err)
		os.Exit(1)
	}
}
