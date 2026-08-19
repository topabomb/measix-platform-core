package main

import (
	"context"
	"github.com/topabomb/measix-platform-core/backend/internal/common/server"
	"github.com/topabomb/measix-platform-core/backend/internal/relay/app"
	"github.com/topabomb/measix-platform-core/backend/internal/relay/config"
	"golang.org/x/sync/errgroup"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load(os.Args[1:])
	if err != nil {
		log.Error("configuration invalid", "error", err)
		os.Exit(2)
	}
	a := app.New()
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
