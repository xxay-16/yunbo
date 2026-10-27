package openlist

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func success(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "message": "success", "data": data})
}

func TestLoginListAndSubpath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/openlist/api/auth/login/hash":
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			h := sha256.Sum256([]byte("s3cret-https://github.com/alist-org/alist"))
			if body["password"] != hex.EncodeToString(h[:]) || body["username"] != "alice" || body["otp_code"] != "123456" {
				t.Error("login credentials were not hashed/encoded as required")
			}
			success(w, map[string]string{"token": "session-token"})
		case "/openlist/api/me":
			if r.Header.Get("Authorization") != "session-token" {
				t.Error("missing raw token authorization")
			}
			success(w, User{Username: "alice", BasePath: "/users/alice"})
		case "/openlist/api/fs/list":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["path"] != "/中文/# 目录" || body["page"] != float64(2) || body["per_page"] != float64(100) || body["password"] != "dirpass" || body["refresh"] != false {
				t.Errorf("bad list request: %#v", body)
			}
			success(w, map[string]any{"content": nil, "total": 101, "write": true})
		default:
			t.Errorf("unexpected endpoint: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	c, err := New(server.URL+"/openlist/", "")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err = c.Login(ctx, "alice", "s3cret", "123456"); err != nil {
		t.Fatal(err)
	}
	if err = c.LoadUser(ctx); err != nil {
		t.Fatal(err)
	}
	list, err := c.List(ctx, "/中文/# 目录", "dirpass", 2)
	if err != nil || list.Total != 101 || !list.Write || list.Content == nil {
		t.Fatalf("listing=%#v, err=%v", list, err)
	}
}

func TestAPIRejectsErrorsAndRedirects(t *testing.T) {
	var leaked atomic.Bool
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked.Store(true); success(w, User{}) }))
	defer other.Close()
	for _, tc := range []struct {
		name, body, contains string
		status               int
	}{
		{"permission", `{"code":403,"message":"denied","data":null}`, "denied", 200},
		{"html", `<html>login proxy</html>`, "有效的 OpenList", 200},
		{"http", `bad gateway`, "HTTP 502", 502},
		{"null", `{"code":200,"data":null}`, "空数据", 200},
		{"redirect", "", "HTTP 307", 307},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.status == 307 {
					w.Header().Set("Location", other.URL)
				}
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer server.Close()
			c, _ := New(server.URL, "secret")
			if err := c.LoadUser(context.Background()); err == nil || !strings.Contains(err.Error(), tc.contains) {
				t.Fatalf("unexpected error %v", err)
			}
		})
	}
	if leaked.Load() {
		t.Fatal("credential-bearing API request followed a redirect")
	}
}

func TestStreamingUploadAndSignedDownload(t *testing.T) {
	ctx := context.Background()
	payload := bytes.Repeat([]byte("stream-data-中文"), 8192)
	local := filepath.Join(t.TempDir(), "source.bin")
	if err := os.WriteFile(local, payload, 0600); err != nil {
		t.Fatal(err)
	}
	var progress, upload atomic.Int64
	storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("account token leaked into download redirect")
		}
		_, _ = w.Write(payload)
	}))
	defer storage.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/prefix/api/fs/put":
			p, _ := url.PathUnescape(r.Header.Get("File-Path"))
			if p != "/测试 #+?.bin" || r.Header.Get("Overwrite") != "false" || r.Header.Get("As-Task") != "false" || r.ContentLength != int64(len(payload)) || r.Header.Get("Authorization") != "secret" {
				t.Errorf("invalid upload headers: %v", r.Header)
			}
			got, _ := io.ReadAll(r.Body)
			if !bytes.Equal(got, payload) {
				t.Error("upload payload changed")
			}
			upload.Store(int64(len(got)))
			success(w, nil)
		case "/prefix/api/fs/get":
			success(w, File{Name: "测试 #+?.bin", Size: int64(len(payload)), Sign: "a+b:123"})
		case "/prefix/d/users/alice/测试 #+?.bin":
			if r.URL.Query().Get("sign") != "a+b:123" || r.Header.Get("Authorization") != "" {
				t.Errorf("bad signed download request: %v", r.URL)
			}
			http.Redirect(w, r, storage.URL, http.StatusFound)
		default:
			t.Errorf("unexpected URL: %s", r.URL)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	c, _ := New(server.URL+"/prefix", "secret")
	c.User.BasePath = "/users/alice"
	if err := c.Upload(ctx, local, "/测试 #+?.bin", func(done, total int64) { progress.Store(done) }); err != nil {
		t.Fatal(err)
	}
	if progress.Load() != int64(len(payload)) || upload.Load() != int64(len(payload)) {
		t.Fatal("upload progress did not reach payload size")
	}
	var output bytes.Buffer
	if err := c.Download(ctx, "/测试 #+?.bin", "", &output, nil); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(output.Bytes(), payload) {
		t.Fatal("download payload changed")
	}
}

func TestTruncatedDownloadFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/fs/get" {
			success(w, File{Name: "test.bin", Size: 100})
			return
		}
		w.Header().Set("Content-Length", "100")
		_, _ = io.WriteString(w, "partial")
	}))
	defer server.Close()
	c, _ := New(server.URL, "")
	if err := c.Download(context.Background(), "/test.bin", "", io.Discard, nil); err == nil {
		t.Fatal("truncated download reported success")
	}
}

func TestValidateAddressAndName(t *testing.T) {
	for _, address := range []string{"", "example.com", "ftp://example.com", "https://user:pass@example.com", "https://example.com?token=x", "https://example.com/#x"} {
		if _, err := New(address, ""); err == nil {
			t.Errorf("accepted invalid URL %q", address)
		}
	}
	for _, name := range []string{"", ".", "..", "a/b", "a\\b", "a\x00b", "a\nb"} {
		if err := ValidateName(name); err == nil {
			t.Errorf("accepted invalid name %q", name)
		}
	}
}
