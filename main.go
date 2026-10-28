//go:build wails

package main

import (
	"crypto/sha256"
	"embed"
	"fmt"
	"log"
	"runtime"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/xxay-16/yunbo/internal/config"
)

//go:embed all:frontend
var assets embed.FS

//go:embed build/windows/icon.ico
var trayIcon []byte

func main() {
	app := NewApp()
	profile, _ := config.Location()
	key := sha256.Sum256([]byte(profile))
	if err := wails.Run(&options.App{
		Title: "云泊 · Yunbo", Width: 1280, Height: 820, MinWidth: 940, MinHeight: 640,
		BackgroundColour: options.NewRGB(246, 248, 251),
		AssetServer:      &assetserver.Options{Assets: assets},
		DragAndDrop:      &options.DragAndDrop{EnableFileDrop: true},
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId:               fmt.Sprintf("yunbo-%x", key[:16]),
			OnSecondInstanceLaunch: func(options.SecondInstanceData) { app.ShowMainWindow() },
		},
		HideWindowOnClose: runtime.GOOS == "darwin",
		OnStartup:         app.startup, OnDomReady: app.domReady, OnBeforeClose: app.beforeClose,
		OnShutdown: app.shutdown, Bind: []interface{}{app},
	}); err != nil {
		log.Fatal(err)
	}
}
