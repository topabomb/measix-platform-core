// Isolated acceptance adapter. Management responses are deterministic fixtures;
// file operations use the independent x/net/webdav server and a real directory.
// This is not an Agent Space implementation or a deployable product component.
package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"golang.org/x/net/webdav"
)

type user struct {
	Space, Token, DAV string
	Enabled           bool
}
type adapter struct {
	mu               sync.Mutex
	root, management string
	users            map[string]*user
	dav              map[string]*webdav.Handler
}

var namePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

func token() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}
func id() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	b[6] = b[6]&15 | 64
	b[8] = b[8]&63 | 128
	return fmt.Sprintf("spc_%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}
func (a *adapter) save() error {
	data, err := json.Marshal(a.users)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(a.root, "users.private.json"), data, 0600)
}
func (a *adapter) handler(name string) *webdav.Handler {
	if h := a.dav[name]; h != nil {
		return h
	}
	dir := filepath.Join(a.root, "files", name)
	if err := os.MkdirAll(dir, 0700); err != nil {
		panic(err)
	}
	h := &webdav.Handler{Prefix: "/u/" + name + "/dav", FileSystem: webdav.Dir(dir), LockSystem: webdav.NewMemLS()}
	a.dav[name] = h
	return h
}
func (a *adapter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/live" {
		w.WriteHeader(200)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/u/") {
		parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/u/"), "/", 2)
		if len(parts) != 2 || !namePattern.MatchString(parts[0]) {
			http.NotFound(w, r)
			return
		}
		a.mu.Lock()
		u := a.users[parts[0]]
		authorized := u != nil && u.Enabled && u.DAV != "" && r.Header.Get("Authorization") == "Bearer "+u.DAV
		var h *webdav.Handler
		if authorized {
			h = a.handler(parts[0])
		}
		a.mu.Unlock()
		if !authorized {
			w.Header().Set("WWW-Authenticate", `Bearer realm="fixture"`)
			w.WriteHeader(401)
			return
		}
		if !strings.HasPrefix(parts[1], "dav") {
			http.NotFound(w, r)
			return
		}
		h.ServeHTTP(w, r)
		return
	}
	if r.Header.Get("Authorization") != "Bearer "+a.management {
		w.WriteHeader(401)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if r.URL.Path == "/v1/chat/completions" {
		_, _ = io.Copy(io.Discard, r.Body)
		_, _ = w.Write([]byte(`{"id":"fixture","object":"chat.completion","model":"fixture","choices":[{"index":0,"message":{"role":"assistant","content":"acceptance"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`))
		return
	}
	if r.URL.Path == "/admin/v1/status" {
		_, _ = w.Write([]byte(`{"ready":true}`))
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	var input struct {
		Username, AgentSpaceID, Token string
		Enabled                       bool
	}
	if r.Body != nil && (r.Method == "POST" || r.Method == "PATCH") {
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&input) != nil {
			w.WriteHeader(400)
			return
		}
	}
	name := strings.TrimPrefix(r.URL.Path, "/admin/v1/users/")
	action := ""
	if p := strings.IndexByte(name, '/'); p >= 0 {
		name, action = name[:p], name[p+1:]
	}
	if r.URL.Path == "/admin/v1/users" && r.Method == "POST" {
		name = input.Username
		if !namePattern.MatchString(name) {
			w.WriteHeader(400)
			return
		}
		if a.users[name] != nil {
			w.WriteHeader(409)
			return
		}
		a.users[name] = &user{Space: id(), Token: token(), Enabled: true}
		a.handler(name)
	}
	u := a.users[name]
	if u == nil {
		w.WriteHeader(404)
		_, _ = w.Write([]byte(`{"error":"not_found"}`))
		return
	}
	if expected := r.URL.Query().Get("agentSpaceId"); expected != "" && expected != u.Space {
		w.WriteHeader(409)
		return
	}
	if input.AgentSpaceID != "" && input.AgentSpaceID != u.Space {
		w.WriteHeader(409)
		return
	}
	credential := u.Token
	switch {
	case action == "dav-token" && r.Method == "POST":
		u.DAV = input.Token
		credential = u.DAV
	case action == "dav-token" && r.Method == "DELETE":
		u.DAV = ""
	case action == "token" && r.Method == "POST":
		u.Token = token()
		credential = u.Token
	case action == "" && r.Method == "PATCH":
		u.Enabled = input.Enabled
		if !u.Enabled {
			u.DAV = ""
			u.Token = ""
		}
	case action == "" && r.Method == "DELETE":
		delete(a.users, name)
	}
	if err := a.save(); err != nil {
		w.WriteHeader(500)
		return
	}
	if r.Method == "DELETE" {
		w.WriteHeader(204)
		return
	}
	status := "disabled"
	if u.Enabled {
		status = "active"
	}
	origin := "http://" + r.Host
	out := map[string]any{"username": name, "agentSpaceId": u.Space, "mcpUrl": origin + "/u/" + name + "/mcp", "davUrl": origin + "/u/" + name + "/dav/", "status": status, "hasCredential": u.Token != "", "hasDavCredential": u.DAV != "", "stopPending": false}
	if r.Method == "POST" {
		out["token"] = credential
		out["keyId"] = "test-key"
	}
	_ = json.NewEncoder(w).Encode(out)
}
func main() {
	listen := flag.String("listen", "127.0.0.1:0", "isolated listen address")
	root := flag.String("root", "", "new isolated evidence directory")
	secret := flag.String("token-file", "", "test management token file")
	flag.Parse()
	if *root == "" || *secret == "" {
		log.Fatal("root and token-file required")
	}
	raw, err := os.ReadFile(*secret)
	if err != nil {
		log.Fatal(err)
	}
	if err = os.MkdirAll(*root, 0700); err != nil {
		log.Fatal(err)
	}
	a := &adapter{root: *root, management: strings.TrimSpace(string(raw)), users: map[string]*user{}, dav: map[string]*webdav.Handler{}}
	if raw, err = os.ReadFile(filepath.Join(*root, "users.private.json")); err == nil {
		if err = json.Unmarshal(raw, &a.users); err != nil {
			log.Fatal(err)
		}
	} else if !os.IsNotExist(err) {
		log.Fatal(err)
	}
	l, err := net.Listen("tcp", *listen)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("http://" + l.Addr().String())
	log.Fatal(http.Serve(l, a))
}
