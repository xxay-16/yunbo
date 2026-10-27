# 云泊 · Yunbo

用 Go + Wails 2 编写的 OpenList 桌面客户端，连接你已经部署的云端服务。在独立窗口中浏览、上传、下载和管理文件。当前首版面向 Windows，界面为中文。

云泊（Yunbo）是独立客户端，通过 HTTP API 与 [OpenList](https://github.com/OpenListTeam/OpenList) 通信，不包含 OpenList 服务端或存储驱动，与 OpenList Team 无官方关联。

![云泊 · Yunbo 首版界面](docs/images/preview.png)

## 首版功能

- 账号密码登录、两步验证码、访问令牌、访客访问。
- 记住上次连接的服务地址、用户名、登录方式和起始目录。
- 支持反向代理子路径，例如 `https://example.com/openlist`。
- 文件夹浏览、路径跳转、分页（每页 100 项）、当前页筛选与排序。
- 新建文件夹、重命名、批量删除；按钮根据账号和目录权限启用，最终由服务器校验。
- 选择本机文件批量上传、选择保存目录批量下载、复制签名下载链接。
- 两个并行传输、进度显示、排队、取消、失败信息及清除已结束任务。
- 流式传输，不把整个文件加载进内存。上传拒绝覆盖同名文件；下载完成后发布文件，同名文件自动编号，取消或失败会清理临时文件。
- 为受保护目录输入访问密码，密码仅在当前会话保留。

## 运行与构建

需要 Go 1.23+、[Wails 2](https://wails.io/docs/gettingstarted/installation/) 及对应桌面依赖。Windows 需要 WebView2 Runtime。当前开发机已安装这些依赖。

```powershell
# 检查桌面环境
wails doctor

# 开发运行
.\scripts\dev.ps1

# 构建 Windows 程序
.\scripts\build.ps1

# 后端测试和静态检查
go test ./...
go vet ./...
```

生成的程序位于 `build/bin/yunbo.exe`。前端使用原生 HTML/CSS/JavaScript，无需 npm 安装或前端构建步骤。Wails CLI 会生成绑定文件，运行时通过 Wails bridge 调用 Go。

脚本使用 `-skipembedcreate`：资源目录已在仓库中，无需 Wails 自动创建，同时避开 Wails 2.11 内置旧版 `x/tools` 在本机 Go 1.27 下的资源扫描错误。也可直接执行 `wails build -skipembedcreate` 或 `wails dev -skipembedcreate`。构建时保留输出目录中其他名称的程序。

## 连接你的云端

1. 启动程序，点击「添加服务器」。
2. 填写服务根地址，例如 `https://cloud.example.com`。请勿填写 `/api`、`/dav` 或网页的文件夹地址；反向代理部署前缀需要保留。
3. 选择账号密码、访问令牌或访客登录。账号开启两步验证时填写验证码。
4. 起始目录默认为 `/`，相对于当前账号的可访问根目录。受保护的起始目录可在「高级设置」输入目录密码。
5. 连接后，点击文件夹名称进入；勾选文件即可下载、复制链接或管理。上传、下载时使用系统文件选择窗口。

首次运行会兼容读取旧版 `openlist-desk` 目录中的连接信息，成功连接后保存到新目录。连接信息保存在系统配置目录：Windows 通常为 `%AppData%/yunbo/profile.json`。**密码、访问令牌、验证码和目录密码不会写入磁盘**，退出后需重新登录。跨公网连接请使用 HTTPS；HTTP 仅适合你信任的网络环境。API 请求由 Go 发出，无需为本客户端开放浏览器 CORS。使用系统证书验证，不跳过 TLS 校验。

下载先通过 `/api/fs/get` 获取签名，再访问服务器 `/d/` 下载路由，支持存储重定向；下载请求不携带账号令牌。账号具有非 `/` 的 `base_path` 时，客户端会转换为下载路由所需的完整路径。复制的下载链接包含访问签名。

## 代码结构

```text
main.go                  Wails 窗口与静态资源
app.go                   桌面桥接、会话、原生文件选择窗口
internal/openlist/       HTTP API、登录哈希、流式上传下载
internal/config/         不含凭据的连接配置
internal/transfer/       并发队列、取消、进度与下载文件发布
frontend/                中文桌面界面
docs/architecture.md     接口、约束和后续路线
```

## 当前边界

当前版本没有本地目录自动同步、虚拟磁盘挂载、断点续传、整文件夹上传下载、托盘、自启动或安装包。传输队列只在内存中保留，断开连接、切换服务器或关闭应用会取消未完成任务。取消上传时，远端存储是否保留已经接收的内容由 OpenList 和存储驱动决定。服务端可能分批执行批量删除，遇到失败时应刷新目录核对实际结果。

接口已依照 OpenList 上游源码核对，并使用本地 HTTP 测试服务验证。尚未使用你的真实云端服务联调，云盘驱动、反向代理的上传大小限制及账号权限需要在你的实例上确认。

## Git

GitHub 仓库：[xxay-16/yunbo](https://github.com/xxay-16/yunbo)。

构建产物、日志和本地环境文件不会提交。

```powershell
git clone https://github.com/xxay-16/yunbo.git
cd yunbo
.\scripts\dev.ps1
```
