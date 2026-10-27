package openlist

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestDownloadRangeResume(t *testing.T) {
	payload := bytes.Repeat([]byte("range-data-"), 300000)
	modified := time.Date(2026, 10, 27, 1, 2, 3, 0, time.UTC)
	var mu sync.Mutex
	var ranges []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/fs/get" {
			success(w, File{Name: "file.bin", Size: int64(len(payload)), Modified: modified.Format(time.RFC3339)})
			return
		}
		mu.Lock()
		ranges = append(ranges, r.Header.Get("Range"))
		mu.Unlock()
		w.Header().Set("ETag", `"v1"`)
		http.ServeContent(w, r, "file.bin", modified, bytes.NewReader(payload))
	}))
	defer server.Close()
	c, _ := New(server.URL, "")
	f, err := os.Create(filepath.Join(t.TempDir(), "partial"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var state DownloadState
	ctx, cancel := context.WithCancel(context.Background())
	err = c.DownloadResumable(ctx, "/file.bin", "", f, &state, func(DownloadState) error { return nil }, func(done, total int64) {
		if done >= 65536 {
			cancel()
		}
	})
	if err == nil || !state.Resumable {
		t.Fatalf("interruption not retained: %#v %v", state, err)
	}
	stat, _ := f.Stat()
	offset := stat.Size()
	if offset <= 0 || offset >= int64(len(payload)) {
		t.Fatalf("invalid partial length %d", offset)
	}
	if err = c.DownloadResumable(context.Background(), "/file.bin", "", f, &state, func(DownloadState) error { return nil }, nil); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(f.Name())
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatal("resumed file differs from source")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(ranges) != 2 || ranges[1] != fmt.Sprintf("bytes=%d-", offset) {
		t.Fatalf("wrong resume offset: %v", ranges)
	}
}

func TestDownloadRefusesUnsafeAppend(t *testing.T) {
	for _, test := range []string{"ignored-range", "wrong-range", "changed-etag", "changed-metadata"} {
		t.Run(test, func(t *testing.T) {
			payload := bytes.Repeat([]byte("x"), 1000)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/fs/get" {
					modified := "old"
					if test == "changed-metadata" {
						modified = "new"
					}
					success(w, File{Name: "x", Size: 1000, Modified: modified})
					return
				}
				w.Header().Set("ETag", `"v1"`)
				switch test {
				case "ignored-range":
					w.Header().Set("Content-Length", "1000")
					w.Write(payload)
				case "wrong-range":
					w.Header().Set("Content-Range", "bytes 0-999/1000")
					w.WriteHeader(206)
					w.Write(payload)
				default:
					w.Header().Set("ETag", `"v2"`)
					w.Header().Set("Content-Range", "bytes 300-999/1000")
					w.WriteHeader(206)
					w.Write(payload[300:])
				}
			}))
			defer server.Close()
			c, _ := New(server.URL, "")
			file := filepath.Join(t.TempDir(), "partial")
			os.WriteFile(file, payload[:300], 0600)
			f, _ := os.OpenFile(file, os.O_RDWR, 0600)
			defer f.Close()
			state := DownloadState{Initialized: true, Resumable: true, Total: 1000, Modified: "old", ETag: `"v1"`}
			err := c.DownloadResumable(context.Background(), "/x", "", f, &state, func(DownloadState) error { return nil }, nil)
			if !errors.Is(err, ErrCannotResume) {
				t.Fatalf("unsafe response accepted: %v", err)
			}
			got, _ := os.ReadFile(file)
			if !bytes.Equal(got, payload[:300]) {
				t.Fatal("partial data was changed before validation")
			}
		})
	}
}

func TestProbeUploadResumeDistinguishesHTTP404(t *testing.T) {
	for _, supported := range []bool{false, true} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !supported {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"code":404,"message":"multipart upload session not found","data":null}`)
		}))
		c, _ := New(server.URL, "")
		c.ProbeUploadResume(context.Background())
		server.Close()
		if c.UploadResume != supported {
			t.Fatalf("capability = %v, want %v", c.UploadResume, supported)
		}
	}
}
