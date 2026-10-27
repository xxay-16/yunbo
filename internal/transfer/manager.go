package transfer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"openlist-desk/internal/openlist"
)

type Task struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Direction  string `json:"direction"`
	Status     string `json:"status"`
	Done       int64  `json:"done"`
	Total      int64  `json:"total"`
	Error      string `json:"error"`
	LocalPath  string `json:"local_path"`
	RemotePath string `json:"remote_path"`
	Started    int64  `json:"started"`
}

type entry struct {
	Task
	cancel context.CancelFunc
}
type Manager struct {
	mu    sync.Mutex
	tasks map[int64]*entry
	next  int64
	slots chan struct{}
	wg    sync.WaitGroup
}

func New() *Manager { return &Manager{tasks: map[int64]*entry{}, slots: make(chan struct{}, 2)} }

func (m *Manager) add(ctx context.Context, name, direction, remote string, total int64, work func(context.Context, openlist.Progress) (string, error)) (int64, error) {
	m.mu.Lock()
	if len(m.tasks) >= 200 {
		m.mu.Unlock()
		return 0, errors.New("传输列表已满，请清除已完成的任务")
	}
	m.next++
	id := m.next
	ctx, cancel := context.WithCancel(ctx)
	m.tasks[id] = &entry{Task: Task{ID: id, Name: name, Direction: direction, RemotePath: remote, Total: total, Status: "queued"}, cancel: cancel}
	m.wg.Add(1)
	m.mu.Unlock()
	go func() {
		defer m.wg.Done()
		defer cancel()
		var err error
		var local string
		select {
		case m.slots <- struct{}{}:
			defer func() { <-m.slots }()
			if err = ctx.Err(); err == nil {
				m.mu.Lock()
				m.tasks[id].Status = "running"
				m.tasks[id].Started = time.Now().UnixMilli()
				m.mu.Unlock()
				local, err = work(ctx, func(done, total int64) {
					m.mu.Lock()
					m.tasks[id].Done = done
					m.tasks[id].Total = total
					m.mu.Unlock()
				})
			}
		case <-ctx.Done():
			err = ctx.Err()
		}
		m.mu.Lock()
		t := m.tasks[id]
		t.LocalPath = local
		switch {
		case errors.Is(err, context.Canceled):
			t.Status = "cancelled"
		case err != nil:
			t.Status = "failed"
			t.Error = err.Error()
		default:
			t.Status = "completed"
		}
		m.mu.Unlock()
	}()
	return id, nil
}

func (m *Manager) Upload(ctx context.Context, c *openlist.Client, local, remote string) (int64, error) {
	stat, err := os.Stat(local)
	if err != nil {
		return 0, err
	}
	if !stat.Mode().IsRegular() {
		return 0, errors.New("只能上传普通文件")
	}
	return m.add(ctx, filepath.Base(local), "upload", remote, stat.Size(), func(ctx context.Context, progress openlist.Progress) (string, error) {
		return local, c.Upload(ctx, local, remote, progress)
	})
}

// SafeLocalName prevents storage-provided names from becoming paths or Windows
// device/alternate-stream names when downloading.
func SafeLocalName(name string) error {
	if err := openlist.ValidateName(name); err != nil {
		return err
	}
	if strings.ContainsAny(name, "<>:\"|?*") || strings.TrimRight(name, ". ") != name {
		return errors.New("此文件名不能保存到 Windows，请先重命名")
	}
	base := strings.ToUpper(strings.SplitN(name, ".", 2)[0])
	if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || (len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9') {
		return errors.New("文件名是 Windows 保留名称")
	}
	for _, r := range name {
		if r < 32 {
			return errors.New("文件名包含控制字符")
		}
	}
	return nil
}

func (m *Manager) Download(ctx context.Context, c *openlist.Client, remote, name, password, dir string) (int64, error) {
	if err := SafeLocalName(name); err != nil {
		return 0, err
	}
	return m.add(ctx, name, "download", remote, 0, func(ctx context.Context, progress openlist.Progress) (string, error) {
		f, err := os.CreateTemp(dir, ".openlist-*.part")
		if err != nil {
			return "", err
		}
		defer os.Remove(f.Name())
		err = c.Download(ctx, remote, password, f, progress)
		if closeErr := f.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			return "", err
		}
		if err = ctx.Err(); err != nil {
			return "", err
		}
		return publish(f.Name(), dir, name)
	})
}

func publish(temp, dir, name string) (string, error) {
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	for i := 0; i < 10000; i++ {
		candidate := name
		if i > 0 {
			candidate = fmt.Sprintf("%s (%d)%s", stem, i, ext)
		}
		dst := filepath.Join(dir, candidate)
		// Reserve exclusively so parallel jobs and existing user files cannot be overwritten.
		reservation, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		if err = reservation.Close(); err != nil {
			os.Remove(dst)
			return "", err
		}
		if err = os.Rename(temp, dst); err != nil {
			os.Remove(dst)
			return "", err
		}
		return dst, nil
	}
	return "", errors.New("同名下载文件过多，请选择其他目录")
}

func (m *Manager) Snapshot() []Task {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]Task, 0, len(m.tasks))
	for _, t := range m.tasks {
		result = append(result, t.Task)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID > result[j].ID })
	return result
}

func (m *Manager) Cancel(id int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if t := m.tasks[id]; t != nil {
		t.cancel()
	}
}

func (m *Manager) ClearFinished() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, t := range m.tasks {
		if t.Status != "running" && t.Status != "queued" {
			delete(m.tasks, id)
		}
	}
}

func (m *Manager) Close() {
	m.mu.Lock()
	for _, t := range m.tasks {
		t.cancel()
	}
	m.mu.Unlock()
	m.wg.Wait()
}
