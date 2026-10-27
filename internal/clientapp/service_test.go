package clientapp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/xxay-16/yunbo/internal/config"
	"github.com/xxay-16/yunbo/internal/openlist"
)

type uploadFixture struct {
	mu        sync.Mutex
	dirs      map[string]bool
	files     map[string]string
	failedDir string
	writes    int
}

func newUploadFixture(t *testing.T) (*Service, *uploadFixture, string) {
	t.Helper()
	fixture := &uploadFixture{dirs: map[string]bool{"/": true, "/target": true}, files: map[string]string{}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reply := func(code int, data any, message string) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"code": code, "data": data, "message": message})
		}
		switch r.URL.Path {
		case "/api/me":
			reply(200, openlist.User{Username: "tester", Role: 2, Permission: 255}, "success")
		case "/api/fs/list":
			var request struct {
				Path string `json:"path"`
			}
			json.NewDecoder(r.Body).Decode(&request)
			if request.Path == "/missing" {
				reply(403, nil, "denied")
				return
			}
			reply(200, openlist.Listing{Content: []openlist.File{}, Write: true}, "success")
		case "/api/fs/mkdir":
			var request struct {
				Path string `json:"path"`
			}
			json.NewDecoder(r.Body).Decode(&request)
			fixture.mu.Lock()
			defer fixture.mu.Unlock()
			fixture.writes++
			if request.Path == fixture.failedDir || !fixture.dirs[path.Dir(request.Path)] {
				reply(500, nil, "cannot create directory")
				return
			}
			fixture.dirs[request.Path] = true
			reply(200, nil, "success")
		case "/api/fs/put":
			remote, err := url.PathUnescape(r.Header.Get("File-Path"))
			if err != nil {
				t.Error(err)
				reply(400, nil, "invalid path")
				return
			}
			content, err := io.ReadAll(r.Body)
			if err != nil {
				t.Error(err)
				return
			}
			fixture.mu.Lock()
			defer fixture.mu.Unlock()
			fixture.writes++
			if !fixture.dirs[path.Dir(remote)] {
				reply(500, nil, "parent missing")
				return
			}
			fixture.files[remote] = string(content)
			reply(200, nil, "success")
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	service := New(context.Background(), filepath.Join(t.TempDir(), "yunbo", "profile.json"))
	t.Cleanup(service.Close)
	if _, err := service.Connect(ConnectRequest{Profile: config.Profile{URL: server.URL, AuthMode: "guest", StartPath: "/target"}}); err != nil {
		t.Fatal(err)
	}
	return service, fixture, server.URL
}

func fixtureFile(t *testing.T, p, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}
func waitUploads(t *testing.T, s *Service) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		active := false
		for _, task := range s.Transfers() {
			if task.Status == "failed" {
				t.Fatalf("upload failed: %#v", task)
			}
			if task.Status == "running" || task.Status == "queued" {
				active = true
			}
		}
		if !active {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("uploads did not finish")
}

func TestUploadPathsPreservesFoldersAndContents(t *testing.T) {
	s, fixture, _ := newUploadFixture(t)
	local := t.TempDir()
	folder := filepath.Join(local, "照片 #+")
	fixtureFile(t, filepath.Join(folder, "中文.txt"), "中文内容")
	fixtureFile(t, filepath.Join(folder, "nested", "child.bin"), "nested data")
	if err := os.MkdirAll(filepath.Join(folder, "empty"), 0700); err != nil {
		t.Fatal(err)
	}
	fixtureFile(t, filepath.Join(local, "single.txt"), "single data")
	result, err := s.UploadPaths("/target", []string{folder, filepath.Join(local, "single.txt")})
	if err != nil || result.Count != 3 || result.Folders != 3 || len(result.Errors) != 0 {
		t.Fatalf("unexpected queue: %#v, %v", result, err)
	}
	waitUploads(t, s)
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	for remote, want := range map[string]string{"/target/照片 #+/中文.txt": "中文内容", "/target/照片 #+/nested/child.bin": "nested data", "/target/single.txt": "single data"} {
		if got := fixture.files[remote]; got != want {
			t.Errorf("%s = %q, want %q", remote, got, want)
		}
	}
	if !fixture.dirs["/target/照片 #+/empty"] {
		t.Fatal("empty directory was lost")
	}
}

func TestUploadPathsPreflightHasNoCloudSideEffects(t *testing.T) {
	s, fixture, _ := newUploadFixture(t)
	local := t.TempDir()
	folder := filepath.Join(local, "valid")
	fixtureFile(t, filepath.Join(folder, "a.txt"), "a")
	if _, err := s.UploadPaths("/target", []string{folder, filepath.Join(local, "missing")}); err == nil {
		t.Fatal("missing local file was accepted")
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if fixture.writes != 0 || len(s.Transfers()) != 0 {
		t.Fatal("partial upload started before preflight completed")
	}
}

func TestFailedDirectorySkipsItsFilesButKeepsIndependentFiles(t *testing.T) {
	s, fixture, _ := newUploadFixture(t)
	fixture.mu.Lock()
	fixture.failedDir = "/target/blocked"
	fixture.mu.Unlock()
	local := t.TempDir()
	fixtureFile(t, filepath.Join(local, "blocked", "child.txt"), "blocked")
	fixtureFile(t, filepath.Join(local, "good.txt"), "good")
	result, err := s.UploadPaths("/target", []string{filepath.Join(local, "blocked"), filepath.Join(local, "good.txt")})
	if err != nil || result.Count != 1 || len(result.Errors) != 1 {
		t.Fatalf("unexpected result: %#v %v", result, err)
	}
	waitUploads(t, s)
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if fixture.files["/target/good.txt"] != "good" {
		t.Fatal("independent file was not uploaded")
	}
	if _, exists := fixture.files["/target/blocked/child.txt"]; exists {
		t.Fatal("file uploaded into a failed directory")
	}
}

func TestFailedConnectPreservesExistingSession(t *testing.T) {
	s, _, address := newUploadFixture(t)
	if _, err := s.Connect(ConnectRequest{Profile: config.Profile{URL: address, AuthMode: "guest", StartPath: "/missing"}}); err == nil {
		t.Fatal("inaccessible start directory was accepted")
	}
	if _, err := s.List("/target", "", 1); err != nil {
		t.Fatalf("old connection was lost: %v", err)
	}
}

func TestUploadPlannerCapacityAndDuplicateTargets(t *testing.T) {
	local := t.TempDir()
	fixtureFile(t, filepath.Join(local, "folder", "a.txt"), "a")
	fixtureFile(t, filepath.Join(local, "folder", "b.txt"), "b")
	if _, err := planUploads(context.Background(), []string{filepath.Join(local, "folder")}, "/target", 1); err == nil {
		t.Fatal("queue capacity exceeded")
	}
	a, b := filepath.Join(local, "first", "same.txt"), filepath.Join(local, "second", "same.txt")
	fixtureFile(t, a, "first")
	fixtureFile(t, b, "second")
	if _, err := planUploads(context.Background(), []string{a, b}, "/target", 10); err == nil {
		t.Fatal("conflicting destinations accepted")
	}
	items, err := planUploads(context.Background(), []string{a, a}, "/target", 1)
	if err != nil || len(items) != 1 {
		t.Fatalf("same input not deduplicated: %#v %v", items, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = planUploads(ctx, []string{a}, "/target", 10); err == nil {
		t.Fatal("cancelled plan continued")
	}
}

func TestUploadPlannerRejectsEmptyAndRelativePaths(t *testing.T) {
	for _, input := range []string{"", ".", "relative.txt"} {
		if _, err := planUploads(context.Background(), []string{input}, "/target", 200); err == nil {
			t.Errorf("accepted invalid drop path %q", input)
		}
	}
}

func TestWailsJSONContract(t *testing.T) {
	s, _, address := newUploadFixture(t)
	var request ConnectRequest
	if err := json.Unmarshal([]byte(`{"url":"`+address+`","auth_mode":"guest","start_path":"/target","directory_password":"secret"}`), &request); err != nil {
		t.Fatal(err)
	}
	if request.DirectoryPassword != "secret" {
		t.Fatal("directory password was not bound")
	}
	result, err := s.Connect(request)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var response map[string]json.RawMessage
	if err = json.Unmarshal(data, &response); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"profile", "user", "listing", "warning"} {
		if _, ok := response[key]; !ok {
			t.Fatalf("missing Wails response field %s", key)
		}
	}
	if strings.Contains(string(data), "secret") {
		t.Fatal("directory password leaked in connection response")
	}
}
