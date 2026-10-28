云泊是使用 Go + Wails 编写的 OpenList 桌面客户端。

本次更新（v0.3.1）：

- 新增收进后台：关闭窗口后连接和传输继续。
- Windows 托盘、macOS 菜单栏提供恢复窗口、打开配置目录与退出入口。
- 再次启动程序会恢复已有窗口；连接设置中可直接打开配置目录。

- 紧凑中文界面，支持文件和文件夹拖放上传。
- 上传、下载显示实时速度和预计剩余时间。
- 分片上传、Range 下载支持暂停、继续及客户端重启后恢复记录。
- 支持登录、浏览、新建目录、重命名、删除和签名下载链接。

下载与安装：

- Windows 10/11 x64：下载 windows-amd64-setup.exe 安装，或解压 portable.zip 直接运行。安装程序在当前用户目录安装，缺少 WebView2 时会联网安装运行时。
- macOS 13+（Intel / Apple Silicon）：下载 macos-universal.dmg，将「云泊」拖入 Applications。
- SHA256SUMS.txt 提供所有安装包的 SHA-256 校验值。

当前安装包未使用商业代码签名证书；macOS 仅作 ad-hoc 签名，未通过 Apple 公证。首次启动时，系统可能提示来源未验证，可按 [Apple 的官方说明](https://support.apple.com/guide/mac-help/open-a-mac-app-from-an-unknown-developer-mh40616/mac)在「系统设置 → 隐私与安全性」允许打开。

上传续传需要服务端启用 OpenList 分片上传；服务端重启或会话过期后需重新上传。密码和访问令牌不会保存到磁盘。macOS 包会检查双架构、签名和 DMG 完整性，界面仍需在实际 Mac 上验收。
