// Command device-demo runs the local real-device composition on one public
// origin. It is a development helper: Hub and Relay retain their normal
// private control connection, while this process dispatches the public
// /runtime/v1 path directly to Relay instead of requiring an external ingress.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sync/errgroup"
	"measix/platform/internal/common/server"
	hubapp "measix/platform/internal/hub/app"
	"measix/platform/internal/hub/config"
	relayapp "measix/platform/internal/relay/app"
	"measix/platform/internal/relay/metering"
)

var buildVersion = "dev"

type options struct {
	listen, hubInternalListen, relayInternalListen string
	db, masterKey, jwtKey, relayToken, spool       string
	publicOrigin, adminAssets, portalAssets        string
}

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(os.Args[1:], log); err != nil {
		log.Error("device demo stopped", "error", err)
		os.Exit(1)
	}
}

func run(args []string, log *slog.Logger) error {
	var opt options
	fs := flag.NewFlagSet("device-demo", flag.ContinueOnError)
	fs.StringVar(&opt.listen, "listen", "", "public Hub and Relay listen address")
	fs.StringVar(&opt.hubInternalListen, "hub-internal-listen", "", "private Hub usage ingest listen address")
	fs.StringVar(&opt.relayInternalListen, "relay-internal-listen", "", "private Relay control listen address")
	fs.StringVar(&opt.db, "db", "", "Hub SQLite database")
	fs.StringVar(&opt.masterKey, "master-key-file", "", "Hub AES-256 master key")
	fs.StringVar(&opt.jwtKey, "jwt-private-key-file", "", "Hub Ed25519 private key")
	fs.StringVar(&opt.relayToken, "relay-service-token-file", "", "Hub and Relay private service credential")
	fs.StringVar(&opt.spool, "spool", "", "Relay durable usage spool")
	fs.StringVar(&opt.publicOrigin, "public-origin", "", "device-reachable platform origin")
	fs.StringVar(&opt.adminAssets, "admin-assets-dir", "", "built Admin SPA directory")
	fs.StringVar(&opt.portalAssets, "portal-assets-dir", "", "built Portal SPA directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if opt.listen == "" || opt.hubInternalListen == "" || opt.relayInternalListen == "" || opt.db == "" || opt.masterKey == "" || opt.jwtKey == "" || opt.relayToken == "" || opt.spool == "" || opt.publicOrigin == "" {
		return errors.New("all device-demo addresses, storage paths, credentials and public origin are required")
	}

	serviceToken, err := readToken(opt.relayToken)
	if err != nil {
		return err
	}
	spool, err := metering.OpenSpool(opt.spool)
	if err != nil {
		return err
	}
	defer spool.Close()
	recorder := metering.NewRecorder(spool)
	recorder.Log = log
	relay := relayapp.New(serviceToken, buildVersion, spool, recorder)

	hubCfg := config.Config{
		PublicOrigin: opt.publicOrigin, ListenAddr: opt.listen, InternalListenAddr: opt.hubInternalListen,
		DBPath: opt.db, MasterKeyFile: opt.masterKey, JWTPrivateKeyFile: opt.jwtKey,
		RelayInternalURL: "http://" + opt.relayInternalListen, RelayServiceTokenFile: opt.relayToken,
		AdminAssetsDir: opt.adminAssets, PortalAssetsDir: opt.portalAssets,
		AccessTokenTTL: 10 * time.Minute, ReconcileInterval: 2 * time.Second,
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()
	hub, err := hubapp.OpenRuntime(ctx, hubapp.RuntimeOptions{Config: hubCfg, BuildVersion: buildVersion})
	if err != nil {
		return err
	}
	defer hub.Close()

	public := publicHandler(hub.Handler, relay.Public)
	sender := metering.NewSender(spool, "http://"+opt.hubInternalListen+"/internal/v1/usage/request-events:batch", serviceToken)
	group, runCtx := errgroup.WithContext(ctx)
	group.Go(func() error { return server.New(opt.listen, public).Run(runCtx, log) })
	group.Go(func() error { return server.New(opt.hubInternalListen, hub.InternalHandler).Run(runCtx, log) })
	group.Go(func() error {
		return server.NewWithGrace(opt.relayInternalListen, relay.Internal, 30*time.Second).Run(runCtx, log)
	})
	group.Go(func() error {
		err := hub.RunReconciler(runCtx)
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return err
	})
	group.Go(func() error {
		err := sender.Run(runCtx, time.Second)
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return err
	})
	runErr := group.Wait()
	flushCtx, flushCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer flushCancel()
	if err := sender.FlushOnce(flushCtx); err != nil {
		log.Warn("final usage flush incomplete; durable spool retained", "error", err)
	}
	return runErr
}

func publicHandler(hub, relay http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/runtime/v1" || strings.HasPrefix(r.URL.Path, "/runtime/v1/") {
			relay.ServeHTTP(w, r)
			return
		}
		hub.ServeHTTP(w, r)
	})
}

func readToken(path string) (string, error) {
	value, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read relay service credential: %w", err)
	}
	token := strings.TrimSpace(string(value))
	if token == "" {
		return "", errors.New("relay service credential is empty")
	}
	return token, nil
}
