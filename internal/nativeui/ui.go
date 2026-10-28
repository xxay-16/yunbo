package nativeui

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sync"
	"sync/atomic"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/xxay-16/yunbo/internal/clientapp"
	"github.com/xxay-16/yunbo/internal/config"
	"github.com/xxay-16/yunbo/internal/openlist"
	"github.com/xxay-16/yunbo/internal/transfer"
)

type UI struct {
	App                                                                                                   fyne.App
	Window                                                                                                fyne.Window
	service                                                                                               *clientapp.Service
	profilePath                                                                                           string
	connection                                                                                            *clientapp.Connection
	dir                                                                                                   string
	page                                                                                                  int
	listing                                                                                               openlist.Listing
	files                                                                                                 []openlist.File
	selected                                                                                              map[string]bool
	passwords                                                                                             map[string]string
	order                                                                                                 string
	ascending                                                                                             bool
	busy                                                                                                  bool
	closing                                                                                               atomic.Bool
	closeOnce                                                                                             sync.Once
	done                                                                                                  chan struct{}
	server, status, pageLabel                                                                             *widget.Label
	pathEntry, filter                                                                                     *widget.Entry
	fileTable                                                                                             *widget.Table
	tasks                                                                                                 []transfer.Task
	taskList                                                                                              *widget.List
	taskSummary                                                                                           *widget.Label
	taskSignature                                                                                         string
	tabs                                                                                                  *container.AppTabs
	connect, upload, uploadFolder, download, mkdir, rename, remove, copyLink, up, refresh, previous, next *widget.Button
}

func New(application fyne.App, service *clientapp.Service, profilePath string) *UI {
	u := &UI{App: application, service: service, profilePath: profilePath, dir: "/", page: 1, selected: map[string]bool{}, passwords: map[string]string{}, order: "name", ascending: true, done: make(chan struct{})}
	u.Window = application.NewWindow("云泊 · Yunbo Fyne")
	u.Window.Resize(fyne.NewSize(1180, 760))
	u.Window.SetMaster()
	u.server = widget.NewLabel("尚未连接")
	u.server.Truncation = fyne.TextTruncateEllipsis
	u.status = widget.NewLabel("连接 OpenList 后即可浏览文件，文件和文件夹可直接拖入窗口上传")
	u.status.Truncation = fyne.TextTruncateEllipsis
	u.connect = widget.NewButtonWithIcon("连接设置", theme.SettingsIcon(), u.connectionDialog)
	background := widget.NewButtonWithIcon("后台", theme.ComputerIcon(), func() { u.Window.Hide() })
	header := container.NewBorder(nil, nil, container.NewHBox(widget.NewIcon(theme.ComputerIcon()), widget.NewLabelWithStyle("云泊 · Fyne", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})),
		container.NewHBox(u.connect, background), u.server)
	u.tabs = container.NewAppTabs(container.NewTabItemWithIcon("全部文件", theme.FolderIcon(), u.filesView()), container.NewTabItemWithIcon("传输中心", theme.DownloadIcon(), u.transfersView()))
	u.tabs.OnSelected = func(tab *container.TabItem) {
		if tab == u.tabs.Items[0] && u.connection != nil && !u.busy && !u.closing.Load() {
			u.loadDir(u.dir, u.page)
		}
	}
	u.Window.SetContent(container.NewBorder(header, u.status, nil, nil, u.tabs))
	if desktopApp, ok := application.(desktop.App); ok {
		desktopApp.SetSystemTrayIcon(application.Icon())
		desktopApp.SetSystemTrayWindow(u.Window)
		desktopApp.SetSystemTrayMenu(fyne.NewMenu("云泊 Fyne",
			fyne.NewMenuItem("显示主窗口", func() { u.Window.Show(); u.Window.RequestFocus() }),
			fyne.NewMenuItem("收进后台", u.Window.Hide),
			fyne.NewMenuItem("打开配置目录", u.openConfig),
			fyne.NewMenuItemSeparator(), fyne.NewMenuItem("退出云泊", u.Quit)))
		u.Window.SetCloseIntercept(u.Window.Hide)
	} else {
		background.Disable()
		u.Window.SetCloseIntercept(u.Quit)
	}
	u.Window.SetMainMenu(fyne.NewMainMenu(fyne.NewMenu("云泊",
		fyne.NewMenuItem("连接设置", u.connectionDialog), fyne.NewMenuItem("打开配置目录", u.openConfig), fyne.NewMenuItem("退出", u.Quit))))
	u.Window.SetOnDropped(func(_ fyne.Position, uris []fyne.URI) {
		if u.Window.Canvas().Overlays().Top() != nil || u.busy || u.tabs.SelectedIndex() != 0 {
			return
		}
		var paths []string
		for _, uri := range uris {
			if uri.Scheme() != "file" {
				u.message(errors.New("只支持本机文件或文件夹"))
				return
			}
			paths = append(paths, uri.Path())
		}
		u.queueUploads(paths)
	})
	u.Window.Canvas().AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyR, Modifier: fyne.KeyModifierControl}, func(fyne.Shortcut) { u.loadDir(u.dir, u.page) })
	u.updateControls()
	if err := service.StorageError(); err != nil {
		u.message(err)
	}
	go u.poll()
	return u
}

func (u *UI) Close() {
	u.closeOnce.Do(func() { u.closing.Store(true); close(u.done); u.service.Close() })
}
func (u *UI) Quit() {
	if u.closing.Swap(true) {
		return
	}
	u.status.SetText("正在保存传输记录…")
	go func() { u.Close(); fyne.Do(func() { u.Window.SetCloseIntercept(nil); u.App.Quit() }) }()
}
func (u *UI) message(err error) {
	if err != nil && !u.closing.Load() {
		u.status.SetText(err.Error())
		dialog.ShowError(err, u.Window)
	}
}
func (u *UI) openConfig() {
	dir := filepath.Dir(u.profilePath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		u.message(err)
		return
	}
	uri, err := url.Parse(storage.NewFileURI(dir).String())
	if err != nil {
		u.message(err)
		return
	}
	u.message(u.App.OpenURL(uri))
}
func (u *UI) async(label string, work func() (any, error), finished func(any, error)) {
	if u.busy || u.closing.Load() {
		return
	}
	u.busy = true
	u.status.SetText(label)
	u.updateControls()
	go func() {
		result, err := work()
		if u.closing.Load() {
			return
		}
		fyne.Do(func() {
			if u.closing.Load() {
				return
			}
			u.busy = false
			u.updateControls()
			finished(result, err)
		})
	}()
}
func (u *UI) can(bit uint) bool {
	return u.connection != nil && (u.connection.User.Role == 2 || u.connection.User.Permission&(1<<bit) != 0 || (bit == 3 && u.listing.WriteContentBypass))
}
func (u *UI) passwordFor(dir string) string {
	best, value := "", ""
	for prefix, password := range u.passwords {
		if (dir == prefix || prefix == "/" || len(dir) > len(prefix) && dir[:len(prefix)+1] == prefix+"/") && len(prefix) > len(best) {
			best, value = prefix, password
		}
	}
	return value
}
func enable(button *widget.Button, yes bool) {
	if yes {
		button.Enable()
	} else {
		button.Disable()
	}
}
func (u *UI) updateControls() {
	connected := u.connection != nil && !u.busy
	writable := connected && u.listing.Write && u.can(3)
	enable(u.connect, !u.busy)
	for _, b := range []*widget.Button{u.upload, u.uploadFolder, u.mkdir} {
		enable(b, writable)
	}
	enable(u.refresh, connected)
	enable(u.up, connected && u.dir != "/")
	enable(u.download, connected && len(u.selected) > 0 && u.selectedOnlyFiles())
	enable(u.copyLink, connected && len(u.selected) == 1 && u.selectedOnlyFiles())
	enable(u.rename, connected && u.listing.Write && len(u.selected) == 1 && u.can(4))
	enable(u.remove, connected && u.listing.Write && len(u.selected) > 0 && u.can(7))
	enable(u.previous, connected && u.page > 1)
	enable(u.next, connected && u.page*100 < u.listing.Total)
	if connected {
		u.pathEntry.Enable()
		u.filter.Enable()
	} else {
		u.pathEntry.Disable()
		u.filter.Disable()
	}
}
func (u *UI) selectedNames() []string {
	names := make([]string, 0, len(u.selected))
	for _, file := range u.listing.Content {
		if u.selected[file.Name] {
			names = append(names, file.Name)
		}
	}
	return names
}
func (u *UI) selectedOnlyFiles() bool {
	for _, file := range u.listing.Content {
		if u.selected[file.Name] && file.IsDir {
			return false
		}
	}
	return true
}
func (u *UI) rebuildFiles() {
	u.files = visibleFiles(u.listing.Content, u.filter.Text, u.order, u.ascending)
	u.fileTable.Refresh()
	u.pageLabel.SetText(fmt.Sprintf("第 %d 页 · 共 %d 项 · 已选 %d 项", u.page, u.listing.Total, len(u.selected)))
	u.updateControls()
}
func (u *UI) loadDir(dir string, page int) {
	if u.connection == nil {
		return
	}
	dir = openlist.CleanPath(dir)
	password := u.passwordFor(dir)
	u.async("正在读取 "+dir, func() (any, error) { return u.service.List(dir, password, page) }, func(value any, err error) {
		if err != nil {
			var api *openlist.APIError
			if errors.As(err, &api) && api.Code == 403 {
				u.passwordDialog("目录访问密码", func(password string) { u.passwords[dir] = password; u.loadDir(dir, page) })
				return
			}
			u.message(err)
			return
		}
		u.listing = value.(openlist.Listing)
		u.dir = dir
		u.page = page
		u.selected = map[string]bool{}
		u.pathEntry.SetText(dir)
		u.rebuildFiles()
		u.status.SetText("当前目录：" + dir + " · 拖入文件或文件夹即可上传")
	})
}
func (u *UI) passwordDialog(title string, action func(string)) {
	password := widget.NewPasswordEntry()
	form := dialog.NewForm(title, "确定", "取消", []*widget.FormItem{widget.NewFormItem("访问密码", password)}, func(ok bool) {
		if ok {
			action(password.Text)
		}
		password.SetText("")
	}, u.Window)
	form.Resize(fyne.NewSize(420, 180))
	form.Show()
}
func (u *UI) queueUploads(paths []string) {
	if u.connection == nil || !u.listing.Write || !u.can(3) {
		u.message(errors.New("当前目录或账号没有上传权限"))
		return
	}
	dir := u.dir
	u.async("正在检查上传项目…", func() (any, error) { return u.service.UploadPaths(dir, paths) }, u.queueResult)
}
func (u *UI) queueResult(value any, err error) {
	if err != nil {
		u.message(err)
		return
	}
	result := value.(clientapp.QueueResult)
	u.status.SetText(fmt.Sprintf("已加入 %d 项传输，创建 %d 个目录", result.Count, result.Folders))
	u.tabs.SelectIndex(1)
	if len(result.Errors) > 0 {
		u.message(fmt.Errorf("部分项目未加入：%v", result.Errors))
	}
}
func (u *UI) pickUpload(folder bool) {
	if folder {
		dialog.ShowFolderOpen(func(uri fyne.ListableURI, err error) {
			if err != nil {
				u.message(err)
			} else if uri != nil {
				u.queueUploads([]string{uri.Path()})
			}
		}, u.Window)
		return
	}
	dialog.ShowFileOpen(func(reader fyne.URIReadCloser, err error) {
		if err != nil {
			u.message(err)
			return
		}
		if reader == nil {
			return
		}
		name := reader.URI().Path()
		reader.Close()
		u.queueUploads([]string{name})
	}, u.Window)
}
func (u *UI) pickDownload() {
	names, dir, password := u.selectedNames(), u.dir, u.passwordFor(u.dir)
	dialog.ShowFolderOpen(func(uri fyne.ListableURI, err error) {
		if err != nil {
			u.message(err)
			return
		}
		if uri == nil {
			return
		}
		u.async("正在准备下载…", func() (any, error) { return u.service.DownloadFiles(dir, names, password, uri.Path()) }, u.queueResult)
	}, u.Window)
}
func (u *UI) fileAction(action string) {
	names, dir := u.selectedNames(), u.dir
	if action == "link" {
		if len(names) != 1 {
			return
		}
		password := u.passwordFor(dir)
		u.async("正在获取下载链接…", func() (any, error) { return u.service.DownloadLink(path.Join(dir, names[0]), password) }, func(value any, err error) {
			if err != nil {
				u.message(err)
				return
			}
			u.App.Clipboard().SetContent(value.(string))
			u.status.SetText("签名下载链接已复制")
		})
		return
	}
	if action == "delete" {
		dialog.ShowConfirm("删除云端文件", fmt.Sprintf("确定删除选中的 %d 个项目？此操作不能撤销。", len(names)), func(ok bool) {
			if !ok {
				return
			}
			u.async("正在删除…", func() (any, error) { return nil, u.service.Remove(dir, names) }, func(_ any, err error) {
				if err != nil {
					u.message(err)
				}
				u.loadDir(dir, u.page)
			})
		}, u.Window)
		return
	}
	name := widget.NewEntry()
	title := "新建文件夹"
	if action == "rename" {
		if len(names) != 1 {
			return
		}
		title = "重命名"
		name.SetText(names[0])
	}
	form := dialog.NewForm(title, "确定", "取消", []*widget.FormItem{widget.NewFormItem("名称", name)}, func(ok bool) {
		if !ok {
			return
		}
		if err := openlist.ValidateName(name.Text); err != nil {
			u.message(err)
			return
		}
		newName := name.Text
		u.async("正在保存…", func() (any, error) {
			if action == "rename" {
				return nil, u.service.Rename(path.Join(dir, names[0]), newName)
			}
			return nil, u.service.Mkdir(dir, newName)
		}, func(_ any, err error) {
			if err != nil {
				u.message(err)
				return
			}
			u.loadDir(dir, u.page)
		})
	}, u.Window)
	form.Resize(fyne.NewSize(460, 180))
	form.Show()
}

func (u *UI) filesView() fyne.CanvasObject {
	u.upload = widget.NewButtonWithIcon("上传文件", theme.UploadIcon(), func() { u.pickUpload(false) })
	u.uploadFolder = widget.NewButtonWithIcon("上传文件夹", theme.FolderOpenIcon(), func() { u.pickUpload(true) })
	u.mkdir = widget.NewButtonWithIcon("新建", theme.FolderNewIcon(), func() { u.fileAction("mkdir") })
	u.download = widget.NewButtonWithIcon("下载", theme.DownloadIcon(), u.pickDownload)
	u.rename = widget.NewButtonWithIcon("重命名", theme.DocumentCreateIcon(), func() { u.fileAction("rename") })
	u.remove = widget.NewButtonWithIcon("删除", theme.DeleteIcon(), func() { u.fileAction("delete") })
	u.copyLink = widget.NewButtonWithIcon("复制链接", theme.ContentCopyIcon(), func() { u.fileAction("link") })
	toolbar := container.NewHBox(u.upload, u.uploadFolder, u.mkdir, layout.NewSpacer(), u.download, u.copyLink, u.rename, u.remove)
	u.pathEntry = widget.NewEntry()
	u.pathEntry.SetText("/")
	u.pathEntry.OnSubmitted = func(value string) { u.loadDir(value, 1) }
	u.filter = widget.NewEntry()
	u.filter.SetPlaceHolder("筛选当前页")
	u.filter.OnChanged = func(string) { u.selected = map[string]bool{}; u.rebuildFiles() }
	u.up = widget.NewButtonWithIcon("", theme.NavigateBackIcon(), func() { u.loadDir(path.Dir(u.dir), 1) })
	u.refresh = widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), func() { u.loadDir(u.dir, u.page) })
	dirPassword := widget.NewButtonWithIcon("", theme.VisibilityOffIcon(), func() {
		u.passwordDialog("设置当前目录访问密码", func(value string) { u.passwords[u.dir] = value; u.loadDir(u.dir, u.page) })
	})
	search := container.NewGridWrap(fyne.NewSize(190, 36), u.filter)
	pathbar := container.NewBorder(nil, nil, container.NewHBox(u.up, u.refresh, dirPassword), search, u.pathEntry)
	u.fileTable = widget.NewTable(func() (int, int) { return len(u.files), 4 }, func() fyne.CanvasObject {
		label := widget.NewLabel("")
		label.Truncation = fyne.TextTruncateEllipsis
		button := widget.NewButtonWithIcon("文件名", theme.DocumentIcon(), nil)
		button.Importance = widget.LowImportance
		button.Alignment = widget.ButtonAlignLeading
		return container.NewStack(widget.NewCheck("", nil), label, button)
	}, func(id widget.TableCellID, object fyne.CanvasObject) {
		if id.Row < 0 || id.Row >= len(u.files) {
			return
		}
		file := u.files[id.Row]
		cell := object.(*fyne.Container)
		check, label, button := cell.Objects[0].(*widget.Check), cell.Objects[1].(*widget.Label), cell.Objects[2].(*widget.Button)
		check.Hide()
		label.Hide()
		button.Hide()
		switch id.Col {
		case 0:
			check.OnChanged = nil
			check.SetChecked(u.selected[file.Name])
			check.OnChanged = func(on bool) {
				if on {
					u.selected[file.Name] = true
				} else {
					delete(u.selected, file.Name)
				}
				u.rebuildFiles()
			}
			check.Show()
		case 1:
			button.SetText(file.Name)
			if file.IsDir {
				button.SetIcon(theme.FolderIcon())
			} else {
				button.SetIcon(theme.DocumentIcon())
			}
			button.OnTapped = func() {
				if file.IsDir {
					u.loadDir(path.Join(u.dir, file.Name), 1)
				} else {
					u.selected[file.Name] = !u.selected[file.Name]
					if !u.selected[file.Name] {
						delete(u.selected, file.Name)
					}
					u.rebuildFiles()
				}
			}
			button.Show()
		case 2:
			if file.IsDir {
				label.SetText("文件夹")
			} else {
				label.SetText(formatBytes(file.Size))
			}
			label.Show()
		case 3:
			label.SetText(formatDate(file.Modified))
			label.Show()
		}
	})
	u.fileTable.ShowHeaderRow = true
	u.fileTable.CreateHeader = func() fyne.CanvasObject {
		b := widget.NewButton("", nil)
		b.Importance = widget.LowImportance
		return b
	}
	u.fileTable.UpdateHeader = func(id widget.TableCellID, object fyne.CanvasObject) {
		b := object.(*widget.Button)
		b.SetText([]string{"全选", "名称", "大小", "修改时间"}[id.Col])
		b.OnTapped = func() {
			if id.Col == 0 {
				all := len(u.files) > 0
				for _, f := range u.files {
					all = all && u.selected[f.Name]
				}
				for _, f := range u.files {
					if all {
						delete(u.selected, f.Name)
					} else {
						u.selected[f.Name] = true
					}
				}
			} else {
				order := []string{"", "name", "size", "modified"}[id.Col]
				if u.order == order {
					u.ascending = !u.ascending
				} else {
					u.order = order
					u.ascending = true
				}
			}
			u.rebuildFiles()
		}
	}
	for col, width := range []float32{48, 520, 120, 180} {
		u.fileTable.SetColumnWidth(col, width)
	}
	u.pageLabel = widget.NewLabel("第 1 页 · 共 0 项")
	u.previous = widget.NewButtonWithIcon("上一页", theme.NavigateBackIcon(), func() { u.loadDir(u.dir, u.page-1) })
	u.next = widget.NewButtonWithIcon("下一页", theme.NavigateNextIcon(), func() { u.loadDir(u.dir, u.page+1) })
	footer := container.NewBorder(nil, nil, u.pageLabel, container.NewHBox(u.previous, u.next))
	return container.NewBorder(container.NewVBox(toolbar, pathbar), footer, nil, nil, u.fileTable)
}

func (u *UI) connectionDialog() {
	profile, err := u.service.LoadProfile()
	if err != nil {
		u.message(err)
		return
	}
	if _, err = os.Stat(u.profilePath); errors.Is(err, os.ErrNotExist) {
		base := filepath.Dir(filepath.Dir(u.profilePath))
		if p, e := config.Load(filepath.Join(base, "yunbo", "profile.json")); e == nil {
			profile = p
		}
	}
	address, username, start := widget.NewEntry(), widget.NewEntry(), widget.NewEntry()
	address.SetText(profile.URL)
	address.SetPlaceHolder("https://cloud.example.com")
	username.SetText(profile.Username)
	start.SetText(profile.StartPath)
	if start.Text == "" {
		start.SetText("/")
	}
	password, token, otp, dirPassword := widget.NewPasswordEntry(), widget.NewPasswordEntry(), widget.NewEntry(), widget.NewPasswordEntry()
	mode := widget.NewSelect([]string{"账号密码", "访问令牌", "访客"}, nil)
	mode.SetSelected(map[string]string{"password": "账号密码", "token": "访问令牌", "guest": "访客"}[profile.AuthMode])
	if mode.Selected == "" {
		mode.SetSelected("账号密码")
	}
	syncMode := func() {
		if mode.Selected == "账号密码" {
			username.Enable()
			password.Enable()
			otp.Enable()
			token.Disable()
		} else if mode.Selected == "访问令牌" {
			username.Disable()
			password.Disable()
			otp.Disable()
			token.Enable()
		} else {
			username.Disable()
			password.Disable()
			otp.Disable()
			token.Disable()
		}
	}
	mode.OnChanged = func(string) { syncMode() }
	syncMode()
	info := widget.NewLabel("不保存密码、令牌或验证码。Fyne 版使用独立配置与传输记录。")
	info.Wrapping = fyne.TextWrapWord
	var form *dialog.FormDialog
	disconnect := widget.NewButton("断开连接", func() {
		u.service.Disconnect()
		u.connection = nil
		u.listing = openlist.Listing{}
		u.selected = map[string]bool{}
		u.passwords = map[string]string{}
		u.dir = "/"
		u.page = 1
		u.pathEntry.SetText("/")
		u.server.SetText("尚未连接")
		u.rebuildFiles()
		u.status.SetText("已断开；可续传任务保留")
		password.SetText("")
		token.SetText("")
		otp.SetText("")
		dirPassword.SetText("")
		form.Hide()
	})
	if u.connection == nil {
		disconnect.Disable()
	}
	actions := container.NewHBox(widget.NewButton("打开配置目录", u.openConfig), disconnect)
	form = dialog.NewForm("连接 OpenList", "连接", "取消", []*widget.FormItem{
		widget.NewFormItem("服务地址", address), widget.NewFormItem("登录方式", mode), widget.NewFormItem("用户名", username),
		widget.NewFormItem("密码", password), widget.NewFormItem("两步验证码", otp), widget.NewFormItem("访问令牌", token),
		widget.NewFormItem("起始目录", start), widget.NewFormItem("目录密码", dirPassword), widget.NewFormItem("", info),
		widget.NewFormItem("", actions),
	}, func(ok bool) {
		if !ok {
			password.SetText("")
			token.SetText("")
			dirPassword.SetText("")
			return
		}
		req := clientapp.ConnectRequest{}
		req.URL = address.Text
		req.Username = username.Text
		req.StartPath = start.Text
		req.AuthMode = map[string]string{"账号密码": "password", "访问令牌": "token", "访客": "guest"}[mode.Selected]
		req.Password = password.Text
		req.Token = token.Text
		req.OTP = otp.Text
		req.DirectoryPassword = dirPassword.Text
		password.SetText("")
		token.SetText("")
		otp.SetText("")
		dirPassword.SetText("")
		u.async("正在连接服务器…", func() (any, error) { return u.service.Connect(req) }, func(value any, err error) {
			if err != nil {
				u.message(err)
				return
			}
			connection := value.(clientapp.Connection)
			u.connection = &connection
			u.passwords = map[string]string{}
			if req.DirectoryPassword != "" {
				u.passwords[connection.Profile.StartPath] = req.DirectoryPassword
			}
			u.dir = connection.Profile.StartPath
			u.page = 1
			u.listing = connection.Listing
			u.selected = map[string]bool{}
			u.pathEntry.SetText(u.dir)
			u.server.SetText(connection.User.Username + " · " + connection.Profile.URL)
			u.rebuildFiles()
			u.status.SetText("已连接 · " + u.dir)
			if connection.Warning != "" {
				u.message(errors.New(connection.Warning))
			}
		})
	}, u.Window)
	form.Resize(fyne.NewSize(600, 480))
	form.Show()
}
