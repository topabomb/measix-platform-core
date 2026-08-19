package app

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"measix/platform/internal/common/health"
	"measix/platform/internal/hub/adminstatic"
	"measix/platform/internal/hub/capability"
	"measix/platform/internal/hub/config"
	"measix/platform/internal/hub/httpapi"
	"measix/platform/internal/hub/identity"
	"measix/platform/internal/hub/maintenance"
	"measix/platform/internal/hub/runtimecontrol"
	"measix/platform/internal/hub/security"
	"measix/platform/internal/hub/store"
	"measix/platform/internal/hub/system"
	"measix/platform/internal/hub/upstream"
	"measix/platform/internal/hub/usage"
	"measix/platform/internal/wire/usageingestapi"
)

type Runtime struct {
	Handler           http.Handler
	Store             *store.Store
	Services          httpapi.Services
	Health            *health.State
	RuntimeControl    *runtimecontrol.Service
	ReconcileInterval time.Duration
}

type RuntimeOptions struct {
	Config       config.Config
	AdminAssets  fs.FS
	BuildVersion string
	HTTPClient   *http.Client
}

func OpenRuntime(ctx context.Context, options RuntimeOptions) (*Runtime, error) {
	cfg := options.Config
	masterKey, err := security.LoadMasterKey(cfg.MasterKeyFile)
	if err != nil {
		return nil, fmt.Errorf("load master key: %w", err)
	}
	privateKey, err := security.LoadEd25519PrivateKey(cfg.JWTPrivateKeyFile)
	if err != nil {
		return nil, fmt.Errorf("load JWT private key: %w", err)
	}
	serviceCredential, err := readCredentialFile(cfg.RelayServiceTokenFile)
	if err != nil {
		return nil, fmt.Errorf("load Relay service credential: %w", err)
	}
	st, err := store.OpenEnt(cfg.DBPath)
	if err != nil {
		return nil, fmt.Errorf("open Hub database: %w", err)
	}
	closeOnError := func(cause error) (*Runtime, error) {
		_ = st.Close()
		return nil, cause
	}
	if _, err := maintenance.Check(ctx, st.DB); err != nil {
		return closeOnError(fmt.Errorf("database check: %w", err))
	}
	deployments, err := st.Client.Deployment.Query().All(ctx)
	if err != nil {
		return closeOnError(err)
	}
	if len(deployments) != 1 || deployments[0].Status != "ACTIVE" {
		return closeOnError(fmt.Errorf("normal Hub startup requires exactly one ACTIVE Deployment; run bootstrap-admin first"))
	}
	deployment := deployments[0]
	publicKey := privateKey.Public().(ed25519.PublicKey)
	kid := keyID(publicKey)
	signer, err := security.NewAccessSigner(privateKey, deployment.ID, kid, cfg.AccessTokenTTL)
	if err != nil {
		return closeOnError(err)
	}
	csrfMaterial := append([]byte("measix:admin-csrf:"), masterKey...)
	csrfDigest := sha256.Sum256(csrfMaterial)
	identityService := identity.New(st.Client, signer, csrfDigest[:])
	box, err := security.NewSecretBox(masterKey, 1)
	if err != nil {
		return closeOnError(err)
	}
	upstreamService := upstream.NewService(st.Client, box)
	capabilityService := capability.NewService(st.Client)
	client := options.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	relayClient := runtimecontrol.NewHTTPRelayClient(cfg.RelayInternalURL, serviceCredential, client)
	runtimeControl := runtimecontrol.NewService(st.Client, capabilityService, upstreamService, signer, relayClient)
	usageService := usage.NewService(st.Client)
	systemService := system.New(st, runtimeControl, options.BuildVersion)
	services := httpapi.Services{
		Identity: identityService, Capability: capabilityService, Upstream: upstreamService,
		RuntimeControl: runtimeControl, Usage: usageService, System: systemService, BuildVersion: options.BuildVersion,
	}

	router := chi.NewRouter()
	h := &health.State{}
	router.Get("/live", h.Live)
	router.Get("/ready", h.Ready)
	httpapi.RegisterFull(router, services, httpapi.Options{PublicBaseURL: cfg.PublicBaseURL, RuntimeAPIBase: cfg.RuntimeAPIBase})
	usageingestapi.HandlerFromMux(usage.NewHandler(usageService, serviceCredential), router)
	if options.AdminAssets != nil {
		static := adminstatic.New(options.AdminAssets)
		router.Handle("/admin", static)
		router.Handle("/admin/*", static)
	}

	// Relay connectivity is operational state. A failed startup reconcile leaves Runtime DEGRADED,
	// while the fully initialized Admin/diagnostics surface remains available for recovery.
	_, _ = runtimeControl.Reconcile(ctx)
	h.SetReady(true)
	return &Runtime{
		Handler: router, Store: st, Services: services, Health: h,
		RuntimeControl: runtimeControl, ReconcileInterval: cfg.ReconcileInterval,
	}, nil
}

func (r *Runtime) Close() error {
	if r == nil || r.Store == nil {
		return nil
	}
	return r.Store.Close()
}

func (r *Runtime) RunReconciler(ctx context.Context) error {
	if r == nil || r.RuntimeControl == nil || r.ReconcileInterval <= 0 {
		return fmt.Errorf("invalid Hub reconciler configuration")
	}
	ticker := time.NewTicker(r.ReconcileInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			_, _ = r.RuntimeControl.Reconcile(ctx)
		}
	}
}

func readCredentialFile(path string) (string, error) {
	value, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	credential := strings.TrimSpace(string(value))
	if credential == "" {
		return "", fmt.Errorf("credential file is empty")
	}
	return credential, nil
}

func keyID(publicKey []byte) string {
	sum := sha256.Sum256(publicKey)
	return "hub-ed25519-" + hex.EncodeToString(sum[:6])
}
