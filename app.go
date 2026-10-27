package main

import (
	"context"
	"errors"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"openlist-desk/internal/config"
	"openlist-desk/internal/openlist"
	"openlist-desk/internal/transfer"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx       context.Context
	mu        sync.RWMutex
	connectMu sync.Mutex
	client    *openlist.Client
	session   context.Context
	cancel    context.CancelFunc
	transfers *transfer.Manager
}

type ConnectRequest struct {
	config.Profile
	Password          string `json:"password"`
	Token             string `json:"token"`
	OTP               string `json:"otp"`
	DirectoryPassword string `json:"directory_password"`
}

type Connection struct {
	Profile config.Profile   `json:"profile"`
	User    openlist.User    `json:"user"`
	Listing openlist.Listing `json:"listing"`
	Warning string           `json:"warning"`
}

type QueueResult struct {
	Count  int      `json:"count"`
	Errors []string `json:"errors"`
}

func NewApp() *App                         { return &App{transfers: transfer.New()} }
func (a *App) startup(ctx context.Context) { a.ctx = ctx }
func (a *App) shutdown(context.Context)    { a.Disconnect(); a.transfers.Close() }

func (a *App) LoadProfile() (config.Profile, error) {
	file, err := config.Location()
	if err != nil {
		return config.Profile{}, err
	}
	return config.Load(file)
}

func (a *App) Connect(req ConnectRequest) (Connection, error) {
	a.connectMu.Lock()
	defer a.connectMu.Unlock()
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
	if req.AuthMode == "password" {
		if strings.TrimSpace(req.Username) == "" {
			return Connection{}, errors.New("请输入用户名")
		}
		if err = c.Login(a.ctx, strings.TrimSpace(req.Username), req.Password, req.OTP); err != nil {
			return Connection{}, err
		}
	}
	if err = c.LoadUser(a.ctx); err != nil {
		return Connection{}, err
	}
	req.StartPath = openlist.CleanPath(req.StartPath)
	listing, err := c.List(a.ctx, req.StartPath, req.DirectoryPassword, 1)
	if err != nil {
		return Connection{}, err
	}
	profile := req.Profile
	profile.URL = c.Address()
	profile.Username = c.User.Username
	a.mu.Lock()
	if a.cancel != nil {
		a.cancel()
	}
	a.client = c
	a.session, a.cancel = context.WithCancel(a.ctx)
	a.mu.Unlock()
	result := Connection{Profile: profile, User: c.User, Listing: listing}
	file, err := config.Location()
	if err == nil {
		err = config.Save(file, profile)
	}
	if err != nil {
		result.Warning = "连接成功，但无法保存连接信息：" + err.Error()
	}
	return result, nil
}

func (a *App) Disconnect() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cancel != nil {
		a.cancel()
	}
	a.client = nil
}

func (a *App) connected() (*openlist.Client, context.Context, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.client == nil {
		return nil, nil, errors.New("请先连接 OpenList 服务器")
	}
	return a.client, a.session, nil
}

func (a *App) List(dir, password string, page int) (openlist.Listing, error) {
	c, ctx, err := a.connected()
	if err != nil {
		return openlist.Listing{}, err
	}
	return c.List(ctx, dir, password, page)
}

func (a *App) Mkdir(parent, name string) error {
	c, ctx, err := a.connected()
	if err != nil {
		return err
	}
	return c.Mkdir(ctx, parent, name)
}

func (a *App) Rename(p, name string) error {
	c, ctx, err := a.connected()
	if err != nil {
		return err
	}
	return c.Rename(ctx, p, name)
}

func (a *App) Remove(dir string, names []string) error {
	c, ctx, err := a.connected()
	if err != nil {
		return err
	}
	return c.Remove(ctx, dir, names)
}

func (a *App) CopyDownloadLink(p, password string) error {
	c, ctx, err := a.connected()
	if err != nil {
		return err
	}
	link, _, err := c.DownloadLink(ctx, p, password)
	if err != nil {
		return err
	}
	return runtime.ClipboardSetText(a.ctx, link)
}

func (a *App) Upload(dir string) (QueueResult, error) {
	c, ctx, err := a.connected()
	if err != nil {
		return QueueResult{}, err
	}
	files, err := runtime.OpenMultipleFilesDialog(a.ctx, runtime.OpenDialogOptions{Title: "选择要上传的文件"})
	if err != nil {
		return QueueResult{}, err
	}
	result := QueueResult{Errors: []string{}}
	for _, local := range files {
		_, err := a.transfers.Upload(ctx, c, local, path.Join(openlist.CleanPath(dir), filepath.Base(local)))
		if err != nil {
			result.Errors = append(result.Errors, filepath.Base(local)+"："+err.Error())
		} else {
			result.Count++
		}
	}
	return result, nil
}

func (a *App) Download(dir string, names []string, password string) (QueueResult, error) {
	c, ctx, err := a.connected()
	if err != nil {
		return QueueResult{}, err
	}
	if len(names) == 0 {
		return QueueResult{}, errors.New("请选择要下载的文件")
	}
	for _, name := range names {
		if err = transfer.SafeLocalName(name); err != nil {
			return QueueResult{}, err
		}
	}
	localDir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{Title: "选择下载保存目录（同名文件会自动编号）"})
	if err != nil || localDir == "" {
		return QueueResult{Errors: []string{}}, err
	}
	result := QueueResult{Errors: []string{}}
	for _, name := range names {
		_, err = a.transfers.Download(ctx, c, path.Join(openlist.CleanPath(dir), name), name, password, localDir)
		if err != nil {
			result.Errors = append(result.Errors, name+"："+err.Error())
		} else {
			result.Count++
		}
	}
	return result, nil
}

func (a *App) Transfers() []transfer.Task { return a.transfers.Snapshot() }
func (a *App) CancelTransfer(id int64)    { a.transfers.Cancel(id) }
func (a *App) ClearFinishedTransfers()    { a.transfers.ClearFinished() }
