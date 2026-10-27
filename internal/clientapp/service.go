// Package clientapp contains desktop-independent connection and transfer logic.
package clientapp

import (
	"context"
	"errors"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/xxay-16/yunbo/internal/config"
	"github.com/xxay-16/yunbo/internal/openlist"
	"github.com/xxay-16/yunbo/internal/transfer"
)

type Service struct {
	ctx         context.Context
	stop        context.CancelFunc
	profilePath string
	mu          sync.RWMutex
	connectMu   sync.Mutex
	uploadMu    sync.Mutex
	client      *openlist.Client
	session     context.Context
	cancel      context.CancelFunc
	closed      bool
	transfers   *transfer.Manager
	storageErr  error
}

type ConnectRequest struct {
	config.Profile
	Password          string `json:"password"`
	Token             string `json:"token"`
	OTP               string `json:"otp"`
	DirectoryPassword string `json:"directory_password"`
}

type Connection struct {
	Profile            config.Profile   `json:"profile"`
	User               openlist.User    `json:"user"`
	Listing            openlist.Listing `json:"listing"`
	Warning            string           `json:"warning"`
	UploadResume       bool             `json:"upload_resume"`
	UploadResumeReason string           `json:"upload_resume_reason"`
}

type QueueResult struct {
	Count   int      `json:"count"`
	Folders int      `json:"folders"`
	Errors  []string `json:"errors"`
}

func New(ctx context.Context, profilePath string) *Service {
	ctx, cancel := context.WithCancel(ctx)
	manager, err := transfer.NewPersistent(filepath.Join(filepath.Dir(profilePath), "transfers.json"))
	if err != nil {
		manager = transfer.New()
	}
	service := &Service{ctx: ctx, stop: cancel, profilePath: profilePath, transfers: manager, storageErr: err}
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				service.transfers.KeepAlive()
			}
		}
	}()
	return service
}

func (s *Service) StorageError() error { return s.storageErr }

func (s *Service) LoadProfile() (config.Profile, error) {
	legacy := filepath.Join(filepath.Dir(filepath.Dir(s.profilePath)), "openlist-desk", "profile.json")
	return config.LoadWithFallback(s.profilePath, legacy)
}

func (s *Service) Connect(req ConnectRequest) (Connection, error) {
	s.connectMu.Lock()
	defer s.connectMu.Unlock()
	if s.storageErr != nil {
		return Connection{}, s.storageErr
	}
	if req.AuthMode != "password" && req.AuthMode != "token" && req.AuthMode != "guest" {
		return Connection{}, errors.New("请选择登录方式")
	}
	if req.AuthMode == "token" && strings.TrimSpace(req.Token) == "" {
		return Connection{}, errors.New("请输入访问令牌")
	}
	token := ""
	if req.AuthMode == "token" {
		token = req.Token
	}
	c, err := openlist.New(req.URL, token)
	if err != nil {
		return Connection{}, err
	}
	ctx, cancel := context.WithTimeout(s.ctx, 35*time.Second)
	defer cancel()
	if req.AuthMode == "password" {
		if strings.TrimSpace(req.Username) == "" {
			return Connection{}, errors.New("请输入用户名")
		}
		if err = c.Login(ctx, strings.TrimSpace(req.Username), req.Password, req.OTP); err != nil {
			return Connection{}, err
		}
	}
	if err = c.LoadUser(ctx); err != nil {
		return Connection{}, err
	}
	req.StartPath = openlist.CleanPath(req.StartPath)
	listing, err := c.List(ctx, req.StartPath, req.DirectoryPassword, 1)
	if err != nil {
		return Connection{}, err
	}
	c.ProbeUploadResume(ctx)
	profile := req.Profile
	profile.URL, profile.Username = c.Address(), c.User.Username
	s.mu.Lock()
	if s.closed || s.ctx.Err() != nil {
		s.mu.Unlock()
		return Connection{}, context.Canceled
	}
	if s.cancel != nil {
		s.cancel()
	}
	s.client = c
	s.session, s.cancel = context.WithCancel(s.ctx)
	session := s.session
	s.mu.Unlock()
	s.transfers.BindClient(session, c)
	result := Connection{Profile: profile, User: c.User, Listing: listing, UploadResume: c.UploadResume, UploadResumeReason: c.UploadResumeReason}
	if err = config.Save(s.profilePath, profile); err != nil {
		result.Warning = "连接成功，但无法保存连接信息：" + err.Error()
	}
	return result, nil
}

func (s *Service) Disconnect() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
	}
	s.client = nil
	s.transfers.BindClient(nil, nil)
}

func (s *Service) Close() {
	s.mu.Lock()
	s.closed = true
	s.stop()
	if s.cancel != nil {
		s.cancel()
	}
	s.client = nil
	s.mu.Unlock()
	s.transfers.Close()
}

func (s *Service) connected() (*openlist.Client, context.Context, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.storageErr != nil {
		return nil, nil, s.storageErr
	}
	if s.client == nil || s.closed {
		return nil, nil, errors.New("请先连接 OpenList 服务器")
	}
	return s.client, s.session, nil
}

func (s *Service) List(dir, password string, page int) (openlist.Listing, error) {
	c, ctx, err := s.connected()
	if err != nil {
		return openlist.Listing{}, err
	}
	return c.List(ctx, dir, password, page)
}
func (s *Service) Mkdir(parent, name string) error {
	c, ctx, err := s.connected()
	if err != nil {
		return err
	}
	return c.Mkdir(ctx, parent, name)
}
func (s *Service) Rename(p, name string) error {
	c, ctx, err := s.connected()
	if err != nil {
		return err
	}
	return c.Rename(ctx, p, name)
}
func (s *Service) Remove(dir string, names []string) error {
	c, ctx, err := s.connected()
	if err != nil {
		return err
	}
	return c.Remove(ctx, dir, names)
}
func (s *Service) DownloadLink(p, password string) (string, error) {
	c, ctx, err := s.connected()
	if err != nil {
		return "", err
	}
	link, _, err := c.DownloadLink(ctx, p, password)
	return link, err
}

// UploadPaths is shared by native file pickers and Wails file-drop callbacks.
// Preflight checks finish before any cloud directory or transfer is created.
func (s *Service) UploadPaths(dir string, paths []string) (QueueResult, error) {
	s.uploadMu.Lock()
	defer s.uploadMu.Unlock()
	c, ctx, err := s.connected()
	if err != nil {
		return QueueResult{}, err
	}
	plan, err := planUploads(ctx, paths, dir, s.transfers.RemainingCapacity())
	if err != nil {
		return QueueResult{}, err
	}
	result := QueueResult{}
	var failedDirs []string
	for _, item := range plan {
		if err = ctx.Err(); err != nil {
			return result, err
		}
		skip := false
		for _, failed := range failedDirs {
			if item.remote == failed || strings.HasPrefix(item.remote, failed+"/") {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		if item.isDir {
			err = c.Mkdir(ctx, path.Dir(item.remote), path.Base(item.remote))
			if err != nil {
				result.Errors = append(result.Errors, item.remote+"："+err.Error())
				failedDirs = append(failedDirs, item.remote)
			} else {
				result.Folders++
			}
			continue
		}
		_, err = s.transfers.Upload(ctx, c, item.local, item.remote)
		if err != nil {
			result.Errors = append(result.Errors, filepath.Base(item.local)+"："+err.Error())
		} else {
			result.Count++
		}
	}
	return result, nil
}

func (s *Service) DownloadFiles(dir string, names []string, password, localDir string) (QueueResult, error) {
	c, ctx, err := s.connected()
	if err != nil {
		return QueueResult{}, err
	}
	if len(names) == 0 {
		return QueueResult{}, errors.New("请选择要下载的文件")
	}
	stat, err := os.Stat(localDir)
	if err != nil {
		return QueueResult{}, err
	}
	if !stat.IsDir() {
		return QueueResult{}, errors.New("请选择本机下载目录")
	}
	for _, name := range names {
		if err = transfer.SafeLocalName(name); err != nil {
			return QueueResult{}, err
		}
	}
	result := QueueResult{}
	for _, name := range names {
		_, err = s.transfers.Download(ctx, c, path.Join(openlist.CleanPath(dir), name), name, password, localDir)
		if err != nil {
			result.Errors = append(result.Errors, name+"："+err.Error())
		} else {
			result.Count++
		}
	}
	return result, nil
}

func (s *Service) Transfers() []transfer.Task    { return s.transfers.Snapshot() }
func (s *Service) CancelTransfer(id int64)       { s.transfers.Cancel(id) }
func (s *Service) ClearFinishedTransfers() error { return s.transfers.ClearFinished() }
func (s *Service) PauseTransfer(id int64) error  { return s.transfers.Pause(id) }
func (s *Service) ResumeTransfer(id int64, password string) error {
	c, ctx, err := s.connected()
	if err != nil {
		return err
	}
	return s.transfers.Resume(ctx, c, id, password)
}
