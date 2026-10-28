//go:build !wails

package main

import (
	"context"
	_ "embed"
	"log"
	"os"
	"path/filepath"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/dialog"
	"github.com/xxay-16/yunbo/internal/clientapp"
	"github.com/xxay-16/yunbo/internal/instance"
	"github.com/xxay-16/yunbo/internal/nativeui"
)

//go:embed build/appicon.png
var nativeIcon []byte

func main() {
	application := app.NewWithID("io.github.xxay16.yunbo.fyne")
	application.SetIcon(fyne.NewStaticResource("yunbo.png", nativeIcon))
	application.Settings().SetTheme(nativeui.CompactTheme{})
	directory, err := os.UserConfigDir()
	if err != nil {
		log.Fatal(err)
	}
	profile := filepath.Join(directory, "yunbo-fyne", "profile.json")
	release, err := instance.Acquire(profile)
	if err != nil {
		window := application.NewWindow("云泊 · Fyne")
		dialog.ShowInformation("无法启动", err.Error(), window)
		window.Resize(fyne.NewSize(520, 220))
		window.ShowAndRun()
		return
	}
	defer release()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	service := clientapp.New(ctx, profile)
	ui := nativeui.New(application, service, profile)
	defer ui.Close()
	ui.Window.ShowAndRun()
}
