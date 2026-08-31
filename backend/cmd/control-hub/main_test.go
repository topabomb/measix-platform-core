package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"measix/platform/internal/hub/app"
	"measix/platform/internal/hub/config"
	"measix/platform/internal/hub/store"
	"measix/platform/migrations"
)

func TestBootstrapRepeatAndConfiguredStaticHosting(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "hub.db")
	st, err := store.OpenEnt(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.Exec(migrations.SQLAfter("")); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	write := func(name, content string) string {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	master := write("master.key", strings.Repeat("m", 32))
	signing := write("signing.seed", strings.Repeat("s", 32))
	token := write("service.token", "synthetic-service-token")
	password := write("password.txt", "synthetic-admin-password")
	write("index.html", "<html>configured Admin SPA</html>")
	args := []string{"--db", dbPath, "--master-key-file", master, "--jwt-private-key-file", signing, "--password-file", password, "--if-empty", "--timezone", "Asia/Shanghai"}
	if err := bootstrapAdmin(args); err != nil {
		t.Fatal(err)
	}
	// Repeat setup must not read/reset the password, create another admin or alter timezone.
	if err := os.Remove(password); err != nil {
		t.Fatal(err)
	}
	if err := bootstrapAdmin(args); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load([]string{"--db", dbPath, "--master-key-file", master, "--jwt-private-key-file", signing, "--relay-service-token-file", token, "--relay-internal-url", "http://127.0.0.1:1", "--admin-assets-dir", dir})
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := app.OpenRuntime(context.Background(), app.RuntimeOptions{Config: cfg, HTTPClient: &http.Client{Timeout: 20 * time.Millisecond}})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	for _, path := range []string{"/admin/", "/admin/resources"} {
		rr := httptest.NewRecorder()
		runtime.Handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
		if rr.Code != 200 || !strings.Contains(rr.Body.String(), "configured Admin SPA") {
			t.Fatalf("%s: %d %s", path, rr.Code, rr.Body.String())
		}
	}
	dep, err := runtime.Store.Client.Deployment.Query().Only(context.Background())
	if err != nil || dep.Timezone != "Asia/Shanghai" {
		t.Fatalf("timezone lost: %v %v", dep, err)
	}
	count, err := runtime.Store.Client.User.Query().Count(context.Background())
	if err != nil || count != 1 {
		t.Fatalf("repeat created users: %d %v", count, err)
	}
	cfg.AdminAssetsDir = filepath.Join(dir, "missing")
	if bad, err := app.OpenRuntime(context.Background(), app.RuntimeOptions{Config: cfg}); err == nil {
		bad.Close()
		t.Fatal("missing SPA accepted")
	}
}
