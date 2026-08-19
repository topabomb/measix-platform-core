package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/topabomb/measix-platform-core/backend/internal/common/server"
	"github.com/topabomb/measix-platform-core/backend/internal/relay/app"
	"github.com/topabomb/measix-platform-core/backend/internal/relay/config"
	"github.com/topabomb/measix-platform-core/backend/internal/relay/metering"
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
	spool, err := metering.OpenSpool(cfg.SpoolPath)
	if err != nil {
		log.Error("usage spool unavailable", "error", err)
		os.Exit(2)
	}
	defer spool.Close()
	recorder := metering.NewRecorder(spool)
	sender := metering.NewSender(spool, cfg.HubUsageURL, serviceToken)
	sender.BatchSize = cfg.UsageBatchSize
	a := app.NewWithMetering(serviceToken, spool, recorder)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()
	g, runCtx := errgroup.WithContext(ctx)
	g.Go(func() error { return server.NewWithGrace(cfg.PublicListenAddr, a.Public, cfg.ShutdownGrace).Run(runCtx, log) })
	g.Go(func() error { return server.NewWithGrace(cfg.InternalListenAddr, a.Internal, cfg.ShutdownGrace).Run(runCtx, log) })
	g.Go(func() error {
		err := sender.Run(runCtx, cfg.UsageFlushInterval)
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return err
	})
	if err := g.Wait(); err != nil {
		log.Error("runtime relay stopped", "error", err)
		os.Exit(1)
	}
	flushCtx, flushCancel := context.WithTimeout(context.Background(), minDuration(cfg.ShutdownGrace, 2*time.Second))
	defer flushCancel()
	if err := sender.FlushOnce(flushCtx); err != nil {
		log.Warn("final usage flush incomplete; durable spool retained", "error", err)
	}
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
