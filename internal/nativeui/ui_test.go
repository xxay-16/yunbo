package nativeui

import (
	"context"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/xxay-16/yunbo/internal/clientapp"
	"github.com/xxay-16/yunbo/internal/openlist"
	"github.com/xxay-16/yunbo/internal/transfer"
)

func testUI(t *testing.T) *UI {
	t.Helper()
	application := test.NewApp()
	application.Settings().SetTheme(CompactTheme{})
	service := clientapp.New(context.Background(), filepath.Join(t.TempDir(), "profile.json"))
	u := New(application, service, filepath.Join(t.TempDir(), "profile.json"))
	// Stop the live poller: this test drives the widgets synchronously.
	u.Close()
	t.Cleanup(func() { u.Window.SetCloseIntercept(nil); u.Window.Close(); application.Quit() })
	return u
}

func TestSelectionSurvivesCellRecyclingAndReadOnlyPermissions(t *testing.T) {
	u := testUI(t)
	u.connection = &clientapp.Connection{User: openlist.User{Role: 2}}
	u.listing = openlist.Listing{Write: true, Total: 2, Content: []openlist.File{{Name: "目录", IsDir: true}, {Name: "report.txt", Size: 42}}}
	u.rebuildFiles()
	cell := u.fileTable.CreateCell()
	u.fileTable.UpdateCell(widget.TableCellID{Row: 0, Col: 0}, cell)
	check := cell.(*fyne.Container).Objects[0].(*widget.Check)
	check.SetChecked(true)
	u.fileTable.UpdateCell(widget.TableCellID{Row: 1, Col: 0}, cell)
	if !u.selected["目录"] || u.selected["report.txt"] {
		t.Fatalf("recycling changed selection: %v", u.selected)
	}
	check.SetChecked(true)
	if !u.selected["report.txt"] {
		t.Fatal("recycled checkbox retained its previous file callback")
	}
	u.listing.Write = false
	u.updateControls()
	if !u.upload.Disabled() || !u.rename.Disabled() || !u.remove.Disabled() {
		t.Fatal("read-only directory enabled mutation controls")
	}
}

func TestPasswordInheritanceAndSorting(t *testing.T) {
	u := &UI{passwords: map[string]string{"/": "root", "/data": "private"}}
	if u.passwordFor("/data/sub") != "private" || u.passwordFor("/database") != "root" {
		t.Fatal("directory password escaped its path boundary")
	}
	source := []openlist.File{{Name: "report.TXT", Size: 8}, {Name: "Archive", IsDir: true}, {Name: "notes.txt", Size: 2}}
	files := visibleFiles(source, "", "size", false)
	if !files[0].IsDir || files[1].Name != "report.TXT" || source[0].Name != "report.TXT" {
		t.Fatal("sorting changed source or moved folders behind files")
	}
	if len(visibleFiles(source, "REPORT", "name", true)) != 1 {
		t.Fatal("case-insensitive filter failed")
	}
	if formatETA(7199) != "剩余 2 小时 0 分钟" {
		t.Fatal("remaining time overflowed to 60 minutes")
	}
}

func TestNativeViewsAndOptionalCapture(t *testing.T) {
	u := testUI(t)
	u.connection = &clientapp.Connection{User: openlist.User{Username: "tester", Role: 2}}
	u.server.SetText("tester · https://cloud.example.com")
	u.listing = openlist.Listing{Write: true, Total: 24}
	for i := 0; i < 24; i++ {
		name := fmt.Sprintf("研究资料-%02d.pdf", i)
		if i < 3 {
			name = []string{"项目文件", "共享资料", "归档"}[i]
		}
		u.listing.Content = append(u.listing.Content, openlist.File{Name: name, IsDir: i < 3, Size: int64(i+1) * 1234567, Modified: "2026-10-28T09:00:00Z"})
	}
	u.status.SetText("界面布局测试数据 · 文件和文件夹可直接拖放上传")
	u.rebuildFiles()
	u.Window.Show()
	if u.fileTable.Length == nil || len(u.files) != 24 {
		t.Fatal("file view was not populated")
	}
	directory := os.Getenv("YUNBO_FYNE_CAPTURE")
	if directory == "" {
		return
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	save := func(name string) {
		t.Helper()
		file, err := os.Create(filepath.Join(directory, name))
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		if err = png.Encode(file, u.Window.Canvas().Capture()); err != nil {
			t.Fatal(err)
		}
	}
	save("fyne-files.png")
	u.Window.Resize(fyne.NewSize(940, 640))
	save("fyne-files-940.png")
	u.Window.Resize(fyne.NewSize(1180, 760))
	u.tabs.SelectIndex(1)
	u.renderTasks([]transfer.Task{
		{ID: 3, Name: "研究数据.zip", Direction: "upload", Status: "running", Done: 650 << 20, Total: 2500 << 20, SpeedBPS: 12 << 20, ETASeconds: 155, CanPause: true, Resumable: true, Phase: "transferring", LocalPath: "D:/资料/研究数据.zip"},
		{ID: 2, Name: "实验记录.pdf", Direction: "download", Status: "paused", Done: 3 << 20, Total: 12 << 20, CanResume: true, Resumable: true, ETASeconds: -1, LocalPath: "D:/下载/实验记录.pdf"},
		{ID: 1, Name: "说明.txt", Direction: "download", Status: "completed", Done: 4096, Total: 4096, ETASeconds: -1, LocalPath: "D:/下载/说明.txt"},
	})
	save("fyne-transfers.png")
	u.connectionDialog()
	save("fyne-connection.png")
}
