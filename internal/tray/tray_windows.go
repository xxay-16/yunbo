package tray

import (
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/energye/systray"
)

func start(icon []byte, callbacks Callbacks) func() {
	var state atomic.Int32 // starting, ready, stopped
	var once sync.Once
	done := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(done)
		systray.Run(func() {
			systray.SetIcon(icon)
			systray.SetTooltip("云泊 · Yunbo — 点击打开，右键查看菜单")
			systray.SetOnClick(func(systray.IMenu) { go callbacks.Show() })
			systray.SetOnDClick(func(systray.IMenu) { go callbacks.Show() })
			systray.AddMenuItem("显示主窗口", "恢复云泊窗口").Click(func() { go callbacks.Show() })
			systray.AddMenuItem("收进后台", "保持连接并继续传输").Click(func() { go callbacks.Background() })
			systray.AddMenuItem("打开配置目录", "查看连接设置和续传记录").Click(func() { go callbacks.OpenConfig() })
			systray.AddSeparator()
			systray.AddMenuItem("退出云泊", "保存续传进度并完全退出").Click(func() { go callbacks.Quit() })
			if state.CompareAndSwap(0, 1) {
				callbacks.Ready()
			} else {
				systray.Quit()
			}
		}, func() {})
	}()
	return func() {
		once.Do(func() {
			if state.Swap(2) == 1 {
				systray.Quit()
			}
			select {
			case <-done:
			case <-time.After(2 * time.Second):
			}
		})
	}
}
