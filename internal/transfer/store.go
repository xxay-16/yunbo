package transfer

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/xxay-16/yunbo/internal/openlist"
)

// Only transfer metadata is persisted. Account tokens, passwords, signed URLs
// and directory passwords have no fields in this structure.
type jobData struct {
	Kind           string                 `json:"kind"`
	DownloadDir    string                 `json:"download_dir,omitempty"`
	Temp           string                 `json:"temp,omitempty"`
	Download       openlist.DownloadState `json:"download,omitempty"`
	UploadID       string                 `json:"upload_id,omitempty"`
	SHA256         string                 `json:"sha256,omitempty"`
	SourceSize     int64                  `json:"source_size,omitempty"`
	SourceModified int64                  `json:"source_modified,omitempty"`
	NeedsPassword  bool                   `json:"needs_password,omitempty"`
}
type diskTask struct {
	Task Task    `json:"task"`
	Data jobData `json:"checkpoint"`
}
type diskStore struct {
	Version int        `json:"version"`
	NextID  int64      `json:"next_id"`
	Tasks   []diskTask `json:"tasks"`
}

func NewPersistent(file string) (*Manager, error) {
	m := New()
	m.store = file
	f, err := os.Open(file)
	if errors.Is(err, os.ErrNotExist) {
		return m, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var data diskStore
	decoder := json.NewDecoder(io.LimitReader(f, 4<<20))
	if err = decoder.Decode(&data); err != nil {
		return nil, fmt.Errorf("无法读取续传记录：%w", err)
	}
	if data.Version != 1 || data.NextID < 0 || len(data.Tasks) > MaxTasks {
		return nil, errors.New("续传记录版本或任务数量无效")
	}
	m.next = data.NextID
	for _, record := range data.Tasks {
		t, d := record.Task, record.Data
		if t.ID <= 0 || m.tasks[t.ID] != nil || (d.Kind != "upload" && d.Kind != "download") || d.Kind != t.Direction || t.Server == "" || !filepath.IsAbs(t.LocalPath) {
			return nil, errors.New("续传记录包含无效任务")
		}
		if d.Kind == "download" {
			if !filepath.IsAbs(d.DownloadDir) || SafeLocalName(t.Name) != nil {
				return nil, errors.New("下载续传路径无效")
			}
			if d.Temp != "" && (filepath.Dir(d.Temp) != filepath.Clean(d.DownloadDir) || !strings.HasPrefix(filepath.Base(d.Temp), ".yunbo-") || !strings.HasSuffix(d.Temp, ".part")) {
				return nil, errors.New("下载临时文件路径无效")
			}
		}
		if t.Status != "completed" && t.Status != "cancelled" {
			if t.Resumable {
				t.Status = "paused"
				t.Error = ""
				t.ResumeReason = "重新连接服务器后可继续"
			} else {
				t.Status = "cancelled"
				t.Error = "上次普通传输已中断，服务器不支持此任务续传"
			}
		}
		t.SpeedBPS = 0
		t.ETASeconds = -1
		t.CanPause = false
		t.CanResume = false
		m.tasks[t.ID] = &entry{Task: t, data: d}
		if t.ID > m.next {
			m.next = t.ID
		}
	}
	return m, nil
}
func (m *Manager) saveLocked() error {
	if m.store == "" {
		return nil
	}
	data := diskStore{Version: 1, NextID: m.next, Tasks: []diskTask{}}
	for _, e := range m.tasks {
		if e.data.Kind == "" {
			continue
		}
		t := e.Task
		t.SpeedBPS = 0
		t.ETASeconds = -1
		t.CanPause = false
		t.CanResume = false
		t.NeedsPassword = false
		data.Tasks = append(data.Tasks, diskTask{Task: t, Data: e.data})
	}
	body, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(m.store), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(m.store), ".transfers-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(body)
	}
	if err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(f.Name(), m.store); err != nil {
		return err
	}
	m.lastSave = time.Now()
	return nil
}
