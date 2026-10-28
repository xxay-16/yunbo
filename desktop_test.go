package main

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
)

func desktopFixture() (*App, *atomic.Int32, *atomic.Int32) {
	a := NewApp()
	a.ctx = context.Background()
	a.closeToBackground = true
	hidden, quits := &atomic.Int32{}, &atomic.Int32{}
	a.windowHide = func(context.Context) { hidden.Add(1) }
	a.windowQuit = func(context.Context) { quits.Add(1) }
	a.windowShow = func(context.Context) {}
	return a, hidden, quits
}

func TestCloseToBackgroundRequiresUsableTray(t *testing.T) {
	a, hidden, quits := desktopFixture()
	if a.beforeClose(a.ctx) || hidden.Load() != 0 {
		t.Fatal("close was hidden without a recovery entry point")
	}
	if err := a.HideToTray(); err == nil {
		t.Fatal("explicit background action accepted before tray was ready")
	}
	a.trayReady.Store(true)
	if !a.beforeClose(a.ctx) || hidden.Load() != 1 || quits.Load() != 0 {
		t.Fatal("title-bar close did not hide the window while keeping the process alive")
	}
	if err := a.HideToTray(); err != nil {
		t.Fatal(err)
	}
	if hidden.Load() != 2 || quits.Load() != 0 {
		t.Fatal("background action terminated the application")
	}
}

func TestExplicitQuitBypassesCloseToTrayAndRunsOnce(t *testing.T) {
	a, hidden, quits := desktopFixture()
	a.trayReady.Store(true)
	var group sync.WaitGroup
	for i := 0; i < 20; i++ {
		group.Add(1)
		go func() { defer group.Done(); a.QuitApplication() }()
	}
	group.Wait()
	if quits.Load() != 1 || a.beforeClose(a.ctx) || hidden.Load() != 0 {
		t.Fatal("explicit quit was duplicated or intercepted as a background action")
	}
	if err := a.HideToTray(); err == nil {
		t.Fatal("window could hide after shutdown started")
	}
}

func TestMacApplicationQuitRemainsDistinctFromWindowClose(t *testing.T) {
	a, hidden, _ := desktopFixture()
	// Cocoa handles the title-bar close using HideWindowOnClose.
	a.closeToBackground = false
	a.trayReady.Store(true)
	if a.beforeClose(a.ctx) || hidden.Load() != 0 {
		t.Fatal("application-level quit was intercepted on macOS")
	}
}
