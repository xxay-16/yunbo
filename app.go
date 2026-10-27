package main

import (
	"context"
	"errors"

	"github.com/xxay-16/yunbo/internal/clientapp"
	"github.com/xxay-16/yunbo/internal/config"
	"github.com/xxay-16/yunbo/internal/openlist"
	"github.com/xxay-16/yunbo/internal/transfer"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx     context.Context
	service *clientapp.Service
	initErr error
}

func NewApp() *App { return &App{} }
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	profile, err := config.Location()
	if err != nil {
		a.initErr = err
		return
	}
	a.service = clientapp.New(ctx, profile)
}
func (a *App) shutdown(context.Context) {
	if a.service != nil {
		a.service.Close()
	}
}
func (a *App) ready() error {
	if a.initErr != nil {
		return a.initErr
	}
	if a.service == nil {
		return errors.New("应用尚未完成初始化")
	}
	if err := a.service.StorageError(); err != nil {
		return err
	}
	return nil
}
func (a *App) LoadProfile() (config.Profile, error) {
	if err := a.ready(); err != nil {
		return config.Profile{}, err
	}
	return a.service.LoadProfile()
}
func (a *App) Connect(req clientapp.ConnectRequest) (clientapp.Connection, error) {
	if err := a.ready(); err != nil {
		return clientapp.Connection{}, err
	}
	return a.service.Connect(req)
}
func (a *App) Disconnect() {
	if a.service != nil {
		a.service.Disconnect()
	}
}
func (a *App) List(dir, password string, page int) (openlist.Listing, error) {
	if err := a.ready(); err != nil {
		return openlist.Listing{}, err
	}
	return a.service.List(dir, password, page)
}
func (a *App) Mkdir(parent, name string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.service.Mkdir(parent, name)
}
func (a *App) Rename(p, name string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.service.Rename(p, name)
}
func (a *App) Remove(dir string, names []string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.service.Remove(dir, names)
}
func (a *App) CopyDownloadLink(p, password string) error {
	if err := a.ready(); err != nil {
		return err
	}
	link, err := a.service.DownloadLink(p, password)
	if err != nil {
		return err
	}
	return runtime.ClipboardSetText(a.ctx, link)
}
func (a *App) Upload(dir string) (clientapp.QueueResult, error) {
	if err := a.ready(); err != nil {
		return clientapp.QueueResult{}, err
	}
	files, err := runtime.OpenMultipleFilesDialog(a.ctx, runtime.OpenDialogOptions{Title: "选择要上传的文件"})
	if err != nil {
		return clientapp.QueueResult{}, err
	}
	return a.service.UploadPaths(dir, files)
}
func (a *App) UploadFolder(dir string) (clientapp.QueueResult, error) {
	if err := a.ready(); err != nil {
		return clientapp.QueueResult{}, err
	}
	folder, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{Title: "选择要上传的文件夹"})
	if err != nil || folder == "" {
		return clientapp.QueueResult{}, err
	}
	return a.service.UploadPaths(dir, []string{folder})
}
func (a *App) UploadDroppedFiles(dir string, paths []string) (clientapp.QueueResult, error) {
	if err := a.ready(); err != nil {
		return clientapp.QueueResult{}, err
	}
	return a.service.UploadPaths(dir, paths)
}
func (a *App) Download(dir string, names []string, password string) (clientapp.QueueResult, error) {
	if err := a.ready(); err != nil {
		return clientapp.QueueResult{}, err
	}
	if len(names) == 0 {
		return clientapp.QueueResult{}, errors.New("请选择要下载的文件")
	}
	for _, name := range names {
		if err := transfer.SafeLocalName(name); err != nil {
			return clientapp.QueueResult{}, err
		}
	}
	localDir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{Title: "选择下载保存目录（同名文件会自动编号）"})
	if err != nil || localDir == "" {
		return clientapp.QueueResult{}, err
	}
	return a.service.DownloadFiles(dir, names, password, localDir)
}
func (a *App) Transfers() []transfer.Task {
	if a.service == nil {
		return []transfer.Task{}
	}
	return a.service.Transfers()
}
func (a *App) CancelTransfer(id int64) {
	if a.service != nil {
		a.service.CancelTransfer(id)
	}
}
func (a *App) ClearFinishedTransfers() error {
	if a.service != nil {
		return a.service.ClearFinishedTransfers()
	}
	return a.ready()
}
func (a *App) PauseTransfer(id int64) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.service.PauseTransfer(id)
}
func (a *App) ResumeTransfer(id int64, password string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.service.ResumeTransfer(id, password)
}
