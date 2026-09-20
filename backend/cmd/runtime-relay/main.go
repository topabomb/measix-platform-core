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

	"golang.org/x/sync/errgroup"
	"measix/platform/internal/common/server"
	"measix/platform/internal/relay/app"
	relaybudget "measix/platform/internal/relay/budget"
	"measix/platform/internal/relay/config"
	"measix/platform/internal/relay/metering"
)

var buildVersion = "dev"

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(os.Args[1:], log); err != nil {
		log.Error("runtime relay stopped", "error", err)
		os.Exit(1)
	}
}

func run(args []string, log *slog.Logger) error {
	cfg, err := config.Load(args)
	if err != nil {
		return err
	}
	credential, err := os.ReadFile(cfg.HubServiceTokenFile)
	if err != nil {
		return err
	}
	serviceToken := strings.TrimSpace(string(credential))
	if serviceToken == "" {
		return errors.New("empty service credential")
	}
	spool, err := metering.OpenSpool(cfg.SpoolPath)
	if err != nil {
		return err
	}
	defer spool.Close()
	recorder := metering.NewRecorder(spool)
	recorder.Log = log
	sender := metering.NewSender(spool, strings.TrimRight(cfg.HubInternalURL, "/")+"/internal/v1/usage/settlements:batch", serviceToken)
	sender.BatchSize = cfg.UsageBatchSize
	budgetClient, err := relaybudget.NewHTTPClient(cfg.HubInternalURL, serviceToken, nil)
	if err != nil {
		return err
	}
	recoveryCtx, recoveryCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer recoveryCancel()
	if err := metering.RecoverLifecycle(recoveryCtx, spool, budgetClient, recorder); err != nil {
		return err
	}
	if err := sender.FlushOnce(recoveryCtx); err != nil {
		return err
	}
	a := app.New(serviceToken, buildVersion, spool, recorder, budgetClient)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()
	g, runCtx := errgroup.WithContext(ctx)
	g.Go(func() error {
		return server.NewWithGrace(cfg.PublicListenAddr, a.Public, cfg.ShutdownGrace).Run(runCtx, log)
	})
	g.Go(func() error {
		return server.NewWithGrace(cfg.InternalListenAddr, a.Internal, cfg.ShutdownGrace).Run(runCtx, log)
	})
	g.Go(func() error {
		err := sender.Run(runCtx, cfg.UsageFlushInterval)
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return err
	})
	runErr := g.Wait()
	flushCtx, flushCancel := context.WithTimeout(context.Background(), minDuration(cfg.ShutdownGrace, 2*time.Second))
	defer flushCancel()
	if err := sender.FlushOnce(flushCtx); err != nil {
		log.Warn("final usage flush incomplete; durable spool retained", "error", err)
	}
	return runErr
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
