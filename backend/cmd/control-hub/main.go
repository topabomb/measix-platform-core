package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sync/errgroup"
	"measix/platform/internal/common/server"
	"measix/platform/internal/hub/app"
	"measix/platform/internal/hub/config"
	"measix/platform/internal/hub/identity"
	"measix/platform/internal/hub/maintenance"
	"measix/platform/internal/hub/security"
	"measix/platform/internal/hub/store"
	"measix/platform/pkg/platformid"
)

var buildVersion = "dev"

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	command := "run"
	args := os.Args[1:]
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		command, args = args[0], args[1:]
	}
	var err error
	switch command {
	case "run":
		err = run(args, log)
	case "bootstrap-admin":
		err = bootstrapAdmin(args)
	case "backup":
		err = backup(args)
	case "check":
		err = check(args)
	default:
		err = fmt.Errorf("unknown command %q", command)
	}
	if err != nil {
		log.Error("control-hub command failed", "command", command, "error", err)
		os.Exit(1)
	}
}

func run(args []string, log *slog.Logger) error {
	cfg, err := config.Load(args)
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()
	runtime, err := app.OpenRuntime(ctx, app.RuntimeOptions{Config: cfg, BuildVersion: buildVersion})
	if err != nil {
		return err
	}
	defer runtime.Close()
	group, runCtx := errgroup.WithContext(ctx)
	group.Go(func() error { return server.New(cfg.ListenAddr, runtime.Handler).Run(runCtx, log) })
	group.Go(func() error { return server.New(cfg.InternalListenAddr, runtime.InternalHandler).Run(runCtx, log) })
	group.Go(func() error {
		err := runtime.RunReconciler(runCtx)
		if err == context.Canceled {
			return nil
		}
		return err
	})
	return group.Wait()
}

func bootstrapAdmin(args []string) error {
	fs := flag.NewFlagSet("bootstrap-admin", flag.ContinueOnError)
	dbPath := fs.String("db", os.Getenv("HUB_DB_PATH"), "SQLite database path")
	masterKeyFile := fs.String("master-key-file", os.Getenv("HUB_MASTER_KEY_FILE"), "master key file")
	jwtKeyFile := fs.String("jwt-private-key-file", os.Getenv("HUB_JWT_PRIVATE_KEY_FILE"), "Ed25519 private key file")
	deploymentName := fs.String("deployment-name", "MEASIX", "deployment name")
	username := fs.String("username", "admin", "admin username")
	displayName := fs.String("display-name", "Administrator", "admin display name")
	addAdmin := fs.Bool("add-admin", false, "add an administrator to an existing deployment")
	passwordFile := fs.String("password-file", "", "read password from a protected file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dbPath == "" || *masterKeyFile == "" || *jwtKeyFile == "" {
		return fmt.Errorf("db, master key and JWT key files are required")
	}
	masterKey, err := security.LoadMasterKey(*masterKeyFile)
	if err != nil {
		return err
	}
	privateKey, err := security.LoadEd25519PrivateKey(*jwtKeyFile)
	if err != nil {
		return err
	}
	st, err := store.OpenEnt(*dbPath)
	if err != nil {
		return err
	}
	defer st.Close()
	if _, err := maintenance.Check(context.Background(), st.DB); err != nil {
		return err
	}
	password, err := readPassword(*passwordFile)
	if err != nil {
		return err
	}
	deployments, err := st.Client.Deployment.Query().All(context.Background())
	if err != nil {
		return err
	}
	var deploymentID string
	if len(deployments) == 0 {
		deploymentID = platformid.New(platformid.Deployment)
	} else if len(deployments) == 1 && *addAdmin {
		deploymentID = deployments[0].ID
	} else if len(deployments) == 1 {
		return fmt.Errorf("deployment already exists; use --add-admin to add another administrator")
	} else {
		return fmt.Errorf("invalid deployment cardinality: %d", len(deployments))
	}
	signer, err := security.NewAccessSigner(privateKey, deploymentID, "hub-bootstrap", 10*time.Minute)
	if err != nil {
		return err
	}
	csrfMaterial := append([]byte("measix:admin-csrf:"), masterKey...)
	csrf := sha256.Sum256(csrfMaterial)
	service := identity.New(st.Client, signer, csrf[:])
	if len(deployments) == 0 {
		result, err := service.Bootstrap(context.Background(), *deploymentName, *username, *displayName, password)
		if err != nil {
			return err
		}
		fmt.Printf("deployment=%s admin=%s draft=%s\n", result.DeploymentID, result.AdminUserID, result.DraftID)
		return nil
	}
	admin, err := service.CreateUser(context.Background(), *username, *displayName, "ADMIN")
	if err != nil {
		return err
	}
	if err := service.SetPassword(context.Background(), admin.ID, password); err != nil {
		return err
	}
	fmt.Printf("admin=%s\n", admin.ID)
	return nil
}

func backup(args []string) error {
	fs := flag.NewFlagSet("backup", flag.ContinueOnError)
	dbPath := fs.String("db", os.Getenv("HUB_DB_PATH"), "SQLite database path")
	output := fs.String("output", "", "new backup database path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dbPath == "" || *output == "" {
		return fmt.Errorf("db and output are required")
	}
	st, err := store.OpenEnt(*dbPath)
	if err != nil {
		return err
	}
	defer st.Close()
	metadata, err := maintenance.Backup(context.Background(), st.DB, *output, buildVersion, time.Now())
	if err != nil {
		return err
	}
	fmt.Printf("backup=%s metadata=%s\n", *output, metadata)
	return nil
}

func check(args []string) error {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	dbPath := fs.String("db", os.Getenv("HUB_DB_PATH"), "SQLite database path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dbPath == "" {
		return fmt.Errorf("db is required")
	}
	st, err := store.OpenEnt(*dbPath)
	if err != nil {
		return err
	}
	defer st.Close()
	result, err := maintenance.Check(context.Background(), st.DB)
	if err != nil {
		return err
	}
	fmt.Printf("integrity=%s tables=%d\n", result.Integrity, result.Tables)
	return nil
}

func readPassword(path string) (string, error) {
	if path != "" {
		value, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		password := strings.TrimSpace(string(value))
		if password == "" {
			return "", fmt.Errorf("password file is empty")
		}
		return password, nil
	}
	fmt.Fprint(os.Stderr, "Password: ")
	value, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return "", err
	}
	password := strings.TrimSpace(value)
	if password == "" {
		return "", fmt.Errorf("password is empty")
	}
	return password, nil
}
