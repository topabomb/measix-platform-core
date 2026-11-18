// This file is overlaid into the fixed consumer's package, never copied into it.
package tuck

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type currentCoreCapture struct {
	ID             string `json:"id"`
	Method         string `json:"method"`
	Path           string `json:"path"`
	AgentSpaceID   string `json:"agentSpaceId"`
	Directory      string `json:"directory"`
	Status         int    `json:"status"`
	ContentType    string `json:"contentType"`
	BodyFile       string `json:"bodyFile"`
	ResponseSHA256 string `json:"responseSHA256"`
	ByteLength     int    `json:"byteLength"`
	raw            []byte
	value          M
	request        *url.URL
}

func currentCoreCaptures(t *testing.T) map[string]*currentCoreCapture {
	t.Helper()
	manifest := os.Getenv("MEASIX_TUCK_COMPAT_MANIFEST")
	if manifest == "" {
		t.Fatal("Pass the real Client capture manifest through the Core compatibility runner")
	}
	raw, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	var input struct {
		SchemaVersion int                  `json:"schemaVersion"`
		Cases         []currentCoreCapture `json:"cases"`
	}
	if err := json.Unmarshal(raw, &input); err != nil || input.SchemaVersion != 1 || len(input.Cases) != 3 {
		t.Fatal("Invalid capture manifest", err)
	}
	out := map[string]*currentCoreCapture{}
	for i := range input.Cases {
		capture := &input.Cases[i]
		if capture.ID != "workspace" && capture.ID != "files-root" && capture.ID != "files-nested" || out[capture.ID] != nil || capture.Status != 200 || capture.Method != "GET" || capture.ContentType != "application/json" || capture.BodyFile != capture.ID+".json" {
			t.Fatal("Invalid capture receipt", capture.ID)
		}
		capture.raw, err = os.ReadFile(filepath.Join(filepath.Dir(manifest), capture.BodyFile))
		if err != nil || len(capture.raw) != capture.ByteLength || len(capture.raw) == 0 || len(capture.raw) > 4<<20 || fmt.Sprintf("sha256:%x", sha256.Sum256(capture.raw)) != capture.ResponseSHA256 {
			t.Fatal("Capture bytes differ from receipt", capture.ID, err)
		}
		capture.request, err = url.ParseRequestURI(capture.Path)
		if err != nil {
			t.Fatal(err)
		}
		schema, endpoint := "WorkspaceFileList", "/api/client/v1/workspace/files"
		if capture.ID == "workspace" {
			schema, endpoint = "WorkspaceProjection", "/api/client/v1/workspace"
		}
		if capture.request.Path != endpoint {
			t.Fatal("Not a Client capture", capture.ID)
		}
		// Both parsing and schema validation are the fixed consumer's real code.
		capture.value, err = responseJSON(&http.Response{StatusCode: capture.Status, Body: io.NopCloser(bytes.NewReader(capture.raw))}, schema)
		if err != nil {
			t.Fatal("Fixed consumer rejected current capture", capture.ID, err)
		}
		if capture.ID != "workspace" && (len(A(capture.value["entries"])) == 0 || capture.request.Query().Get("path") != capture.Directory || capture.request.Query().Get("agentSpaceId") != capture.AgentSpaceID || (capture.Directory == "") != (capture.ID == "files-root")) {
			t.Fatal("Nonempty root and nested directory captures are required", capture.ID)
		}
		out[capture.ID] = capture
	}
	space := S(out["workspace"].value["agentSpaceId"])
	for _, capture := range out {
		if space == "" || capture.AgentSpaceID != space {
			t.Fatal("Capture space differs from the real Client projection", capture.ID)
		}
	}
	return out
}

func currentCoreReplayFixture(t *testing.T, captures map[string]*currentCoreCapture, replacement map[string][]byte) (*workspaceFixture, string) {
	t.Helper()
	f := newWorkspaceFixture(t)
	f.space = S(captures["workspace"].value["agentSpaceId"])
	f.setDAVHook(func(w http.ResponseWriter, r *http.Request, _ string) bool {
		if r.Header.Get("Authorization") != "Bearer synthetic.access.token" {
			t.Error("Fixed consumer did not use its existing authenticated workspace transport")
			w.WriteHeader(401)
			return true
		}
		for _, capture := range captures {
			if r.Method == capture.Method && r.URL.Path == capture.request.Path && r.URL.Query().Encode() == capture.request.Query().Encode() {
				body := capture.raw
				if alternate, ok := replacement[capture.ID]; ok {
					body = alternate
				}
				w.Header().Set("Content-Type", capture.ContentType)
				w.WriteHeader(capture.Status)
				_, _ = w.Write(body)
				return true
			}
		}
		t.Errorf("Unexpected fixed consumer request: %s %s", r.Method, r.URL.RequestURI())
		w.WriteHeader(404)
		return true
	})
	_, proxy, query := f.proxy(t)
	return f, proxy.URL + "/api/enterprise-workspace/files?" + query
}

func TestCurrentCoreWorkspaceCaptureReplay(t *testing.T) {
	captures := currentCoreCaptures(t)
	f, filesURL := currentCoreReplayFixture(t, captures, nil)
	for _, id := range []string{"workspace", "files-root", "files-nested"} {
		t.Run(id, func(t *testing.T) {
			capture := captures[id]
			target, schema := filesURL+"&path="+url.QueryEscape(capture.Directory), "WorkspaceFileList"
			if id == "workspace" {
				u, _ := url.Parse(filesURL)
				u.Path, u.RawQuery = "/api/enterprise-workspace", ""
				target, schema = u.String(), "WorkspaceProjection"
			}
			response, err := http.Get(target)
			if err != nil {
				t.Fatal(err)
			}
			status := response.StatusCode
			value, err := responseJSON(response, schema)
			if err != nil || status != 200 {
				t.Fatal("EnterpriseWorkspace.Handle rejected capture", id, status, err)
			}
			if id == "workspace" {
				if value["subject"] != workspaceSubject(f.p.platform.Binding()) {
					t.Fatal("Existing local subject projection was not applied")
				}
				delete(value, "subject")
				delete(value, "employeeName")
			}
			if !reflect.DeepEqual(value, capture.value) {
				t.Fatal("Handle changed original Client data", id)
			}
			t.Logf("Raw Client %s accepted through parser/schema and Handle, %s, %d bytes", id, capture.ResponseSHA256, capture.ByteLength)
		})
	}
}

func TestCurrentCoreWorkspacePathGuards(t *testing.T) {
	captures := currentCoreCaptures(t)
	// Deliberately corrupt a separate in-memory negative case. Positive replays
	// above always deliver the original captured bytes without reserialization.
	bad := Clone(captures["files-nested"].value)
	O(A(bad["entries"])[0])["path"] = "outside.txt"
	f, filesURL := currentCoreReplayFixture(t, captures, map[string][]byte{"files-nested": JSON(bad)})
	response, err := http.Get(filesURL + "&path=" + url.QueryEscape(captures["files-nested"].Directory))
	if err != nil {
		t.Fatal(err)
	}
	status := response.StatusCode
	problem, err := responseJSON(response, "")
	if err != nil || status != 422 || problem["code"] != "invalid_platform_data" {
		t.Fatal("Real parent-directory validation did not reject foreign entry", status, err)
	}
	f.mu.Lock()
	before := len(f.calls)
	f.mu.Unlock()
	response, err = http.Get(filesURL + "&path=" + url.QueryEscape("../escape"))
	if err != nil {
		t.Fatal(err)
	}
	status = response.StatusCode
	problem, err = responseJSON(response, "")
	f.mu.Lock()
	after := len(f.calls)
	f.mu.Unlock()
	if err != nil || status != 400 || problem["code"] != "invalid_workspace_path" || after != before {
		t.Fatal("Real path guard did not reject traversal before upstream access", status, err)
	}
}
