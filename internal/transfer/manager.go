package transfer

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/xxay-16/yunbo/internal/openlist"
)

const MaxTasks = 200

type Task struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	Direction     string `json:"direction"`
	Status        string `json:"status"`
	Done          int64  `json:"done"`
	Total         int64  `json:"total"`
	Error         string `json:"error"`
	LocalPath     string `json:"local_path"`
	RemotePath    string `json:"remote_path"`
	Started       int64  `json:"started"`
	Server        string `json:"server"`
	Account       string `json:"account"`
	SpeedBPS      int64  `json:"speed_bps"`
	ETASeconds    int64  `json:"eta_seconds"`
	Resumable     bool   `json:"resumable"`
	CanPause      bool   `json:"can_pause"`
	CanResume     bool   `json:"can_resume"`
	NeedsPassword bool   `json:"needs_password"`
	ResumeReason  string `json:"resume_reason"`
	Phase         string `json:"phase"`
	PhaseProgress int    `json:"phase_progress"`
}

type rateSample struct {
	at   time.Time
	done int64
}
type entry struct {
	Task
	data         jobData
	client       *openlist.Client
	parent       context.Context
	password     string
	cancel       context.CancelFunc
	active       bool
	intent       string
	samples      []rateSample
	lastProgress time.Time
	persistErr   error
	work         func(context.Context, openlist.Progress) (string, error)
}
type Manager struct {
	mu       sync.Mutex
	tasks    map[int64]*entry
	next     int64
	slots    chan struct{}
	wg       sync.WaitGroup
	closed   bool
	store    string
	lastSave time.Time
}

func New() *Manager { return &Manager{tasks: map[int64]*entry{}, slots: make(chan struct{}, 2)} }

func (m *Manager) add(ctx context.Context, name, direction, remote string, total int64, work func(context.Context, openlist.Progress) (string, error)) (int64, error) {
	return m.insert(&entry{Task: Task{Name: name, Direction: direction, RemotePath: remote, Total: total}, parent: ctx, work: work})
}

func (m *Manager) insert(e *entry) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || e.parent.Err() != nil {
		return 0, context.Canceled
	}
	if len(m.tasks) >= MaxTasks {
		return 0, errors.New("传输列表已满，请清除已结束的任务")
	}
	m.next++
	e.ID = m.next
	e.Status = "queued"
	e.ETASeconds = -1
	m.tasks[e.ID] = e
	if err := m.saveLocked(); err != nil {
		delete(m.tasks, e.ID)
		return 0, err
	}
	m.startLocked(e)
	return e.ID, nil
}

func (m *Manager) startLocked(e *entry) {
	ctx, cancel := context.WithCancel(e.parent)
	e.cancel = cancel
	e.active = true
	e.intent = ""
	e.persistErr = nil
	e.Status = "queued"
	e.Error = ""
	e.Phase = ""
	e.PhaseProgress = 0
	e.samples = nil
	m.wg.Add(1)
	go func() { defer cancel(); m.run(e, ctx) }()
}

func (m *Manager) run(e *entry, ctx context.Context) {
	defer m.wg.Done()
	var err error
	var local string
	select {
	case m.slots <- struct{}{}:
		if err = ctx.Err(); err == nil {
			m.mu.Lock()
			if e.intent == "" {
				e.Status = "running"
			}
			if e.Started == 0 {
				e.Started = time.Now().UnixMilli()
			}
			e.samples = []rateSample{{time.Now(), e.Done}}
			m.mu.Unlock()
			progress := func(done, total int64) { m.report(e, done, total) }
			if e.work != nil {
				m.phase(e, "transferring", 0)
				local, err = e.work(ctx, progress)
			} else {
				local, err = m.execute(e, ctx, progress)
			}
		}
		<-m.slots
	case <-ctx.Done():
		err = ctx.Err()
	}
	m.mu.Lock()
	if e.persistErr != nil {
		err = e.persistErr
	}
	if local != "" {
		e.LocalPath = local
	}
	if errors.Is(err, openlist.ErrCannotResume) {
		e.Resumable = false
		e.ResumeReason = err.Error()
	}
	var apiError *openlist.APIError
	if e.Direction == "download" && e.data.NeedsPassword && errors.As(err, &apiError) && apiError.Code == 403 {
		e.password = ""
	}
	status := "completed"
	switch {
	case err == nil:
	case e.intent == "cancel":
		status = "cancelled"
	case e.persistErr != nil:
		status = "failed"
	case (errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled)) && e.Resumable:
		status = "paused"
	case errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled):
		status = "cancelled"
	default:
		status = "failed"
	}
	cleanup := status == "cancelled"
	if err != nil && status == "failed" {
		e.Error = err.Error()
	}
	m.mu.Unlock()
	if cleanup {
		m.cleanup(e)
	}
	m.mu.Lock()
	e.Status = status
	e.active = false
	e.cancel = nil
	e.samples = nil
	e.SpeedBPS = 0
	e.ETASeconds = -1
	if status == "completed" {
		e.Done = e.Total
		e.Phase = ""
		e.Error = ""
	}
	if saveErr := m.saveLocked(); saveErr != nil {
		e.Error = "保存传输记录失败：" + saveErr.Error()
	}
	m.mu.Unlock()
}

func (m *Manager) report(e *entry, done, total int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	if done != e.Done {
		e.lastProgress = now
	}
	if done < e.Done || e.Phase != "transferring" {
		e.samples = []rateSample{{now, done}}
	}
	e.Done, e.Total = done, total
	if len(e.samples) == 0 || now.Sub(e.samples[len(e.samples)-1].at) >= 250*time.Millisecond {
		e.samples = append(e.samples, rateSample{now, done})
	}
	m.pruneSamplesLocked(e, now)
	if m.store != "" && time.Since(m.lastSave) > 2*time.Second {
		if err := m.saveLocked(); err != nil {
			e.persistErr = err
			if e.cancel != nil {
				e.cancel()
			}
		}
	}
}
func (m *Manager) phase(e *entry, phase string, progress int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e.Phase != phase {
		e.samples = []rateSample{{time.Now(), e.Done}}
		e.lastProgress = time.Now()
	}
	e.Phase, e.PhaseProgress = phase, progress
}
func (m *Manager) pruneSamplesLocked(e *entry, now time.Time) {
	cutoff := now.Add(-5 * time.Second)
	i := 0
	for i+1 < len(e.samples) && e.samples[i+1].at.Before(cutoff) {
		i++
	}
	if i > 0 {
		e.samples = append([]rateSample(nil), e.samples[i:]...)
	}
}
func (m *Manager) snapshotLocked(e *entry, now time.Time) Task {
	t := e.Task
	t.SpeedBPS = 0
	t.ETASeconds = -1
	t.CanPause = false
	t.CanResume = false
	t.NeedsPassword = e.data.NeedsPassword && e.password == ""
	if t.Status == "running" && t.Phase == "transferring" && len(e.samples) > 0 && now.Sub(e.lastProgress) < 5*time.Second {
		m.pruneSamplesLocked(e, now)
		base := e.samples[0]
		seconds := now.Sub(base.at).Seconds()
		if seconds >= 0.25 && t.Done >= base.done {
			t.SpeedBPS = int64(float64(t.Done-base.done) / seconds)
		}
		if t.SpeedBPS > 0 && t.Total >= t.Done {
			t.ETASeconds = int64(math.Ceil(float64(t.Total-t.Done) / float64(t.SpeedBPS)))
		}
	}
	t.CanPause = t.Resumable && (t.Status == "running" || t.Status == "queued") && e.intent == ""
	if t.Resumable && !e.active && (t.Status == "paused" || t.Status == "failed") {
		if e.client != nil && e.parent != nil && e.parent.Err() == nil {
			t.CanResume = t.Direction != "upload" || e.client.UploadResume
			if !t.CanResume {
				t.ResumeReason = e.client.UploadResumeReason
			}
		} else {
			t.ResumeReason = "连接创建此任务的服务器和账号后可继续"
		}
	}
	return t
}
func (m *Manager) Snapshot() []Task {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	result := make([]Task, 0, len(m.tasks))
	for _, e := range m.tasks {
		result = append(result, m.snapshotLocked(e, now))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID > result[j].ID })
	return result
}

func (m *Manager) Pause(id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.tasks[id]
	if e == nil {
		return errors.New("任务不存在")
	}
	if !e.Resumable {
		return errors.New(e.ResumeReason)
	}
	if !e.active || (e.Status != "queued" && e.Status != "running") {
		return errors.New("该任务当前不能暂停")
	}
	e.intent = "pause"
	e.Status = "pausing"
	e.cancel()
	return nil
}
func (m *Manager) Resume(ctx context.Context, c *openlist.Client, id int64, password string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.tasks[id]
	if e == nil {
		return errors.New("任务不存在")
	}
	if m.closed || ctx.Err() != nil {
		return context.Canceled
	}
	if e.active || (e.Status != "paused" && e.Status != "failed") || !e.Resumable {
		return errors.New("该任务当前无法续传")
	}
	if e.Server != c.Address() || e.Account != c.User.Username {
		return errors.New("请连接创建此任务的服务器和账号")
	}
	if e.Direction == "upload" && !c.UploadResume {
		return errors.New(c.UploadResumeReason)
	}
	if password != "" {
		e.password = password
	}
	if e.data.NeedsPassword && e.password == "" {
		return errors.New("此下载需要目录访问密码")
	}
	e.parent, e.client = ctx, c
	if err := m.saveLocked(); err != nil {
		return err
	}
	m.startLocked(e)
	return nil
}
func (m *Manager) Cancel(id int64) {
	m.mu.Lock()
	e := m.tasks[id]
	if m.closed || e == nil || e.Status == "completed" || e.Status == "cancelled" {
		m.mu.Unlock()
		return
	}
	if e.active {
		e.intent = "cancel"
		e.Status = "cancelling"
		e.cancel()
		m.mu.Unlock()
		return
	}
	e.active = true
	e.Status = "cancelling"
	e.Error = ""
	m.wg.Add(1)
	m.mu.Unlock()
	go func() {
		defer m.wg.Done()
		m.cleanup(e)
		m.mu.Lock()
		defer m.mu.Unlock()
		e.active = false
		e.Status = "cancelled"
		m.saveLocked()
	}()
}
func (m *Manager) BindClient(ctx context.Context, c *openlist.Client) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.tasks {
		if c != nil && e.Server == c.Address() && e.Account == c.User.Username {
			e.client, e.parent = c, ctx
		} else {
			e.client = nil
			e.password = ""
		}
	}
}

// Keep live multipart checkpoints from expiring while the client is paused and
// still connected. Server restarts and offline expiration cannot be prevented.
func (m *Manager) KeepAlive() {
	type pending struct {
		ctx context.Context
		c   *openlist.Client
		id  string
	}
	m.mu.Lock()
	var requests []pending
	if !m.closed {
		for _, e := range m.tasks {
			if !e.active && e.Resumable && (e.Status == "paused" || e.Status == "failed") && e.data.UploadID != "" && e.client != nil && e.parent != nil && e.parent.Err() == nil {
				requests = append(requests, pending{e.parent, e.client, e.data.UploadID})
			}
		}
	}
	m.mu.Unlock()
	for _, request := range requests {
		request.c.MultipartStatus(request.ctx, request.id, "", 0)
	}
}
func (m *Manager) ClearFinished() error {
	m.mu.Lock()
	var removed []*entry
	for id, e := range m.tasks {
		if !e.active && (e.Status == "completed" || e.Status == "cancelled" || (e.Status == "failed" && !e.Resumable)) {
			delete(m.tasks, id)
			removed = append(removed, e)
		}
	}
	err := m.saveLocked()
	if err != nil {
		for _, e := range removed {
			m.tasks[e.ID] = e
		}
		m.mu.Unlock()
		return err
	}
	m.mu.Unlock()
	for _, e := range removed {
		if e.Status != "completed" {
			m.cleanup(e)
		}
	}
	return nil
}
func (m *Manager) Close() {
	m.mu.Lock()
	m.closed = true
	for _, e := range m.tasks {
		if e.active && e.cancel != nil {
			if e.intent == "" && e.Resumable {
				e.intent = "pause"
			}
			e.cancel()
		}
	}
	m.mu.Unlock()
	m.wg.Wait()
	m.mu.Lock()
	m.saveLocked()
	m.mu.Unlock()
}
func (m *Manager) RemainingCapacity() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return 0
	}
	return MaxTasks - len(m.tasks)
}

func (m *Manager) checkpoint(e *entry, update func(*jobData)) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	update(&e.data)
	return m.saveLocked()
}
func (m *Manager) disableResume(e *entry, reason string) {
	m.mu.Lock()
	e.Resumable = false
	e.ResumeReason = reason
	m.mu.Unlock()
}
func isSessionMissing(err error) bool {
	var api *openlist.APIError
	return errors.As(err, &api) && api.Code == 404
}
func cannotResume(message string) error {
	return fmt.Errorf("%w：%s", openlist.ErrCannotResume, message)
}
