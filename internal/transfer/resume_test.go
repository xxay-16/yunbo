package transfer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/xxay-16/yunbo/internal/openlist"
)

func waitStatus(t *testing.T, m *Manager, id int64, status string) Task {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		for _, task := range m.Snapshot() {
			if task.ID == id {
				if task.Status == status {
					return task
				}
				if task.Status == "failed" && status != "failed" {
					t.Fatalf("task failed: %#v", task)
				}
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("task %d did not reach %s: %#v", id, status, m.Snapshot())
	return Task{}
}

func TestSpeedWindowExcludesRestoredBytesAndStalls(t *testing.T) {
	m := New()
	now := time.Now()
	e := &entry{Task: Task{Status: "running", Phase: "transferring", Done: 11 << 20, Total: 15 << 20}, samples: []rateSample{{now.Add(-2 * time.Second), 10 << 20}}, lastProgress: now}
	task := m.snapshotLocked(e, now)
	if task.SpeedBPS != 512<<10 || task.ETASeconds != 8 {
		t.Fatalf("bad speed/ETA: %#v", task)
	}
	e.lastProgress = now.Add(-6 * time.Second)
	task = m.snapshotLocked(e, now)
	if task.SpeedBPS != 0 || task.ETASeconds != -1 {
		t.Fatalf("stalled task retained old speed: %#v", task)
	}
	for _, status := range []string{"paused", "queued", "completed", "failed"} {
		e.Status = status
		e.lastProgress = now
		task = m.snapshotLocked(e, now)
		if task.SpeedBPS != 0 {
			t.Fatalf("%s reported active speed", status)
		}
	}
}

type multipartFixture struct {
	mu            sync.Mutex
	payload       []byte
	chunks        map[int][]byte
	calls         map[int]int
	active        bool
	completed     bool
	block         bool
	acceptBlocked bool
	blocked       chan struct{}
	once          sync.Once
}

func (f *multipartFixture) snapshot() openlist.MultipartSnapshot {
	state := "receiving"
	if f.completed {
		state = "completed"
	}
	s := openlist.MultipartSnapshot{ID: "test-session", State: state, Path: "/large.bin", Size: int64(len(f.payload)), ChunkSize: 1 << 20, TotalChunks: (len(f.payload) + (1 << 20) - 1) / (1 << 20), Received: [][2]int{}}
	var indexes []int
	for index, chunk := range f.chunks {
		indexes = append(indexes, index)
		s.ReceivedBytes += int64(len(chunk))
	}
	sort.Ints(indexes)
	for _, index := range indexes {
		last := len(s.Received) - 1
		if last >= 0 && s.Received[last][1] == index-1 {
			s.Received[last][1] = index
		} else {
			s.Received = append(s.Received, [2]int{index, index})
		}
	}
	return s
}
func newMultipartFixture(t *testing.T) (*openlist.Client, *multipartFixture) {
	t.Helper()
	fixture := &multipartFixture{payload: bytes.Repeat([]byte("a"), 3<<20), chunks: map[int][]byte{}, calls: map[int]int{}, block: true, blocked: make(chan struct{})}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reply := func(code int, data any) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"code": code, "message": "test", "data": data})
		}
		switch r.URL.Path {
		case "/api/fs/multipart/status":
			fixture.mu.Lock()
			defer fixture.mu.Unlock()
			if !fixture.active || r.URL.Query().Get("upload_id") == "yunbo-capability-probe" {
				reply(404, nil)
				return
			}
			reply(200, fixture.snapshot())
		case "/api/fs/multipart/init":
			target, _ := url.PathUnescape(r.Header.Get("File-Path"))
			if target != "/large.bin" || r.Header.Get("Overwrite") != "false" || r.Header.Get("X-File-Sha256") == "" {
				t.Error("invalid multipart initialization")
			}
			fixture.mu.Lock()
			fixture.active = true
			snapshot := fixture.snapshot()
			fixture.mu.Unlock()
			reply(200, snapshot)
		case "/api/fs/multipart/chunk":
			index, _ := strconv.Atoi(r.Header.Get("X-Chunk-Index"))
			body, _ := io.ReadAll(r.Body)
			fixture.mu.Lock()
			fixture.calls[index]++
			block := fixture.block && index == 1
			if block && fixture.acceptBlocked {
				fixture.chunks[index] = body
			}
			fixture.mu.Unlock()
			if block {
				fixture.once.Do(func() { close(fixture.blocked) })
				<-r.Context().Done()
				return
			}
			fixture.mu.Lock()
			fixture.chunks[index] = body
			snapshot := fixture.snapshot()
			fixture.mu.Unlock()
			reply(200, snapshot)
		case "/api/fs/multipart/complete":
			fixture.mu.Lock()
			fixture.completed = true
			snapshot := fixture.snapshot()
			fixture.mu.Unlock()
			reply(200, snapshot)
		case "/api/fs/multipart/abort":
			reply(200, nil)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	c, _ := openlist.New(server.URL, "do-not-persist-token")
	c.User.Username = "alice"
	c.UploadResume = true
	return c, fixture
}

func TestMultipartUploadResumesAfterProcessRestart(t *testing.T) {
	c, fixture := newMultipartFixture(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "large.bin")
	os.WriteFile(file, fixture.payload, 0600)
	store := filepath.Join(dir, "state", "transfers.json")
	m, err := NewPersistent(store)
	if err != nil {
		t.Fatal(err)
	}
	id, err := m.Upload(context.Background(), c, file, "/large.bin")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-fixture.blocked:
	case <-time.After(5 * time.Second):
		t.Fatal("second chunk did not start")
	}
	if err = m.Pause(id); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, m, id, "paused")
	m.Close()
	body, _ := os.ReadFile(store)
	if strings.Contains(string(body), "do-not-persist-token") {
		t.Fatal("account token leaked into checkpoint")
	}
	resumed, err := NewPersistent(store)
	if err != nil {
		t.Fatal(err)
	}
	defer resumed.Close()
	resumed.BindClient(context.Background(), c)
	fixture.mu.Lock()
	fixture.block = false
	fixture.mu.Unlock()
	if err = resumed.Resume(context.Background(), c, id, ""); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, resumed, id, "completed")
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if fixture.calls[0] != 1 {
		t.Fatalf("confirmed chunk was retransmitted: %v", fixture.calls)
	}
	var result []byte
	for i := 0; i < 3; i++ {
		result = append(result, fixture.chunks[i]...)
	}
	if !bytes.Equal(result, fixture.payload) {
		t.Fatal("multipart content changed after resume")
	}
}

func TestMultipartResumeRejectsChangedSource(t *testing.T) {
	c, fixture := newMultipartFixture(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "large.bin")
	os.WriteFile(file, fixture.payload, 0600)
	info, _ := os.Stat(file)
	m := New()
	defer m.Close()
	id, _ := m.Upload(context.Background(), c, file, "/large.bin")
	select {
	case <-fixture.blocked:
	case <-time.After(5 * time.Second):
		t.Fatal("upload did not start")
	}
	if err := m.Pause(id); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, m, id, "paused")
	changed := append([]byte(nil), fixture.payload...)
	changed[len(changed)-100] ^= 1
	if err := os.WriteFile(file, changed, 0600); err != nil {
		t.Fatal(err)
	}
	os.Chtimes(file, info.ModTime(), info.ModTime())
	if err := m.Resume(context.Background(), c, id, ""); err != nil {
		t.Fatal(err)
	}
	task := waitStatus(t, m, id, "failed")
	if task.Resumable || !strings.Contains(task.Error, "内容已变化") {
		t.Fatalf("changed source accepted: %#v", task)
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if fixture.calls[0] != 1 {
		t.Fatal("changed source reached upload endpoint")
	}
}

func TestMultipartResumeSkipsChunkWhoseAcknowledgementWasLost(t *testing.T) {
	c, fixture := newMultipartFixture(t)
	fixture.mu.Lock()
	fixture.acceptBlocked = true
	fixture.mu.Unlock()
	file := filepath.Join(t.TempDir(), "large.bin")
	os.WriteFile(file, fixture.payload, 0600)
	m := New()
	defer m.Close()
	id, err := m.Upload(context.Background(), c, file, "/large.bin")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-fixture.blocked:
	case <-time.After(5 * time.Second):
		t.Fatal("upload did not reach blocked acknowledgement")
	}
	if err = m.Pause(id); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, m, id, "paused")
	fixture.mu.Lock()
	fixture.block = false
	fixture.mu.Unlock()
	if err = m.Resume(context.Background(), c, id, ""); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, m, id, "completed")
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if fixture.calls[1] != 1 {
		t.Fatalf("confirmed but unacknowledged chunk resent: %v", fixture.calls)
	}
}

func TestMultipartResumeRejectsExpiredSessionAndWrongAccount(t *testing.T) {
	c, fixture := newMultipartFixture(t)
	file := filepath.Join(t.TempDir(), "large.bin")
	os.WriteFile(file, fixture.payload, 0600)
	m := New()
	defer m.Close()
	id, err := m.Upload(context.Background(), c, file, "/large.bin")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-fixture.blocked:
	case <-time.After(5 * time.Second):
		t.Fatal("upload did not start")
	}
	if err = m.Pause(id); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, m, id, "paused")
	other, _ := openlist.New(c.Address(), "other-token")
	other.User.Username = "bob"
	other.UploadResume = true
	if err = m.Resume(context.Background(), other, id, ""); err == nil {
		t.Fatal("task resumed under a different account")
	}
	fixture.mu.Lock()
	fixture.active = false
	fixture.mu.Unlock()
	if err = m.Resume(context.Background(), c, id, ""); err != nil {
		t.Fatal(err)
	}
	task := waitStatus(t, m, id, "failed")
	if task.Resumable || !strings.Contains(task.Error, "会话已过期") {
		t.Fatalf("expired session advertised as resumable: %#v", task)
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if fixture.calls[0] != 1 {
		t.Fatal("expired session silently restarted from zero")
	}
}

func TestCorruptCheckpointIsNotOverwritten(t *testing.T) {
	file := filepath.Join(t.TempDir(), "transfers.json")
	body := []byte("not JSON")
	os.WriteFile(file, body, 0600)
	if _, err := NewPersistent(file); err == nil {
		t.Fatal("invalid checkpoint accepted")
	}
	got, _ := os.ReadFile(file)
	if !bytes.Equal(got, body) {
		t.Fatal("corrupt checkpoint was overwritten")
	}
}

func TestDownloadResumesAfterRestartAndKeepsPasswordPrivate(t *testing.T) {
	payload := bytes.Repeat([]byte("download"), 256<<10)
	var mu sync.Mutex
	var starts []int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/fs/get" {
			json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": openlist.File{Name: "data.bin", Size: int64(len(payload)), Modified: "2026-10-27T00:00:00Z"}})
			return
		}
		var start int64
		fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-", &start)
		mu.Lock()
		starts = append(starts, start)
		mu.Unlock()
		w.Header().Set("ETag", `"v1"`)
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, len(payload)-1, len(payload)))
		w.Header().Set("Content-Length", strconv.Itoa(len(payload)-int(start)))
		w.WriteHeader(206)
		for offset := start; offset < int64(len(payload)); offset += 16384 {
			end := min(offset+16384, int64(len(payload)))
			if _, err := w.Write(payload[offset:end]); err != nil {
				return
			}
			w.(http.Flusher).Flush()
			select {
			case <-r.Context().Done():
				return
			case <-time.After(3 * time.Millisecond):
			}
		}
	}))
	defer server.Close()
	c, _ := openlist.New(server.URL, "private-account-token")
	c.User.Username = "alice"
	root := t.TempDir()
	dir := filepath.Join(root, "downloads")
	os.Mkdir(dir, 0700)
	store := filepath.Join(root, "state", "transfers.json")
	m, err := NewPersistent(store)
	if err != nil {
		t.Fatal(err)
	}
	id, err := m.Download(context.Background(), c, "/data.bin", "data.bin", "private-dir-password", dir)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		if m.Snapshot()[0].Done > 65536 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if err = m.Pause(id); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, m, id, "paused")
	m.mu.Lock()
	partial := m.tasks[id].data.Temp
	m.mu.Unlock()
	info, _ := os.Stat(partial)
	offset := info.Size()
	m.Close()
	body, _ := os.ReadFile(store)
	if strings.Contains(string(body), "private-dir-password") || strings.Contains(string(body), "private-account-token") {
		t.Fatal("credentials persisted")
	}
	resumed, err := NewPersistent(store)
	if err != nil {
		t.Fatal(err)
	}
	defer resumed.Close()
	resumed.BindClient(context.Background(), c)
	if !resumed.Snapshot()[0].NeedsPassword {
		t.Fatal("restored protected download did not request its password")
	}
	if err = resumed.Resume(context.Background(), c, id, "private-dir-password"); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, resumed, id, "completed")
	got, _ := os.ReadFile(filepath.Join(dir, "data.bin"))
	if !bytes.Equal(got, payload) {
		t.Fatal("resumed download is corrupt")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(starts) != 2 || starts[1] != offset {
		t.Fatalf("wrong resumed range: %v, expected %d", starts, offset)
	}
}
