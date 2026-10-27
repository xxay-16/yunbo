package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend
var assets embed.FS

func main() {
	app := NewApp()
	if err := wails.Run(&options.App{
		Title: "OpenList Desk", Width: 1280, Height: 820, MinWidth: 940, MinHeight: 640,
		BackgroundColour: options.NewRGB(246, 248, 251),
		AssetServer:      &assetserver.Options{Assets: assets},
		OnStartup:        app.startup, OnShutdown: app.shutdown, Bind: []interface{}{app},
	}); err != nil {
		log.Fatal(err)
	}
}
