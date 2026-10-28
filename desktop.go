//go:build wails

package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"github.com/xxay-16/yunbo/internal/config"
	"github.com/xxay-16/yunbo/internal/tray"
)

func (a *App) domReady(ctx context.Context) {
	a.trayOnce.Do(func() {
		stop := tray.Start(trayIcon, tray.Callbacks{
			Ready: func() {
				if !a.exiting.Load() {
					a.trayReady.Store(true)
				}
			},
			Show:       a.ShowMainWindow,
			Background: func() { _ = a.HideToTray() },
			OpenConfig: func() {
				if err := a.OpenConfigDirectory(); err != nil {
					log.Printf("打开配置目录失败：%v", err)
				}
			},
			Quit: a.QuitApplication,
		})
		a.desktopMu.Lock()
		if a.exiting.Load() {
			a.desktopMu.Unlock()
			stop()
			return
		}
		a.stopTray = stop
		a.desktopMu.Unlock()
	})
}

func (a *App) beforeClose(ctx context.Context) bool {
	// Cocoa hides title-bar closes natively; Cmd+Q and Dock Quit still exit.
	if a.closeToBackground && !a.exiting.Load() && a.trayReady.Load() {
		a.windowHide(ctx)
		return true
	}
	return false
}

func (a *App) HideToTray() error {
	if a.ctx == nil || a.exiting.Load() {
		return errors.New("应用正在退出")
	}
	if !a.trayReady.Load() {
		return errors.New("系统托盘尚未就绪，暂时不能收进后台")
	}
	a.windowHide(a.ctx)
	return nil
}

func (a *App) ShowMainWindow() {
	if a.ctx != nil && !a.exiting.Load() {
		a.windowShow(a.ctx)
	}
}

func (a *App) QuitApplication() {
	if a.ctx != nil && a.exiting.CompareAndSwap(false, true) {
		a.windowQuit(a.ctx)
	}
}

func (a *App) OpenConfigDirectory() error {
	file, err := config.Location()
	if err != nil {
		return err
	}
	dir := filepath.Dir(file)
	if err = os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	program := "xdg-open"
	switch goruntime.GOOS {
	case "windows":
		program = "explorer.exe"
	case "darwin":
		program = "open"
	}
	command := exec.Command(program, dir)
	if err = command.Start(); err != nil {
		return err
	}
	go func() { _ = command.Wait() }()
	return nil
}

func showWindow(ctx context.Context) {
	runtime.WindowUnminimise(ctx)
	runtime.WindowShow(ctx)
}
