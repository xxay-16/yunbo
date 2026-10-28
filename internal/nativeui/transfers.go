package nativeui

import (
	"encoding/json"
	"fmt"
	"path"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/xxay-16/yunbo/internal/transfer"
)

type transferRow struct {
	widget.BaseWidget
	name, meta, details  *widget.Label
	progress             *compactProgress
	action, cancel, info *widget.Button
	content              fyne.CanvasObject
}

func newTransferRow() *transferRow {
	row := &transferRow{name: widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), meta: widget.NewLabel(""), details: widget.NewLabel(""), progress: newCompactProgress(), action: widget.NewButton("暂停", nil), cancel: widget.NewButtonWithIcon("", theme.CancelIcon(), nil), info: widget.NewButtonWithIcon("", theme.InfoIcon(), nil)}
	row.name.Truncation = fyne.TextTruncateEllipsis
	row.meta.Truncation = fyne.TextTruncateEllipsis
	row.details.Truncation = fyne.TextTruncateEllipsis
	row.action.Importance = widget.LowImportance
	row.cancel.Importance = widget.LowImportance
	row.info.Importance = widget.LowImportance
	row.content = container.NewBorder(nil, nil, nil, container.NewHBox(row.action, row.cancel, row.info),
		container.NewVBox(row.name, row.meta, row.progress, row.details))
	row.ExtendBaseWidget(row)
	return row
}
func (row *transferRow) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(row.content)
}

func (u *UI) transfersView() fyne.CanvasObject {
	u.taskSummary = widget.NewLabel("暂无传输任务")
	clear := widget.NewButtonWithIcon("清除已结束", theme.ContentClearIcon(), func() {
		if err := u.service.ClearFinishedTransfers(); err != nil {
			u.message(err)
		}
		u.renderTasks(u.service.Transfers())
	})
	toolbar := container.NewHBox(u.taskSummary, layout.NewSpacer(), clear)
	u.taskList = widget.NewList(func() int { return len(u.tasks) }, func() fyne.CanvasObject { return newTransferRow() }, func(id widget.ListItemID, object fyne.CanvasObject) {
		if id >= len(u.tasks) {
			return
		}
		task := u.tasks[id]
		row := object.(*transferRow)
		direction := "下载"
		if task.Direction == "upload" {
			direction = "上传"
		}
		row.name.SetText(task.Name)
		meta := fmt.Sprintf("%s · %s / %s · %s", direction, formatBytes(task.Done), formatBytes(task.Total), taskStatus(task))
		if task.Status == "running" && task.Phase == "transferring" {
			meta += fmt.Sprintf(" · %s/s · %s", formatBytes(task.SpeedBPS), formatETA(task.ETASeconds))
		}
		row.meta.SetText(meta)
		row.details.SetText(task.LocalPath)
		if task.Error != "" {
			row.details.SetText(task.Error)
		} else if task.ResumeReason != "" && task.Status != "completed" {
			row.details.SetText(task.ResumeReason)
		}
		progress := 0.0
		if task.Total > 0 {
			progress = float64(task.Done) / float64(task.Total)
		}
		if task.Status == "completed" {
			progress = 1
		}
		row.progress.SetValue(progress)
		row.action.Show()
		switch {
		case task.CanPause:
			row.action.SetText("暂停")
			row.action.Enable()
			row.action.OnTapped = func() { u.performTask(task.ID, "pause", "") }
		case task.CanResume:
			row.action.SetText("继续")
			row.action.Enable()
			row.action.OnTapped = func() {
				if task.NeedsPassword {
					u.passwordDialog("下载访问密码", func(password string) { u.performTask(task.ID, "resume", password) })
				} else {
					u.performTask(task.ID, "resume", "")
				}
			}
		case task.Status == "paused" || task.Status == "failed":
			row.action.SetText("继续")
			row.action.Disable()
		default:
			row.action.Hide()
		}
		row.cancel.Show()
		row.cancel.OnTapped = func() { u.performTask(task.ID, "cancel", "") }
		if task.Status == "completed" || task.Status == "cancelled" {
			row.cancel.Hide()
		}
		row.info.OnTapped = func() {
			dialog.ShowInformation("传输详情", fmt.Sprintf("文件：%s\n状态：%s\n服务器：%s\n账号：%s\n云端：%s\n本机：%s\n%s\n%s", task.Name, taskStatus(task), task.Server, task.Account, task.RemotePath, task.LocalPath, task.Error, task.ResumeReason), u.Window)
		}
	})
	note := widget.NewLabel("任务记录保存在本机；重启后登录原服务器和账号可继续。上传断点受服务端会话有效期限制。")
	note.Truncation = fyne.TextTruncateEllipsis
	return container.NewBorder(toolbar, note, nil, nil, u.taskList)
}
func (u *UI) performTask(id int64, action, password string) {
	if u.closing.Load() {
		return
	}
	go func() {
		var err error
		switch action {
		case "pause":
			err = u.service.PauseTransfer(id)
		case "resume":
			err = u.service.ResumeTransfer(id, password)
		case "cancel":
			u.service.CancelTransfer(id)
		}
		if u.closing.Load() {
			return
		}
		tasks := u.service.Transfers()
		fyne.Do(func() {
			if u.closing.Load() {
				return
			}
			if err != nil {
				u.message(err)
			}
			u.renderTasks(tasks)
		})
	}()
}
func (u *UI) renderTasks(tasks []transfer.Task) {
	u.tasks = tasks
	var active, waiting, completed int
	for _, task := range tasks {
		switch task.Status {
		case "queued", "running", "pausing", "cancelling":
			active++
		case "paused", "failed":
			waiting++
		case "completed":
			completed++
		}
	}
	u.taskSummary.SetText(fmt.Sprintf("%d 进行中 · %d 待继续 · %d 已完成", active, waiting, completed))
	u.taskList.Refresh()
}
func (u *UI) poll() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-u.done:
			return
		case <-ticker.C:
			tasks := u.service.Transfers()
			signature, _ := json.Marshal(tasks)
			text := string(signature)
			if u.closing.Load() {
				return
			}
			fyne.Do(func() {
				if u.closing.Load() || text == u.taskSignature {
					return
				}
				refresh := false
				old := map[int64]string{}
				for _, task := range u.tasks {
					old[task.ID] = task.Status
				}
				for _, task := range tasks {
					if task.Direction == "upload" && task.Status == "completed" && old[task.ID] != "completed" && path.Dir(task.RemotePath) == u.dir {
						refresh = true
					}
				}
				u.taskSignature = text
				u.renderTasks(tasks)
				if refresh && u.connection != nil && !u.busy && u.tabs.SelectedIndex() == 0 {
					u.loadDir(u.dir, u.page)
				}
			})
		}
	}
}
