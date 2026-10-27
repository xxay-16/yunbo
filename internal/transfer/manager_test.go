package transfer

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/xxay-16/yunbo/internal/openlist"
)

func waitFinished(t *testing.T, m *Manager) []Task {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		tasks := m.Snapshot()
		finished := true
		for _, task := range tasks {
			if task.Status == "running" || task.Status == "queued" || task.Status == "pausing" || task.Status == "cancelling" {
				finished = false
			}
		}
		if finished {
			return tasks
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("transfer did not finish")
	return nil
}

func TestDownloadPreservesExistingFiles(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/fs/get" {
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": openlist.File{Name: "sample.txt", Size: 8}})
			return
		}
		_, _ = io.WriteString(w, "new data")
	}))
	defer server.Close()
	c, _ := openlist.New(server.URL, "")
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "sample.txt"), []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	m := New()
	defer m.Close()
	for i := 0; i < 2; i++ {
		if _, err := m.Download(context.Background(), c, "/sample.txt", "sample.txt", "", dir); err != nil {
			t.Fatal(err)
		}
	}
	for _, task := range waitFinished(t, m) {
		if task.Status != "completed" {
			t.Fatalf("task failed: %#v", task)
		}
	}
	for _, name := range []string{"sample.txt", "sample (1).txt", "sample (2).txt"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		want := "new data"
		if name == "sample.txt" {
			want = "original"
		}
		if string(data) != want {
			t.Fatalf("%s changed unexpectedly: %q", name, data)
		}
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 3 {
		t.Fatal("temporary files were left behind")
	}
}

func TestCancelRemovesPartialDownload(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/fs/get" {
			_, _ = io.WriteString(w, `{"code":200,"data":{"name":"test.txt","size":100}}`)
			return
		}
		w.Header().Set("Content-Length", "100")
		_, _ = io.WriteString(w, "partial")
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()
	c, _ := openlist.New(server.URL, "")
	dir := t.TempDir()
	m := New()
	defer m.Close()
	id, err := m.Download(context.Background(), c, "/test.txt", "test.txt", "", dir)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("download did not start")
	}
	m.Cancel(id)
	tasks := waitFinished(t, m)
	if tasks[0].Status != "cancelled" {
		t.Fatalf("unexpected task: %#v", tasks[0])
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatal("cancel left partial files behind")
	}
	m.ClearFinished()
	if len(m.Snapshot()) != 0 {
		t.Fatal("clear finished tasks failed")
	}
}

func TestQueueHonorsConcurrencyAndCancellation(t *testing.T) {
	m := New()
	defer m.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for i := 0; i < 5; i++ {
		_, err := m.add(ctx, "test", "upload", "/test", 0, func(ctx context.Context, _ openlist.Progress) (string, error) { <-ctx.Done(); return "", ctx.Err() })
		if err != nil {
			t.Fatal(err)
		}
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		running := 0
		for _, task := range m.Snapshot() {
			if task.Status == "running" {
				running++
			}
		}
		if running > 2 {
			t.Fatal("concurrency exceeded two")
		}
		if running == 2 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	for _, task := range waitFinished(t, m) {
		if task.Status != "cancelled" {
			t.Fatalf("task not cancelled: %#v", task)
		}
	}
}

func TestSafeLocalName(t *testing.T) {
	for _, name := range []string{"../escape", "a\\b", "C:stream", "a\x01b", "CON", "nul.txt", "COM1", "lpt9.log", "file.", "file ", "a?b"} {
		if err := SafeLocalName(name); err == nil {
			t.Errorf("accepted %q", name)
		}
	}
	for _, name := range []string{"中文.txt", "notes #+%.md", "CONTEXT.md", "a.b.c"} {
		if err := SafeLocalName(name); err != nil {
			t.Errorf("rejected %q: %v", name, err)
		}
	}
}

func TestClosedManagerRejectsNewTasks(t *testing.T) {
	m := New()
	m.Close()
	if _, err := m.add(context.Background(), "late", "upload", "/late", 0, func(context.Context, openlist.Progress) (string, error) {
		t.Error("work ran after shutdown")
		return "", nil
	}); err == nil {
		t.Fatal("closed manager accepted a task")
	}
	if m.RemainingCapacity() != 0 || len(m.Snapshot()) != 0 {
		t.Fatal("closed manager reported writable capacity")
	}
}
