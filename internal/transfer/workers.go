package transfer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/xxay-16/yunbo/internal/openlist"
)

func (m *Manager) Upload(ctx context.Context, c *openlist.Client, local, remote string) (int64, error) {
	stat, err := os.Lstat(local)
	if err != nil {
		return 0, err
	}
	if !filepath.IsAbs(local) || !stat.Mode().IsRegular() {
		return 0, errors.New("只能上传本机的普通文件")
	}
	resumable := c.UploadResume && stat.Size() > 0
	reason := c.UploadResumeReason
	if !resumable && reason == "" {
		reason = "当前上传使用普通接口，不支持断点继续"
	}
	return m.insert(&entry{Task: Task{Name: filepath.Base(local), Direction: "upload", RemotePath: openlist.CleanPath(remote), LocalPath: local, Total: stat.Size(), Server: c.Address(), Account: c.User.Username, Resumable: resumable, ResumeReason: reason}, parent: ctx, client: c, data: jobData{Kind: "upload", SourceSize: stat.Size(), SourceModified: stat.ModTime().UnixNano()}})
}

func (m *Manager) Download(ctx context.Context, c *openlist.Client, remote, name, password, dir string) (int64, error) {
	if err := SafeLocalName(name); err != nil {
		return 0, err
	}
	if !filepath.IsAbs(dir) {
		return 0, errors.New("下载目录必须是本机绝对路径")
	}
	return m.insert(&entry{Task: Task{Name: name, Direction: "download", RemotePath: openlist.CleanPath(remote), LocalPath: filepath.Join(dir, name), Server: c.Address(), Account: c.User.Username, Resumable: true}, parent: ctx, client: c, password: password, data: jobData{Kind: "download", DownloadDir: filepath.Clean(dir), NeedsPassword: password != ""}})
}

func (m *Manager) execute(e *entry, ctx context.Context, progress openlist.Progress) (string, error) {
	m.mu.Lock()
	data, c, password, local, remote := e.data, e.client, e.password, e.LocalPath, e.RemotePath
	m.mu.Unlock()
	if c == nil {
		return "", errors.New("请先连接原服务器和账号")
	}
	if data.Kind == "upload" {
		return m.upload(e, ctx, c, local, remote, data, progress)
	}
	return m.download(e, ctx, c, remote, password, data, progress)
}

func (m *Manager) download(e *entry, ctx context.Context, c *openlist.Client, remote, password string, data jobData, progress openlist.Progress) (string, error) {
	m.phase(e, "checking", 0)
	var f *os.File
	var err error
	if data.Temp == "" {
		f, err = os.CreateTemp(data.DownloadDir, ".yunbo-*.part")
		if err != nil {
			return "", err
		}
		data.Temp = f.Name()
		if err = m.checkpoint(e, func(d *jobData) { d.Temp = data.Temp }); err != nil {
			f.Close()
			os.Remove(data.Temp)
			return "", err
		}
	} else {
		stat, statErr := os.Lstat(data.Temp)
		if statErr != nil || !stat.Mode().IsRegular() {
			return "", cannotResume("本地续传文件已丢失或不是普通文件，请重新下载")
		}
		f, err = os.OpenFile(data.Temp, os.O_RDWR, 0600)
		if err != nil {
			return "", err
		}
	}
	stat, err := f.Stat()
	if err != nil {
		f.Close()
		return "", err
	}
	progress(stat.Size(), data.Download.Total)
	m.phase(e, "transferring", 0)
	err = c.DownloadResumable(ctx, remote, password, f, &data.Download, func(state openlist.DownloadState) error {
		m.mu.Lock()
		e.Resumable = state.Resumable
		if !state.Resumable {
			e.ResumeReason = "源服务器不支持 Range 下载，不能暂停续传"
		} else {
			e.ResumeReason = ""
		}
		m.mu.Unlock()
		return m.checkpoint(e, func(d *jobData) { d.Download = state })
	}, progress)
	if syncErr := f.Sync(); err == nil {
		err = syncErr
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return "", err
	}
	if err = ctx.Err(); err != nil {
		return "", err
	}
	m.phase(e, "committing", 0)
	result, err := publish(data.Temp, data.DownloadDir, e.Name)
	if err == nil {
		m.checkpoint(e, func(d *jobData) {
			d.Temp = ""
			e.LocalPath = result
			e.Status = "completed"
			e.Done = e.Total
		})
	}
	return result, err
}

func (m *Manager) upload(e *entry, ctx context.Context, c *openlist.Client, local, remote string, data jobData, progress openlist.Progress) (string, error) {
	m.phase(e, "checking", 0)
	if data.UploadID != "" {
		snapshot, err := c.MultipartStatus(ctx, data.UploadID, remote, data.SourceSize)
		if isSessionMissing(err) {
			return "", cannotResume("服务端续传会话已过期或因重启丢失，需要重新上传")
		}
		if err != nil {
			return "", err
		}
		if err = validateMultipart(snapshot, c, remote, data.SourceSize, data.UploadID); err != nil {
			return "", err
		}
		if snapshot.State == "completed" {
			progress(data.SourceSize, data.SourceSize)
			return local, nil
		}
	}
	f, err := openSource(local)
	if err != nil {
		return "", err
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return "", err
	}
	if !stat.Mode().IsRegular() || stat.Size() != data.SourceSize || stat.ModTime().UnixNano() != data.SourceModified {
		return "", cannotResume("本机源文件已变化，请重新选择上传")
	}
	m.mu.Lock()
	resumable := e.Resumable
	m.mu.Unlock()
	if !resumable {
		m.phase(e, "transferring", 0)
		err = c.UploadFile(ctx, f, remote, progress)
		return local, err
	}
	hash, err := hashSource(ctx, f, stat.Size(), func(p int) { m.phase(e, "checking", p) })
	if err != nil {
		return "", err
	}
	if data.SHA256 != "" && hash != data.SHA256 {
		return "", cannotResume("本机源文件内容已变化，已阻止混合上传")
	}
	data.SHA256 = hash
	if err = m.checkpoint(e, func(d *jobData) { d.SHA256 = hash }); err != nil {
		return "", err
	}
	var snapshot openlist.MultipartSnapshot
	if data.UploadID != "" {
		snapshot, err = c.MultipartStatus(ctx, data.UploadID, remote, stat.Size())
		if isSessionMissing(err) {
			return "", cannotResume("服务端续传会话已过期或因重启丢失，需要重新上传")
		}
	} else {
		snapshot, err = c.MultipartInit(ctx, remote, stat.Size(), stat.ModTime(), hash)
		if errors.Is(err, openlist.ErrMultipartUnavailable) {
			m.disableResume(e, "服务器已关闭分片上传，当前任务改用普通上传")
			f.Seek(0, io.SeekStart)
			m.phase(e, "transferring", 0)
			return local, c.UploadFile(ctx, f, remote, progress)
		}
		if err == nil {
			data.UploadID = snapshot.ID
			if err = m.checkpoint(e, func(d *jobData) { d.UploadID = data.UploadID }); err != nil {
				return "", err
			}
		}
	}
	if err != nil {
		return "", err
	}
	if err = validateMultipart(snapshot, c, remote, stat.Size(), data.UploadID); err != nil {
		return "", err
	}
	progress(snapshot.ReceivedBytes, stat.Size())
	if snapshot.State == "completed" {
		return local, nil
	}
	m.phase(e, "transferring", 0)
	for index := 0; index < snapshot.TotalChunks; index++ {
		if err = ctx.Err(); err != nil {
			return "", err
		}
		if receivedChunk(snapshot, index) {
			continue
		}
		offset := int64(index) * snapshot.ChunkSize
		length := min(snapshot.ChunkSize, stat.Size()-offset)
		base := snapshot.ReceivedBytes
		for attempt := 0; ; attempt++ {
			next, chunkErr := c.MultipartChunk(ctx, snapshot.ID, index, io.NewSectionReader(f, offset, length), length, progress, base, stat.Size())
			if chunkErr == nil {
				snapshot = next
				break
			}
			var apiErr *openlist.APIError
			if !errors.As(chunkErr, &apiErr) || (apiErr.Code != 409 && apiErr.Code != 429) || attempt >= 119 {
				return "", chunkErr
			}
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(min(time.Second, time.Duration(attempt+1)*200*time.Millisecond)):
			}
			snapshot, err = c.MultipartStatus(ctx, data.UploadID, remote, stat.Size())
			if err != nil {
				return "", err
			}
			if err = validateMultipart(snapshot, c, remote, stat.Size(), data.UploadID); err != nil {
				return "", err
			}
			if receivedChunk(snapshot, index) {
				break
			}
			base = snapshot.ReceivedBytes
		}
		if err = validateMultipart(snapshot, c, remote, stat.Size(), data.UploadID); err != nil {
			return "", err
		}
		progress(snapshot.ReceivedBytes, stat.Size())
		if snapshot.State == "completed" {
			return local, nil
		}
	}
	m.phase(e, "committing", 0)
	snapshot, err = c.MultipartComplete(ctx, data.UploadID)
	if err != nil {
		return "", err
	}
	if err = validateMultipart(snapshot, c, remote, stat.Size(), data.UploadID); err != nil {
		return "", err
	}
	if snapshot.State != "completed" {
		return "", errors.New("服务端尚未确认上传完成，可稍后继续查询")
	}
	progress(stat.Size(), stat.Size())
	return local, nil
}

func hashSource(ctx context.Context, f *os.File, size int64, report func(int)) (string, error) {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	hash := sha256.New()
	buf := make([]byte, 1<<20)
	done := int64(0)
	last := -1
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		n, err := f.Read(buf)
		if n > 0 {
			hash.Write(buf[:n])
			done += int64(n)
			p := 0
			if size > 0 {
				p = int(float64(done) / float64(size) * 100)
			}
			if p != last {
				report(p)
				last = p
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
	}
	if done != size {
		return "", cannotResume("读取源文件时长度发生变化")
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func receivedChunk(s openlist.MultipartSnapshot, index int) bool {
	i := sort.Search(len(s.Received), func(i int) bool { return s.Received[i][1] >= index })
	return i < len(s.Received) && s.Received[i][0] <= index
}
func validateMultipart(s openlist.MultipartSnapshot, c *openlist.Client, remote string, size int64, id string) error {
	if s.ID == "" || s.ID != id || s.Size != size || s.Path != path.Join("/", c.User.BasePath, openlist.CleanPath(remote)) || s.ChunkSize <= 0 || s.ChunkSize > 64<<20 || s.TotalChunks != int(1+(size-1)/s.ChunkSize) {
		return cannotResume("分片会话元数据与源文件不一致")
	}
	if s.State != "receiving" && s.State != "completed" {
		return cannotResume("服务端上传状态为 " + s.State + "，数据已不可继续：" + s.Error)
	}
	last := -1
	received := int64(0)
	for _, span := range s.Received {
		if span[0] < 0 || span[0] > span[1] || span[1] >= s.TotalChunks || span[0] <= last {
			return cannotResume("服务端返回了无效分片范围")
		}
		start := int64(span[0]) * s.ChunkSize
		end := min(size, int64(span[1]+1)*s.ChunkSize)
		received += end - start
		last = span[1]
	}
	if received != s.ReceivedBytes {
		return cannotResume("服务端确认的分片字节数不一致")
	}
	if s.State == "completed" && received != size {
		return cannotResume("服务端未确认完整文件")
	}
	return nil
}

func (m *Manager) cleanup(e *entry) {
	m.mu.Lock()
	data, c := e.data, e.client
	m.mu.Unlock()
	var cleanupErr error
	if data.Temp != "" && filepath.Dir(data.Temp) == filepath.Clean(data.DownloadDir) && strings.HasPrefix(filepath.Base(data.Temp), ".yunbo-") && strings.HasSuffix(data.Temp, ".part") {
		if err := os.Remove(data.Temp); err != nil && !errors.Is(err, os.ErrNotExist) {
			cleanupErr = err
		}
	}
	if data.UploadID != "" && c != nil {
		if err := c.MultipartAbort(context.Background(), data.UploadID); err != nil && !isSessionMissing(err) {
			cleanupErr = err
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if cleanupErr != nil {
		e.Error = "任务已取消，部分临时数据清理失败：" + cleanupErr.Error()
	} else {
		e.data.Temp = ""
		e.data.UploadID = ""
	}
}
