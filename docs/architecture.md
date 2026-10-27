# 设计与接口记录

## 目标

把用户已部署的 OpenList 作为远端文件服务，客户端专注桌面文件操作和传输体验。Go 负责网络、磁盘、会话和任务队列，Wails 提供原生窗口、文件选择和 Go/JavaScript 桥接，HTML/CSS/JavaScript 负责界面。

没有嵌入 OpenList 服务端，没有再次实现各网盘驱动，也没有把用户凭据发送给第三方服务。

## 接口依据

2026-10-27 核对 [OpenList 上游路由](https://github.com/OpenListTeam/OpenList/blob/main/server/router.go)、[认证处理](https://github.com/OpenListTeam/OpenList/blob/main/server/handles/auth.go)、[用户模型](https://github.com/OpenListTeam/OpenList/blob/main/internal/model/user.go)、[目录读取](https://github.com/OpenListTeam/OpenList/blob/main/server/handles/fsread.go)、[文件管理](https://github.com/OpenListTeam/OpenList/blob/main/server/handles/fsmanage.go)和[上传处理](https://github.com/OpenListTeam/OpenList/blob/main/server/handles/fsup.go)。

| 操作 | 接口 | 关键行为 |
| --- | --- | --- |
| 密码登录 | `POST /api/auth/login/hash` | SHA-256(password + `-https://github.com/alist-org/alist`)，支持 `otp_code` |
| 当前账号 | `GET /api/me` | 读取账号权限与 `base_path` |
| 目录 | `POST /api/fs/list` | `path`, `password`, `page`, `per_page=100`；`refresh=false`，避免只读账号触发强制刷新权限错误 |
| 文件详情 | `POST /api/fs/get` | 校验目录访问，获取下载签名和大小 |
| 上传 | `PUT /api/fs/put` | 二进制流，URL 编码 `File-Path`，`Content-Length`，`As-Task=false`，`Overwrite=false` |
| 下载 | `GET /d/<账号完整路径>?sign=...` | 不发送账号令牌，跟随存储重定向 |
| 新建目录 | `POST /api/fs/mkdir` | `path` |
| 重命名 | `POST /api/fs/rename` | `path`, `name`, `overwrite=false` |
| 删除 | `POST /api/fs/remove` | `dir`, `names`；客户端确认后才请求 |

API 既检查 HTTP 状态，也检查响应 JSON 中的 `code`；HTTP 200 并不意味着操作成功。API 请求禁止重定向，避免凭据发送到代理登录页或其他域名。目录读取和普通操作有 35 秒超时，传输使用会话取消上下文和响应头超时。

## 会话与文件安全

- 连接成功必须通过当前用户读取与起始目录读取两个检查。新连接失败时保留原有会话。
- 登录密码、令牌不返回界面；只在 Go 会话中持有令牌。配置类型没有凭据字段。
- 切换服务器先取消旧会话，旧任务始终持有旧客户端实例，不会误用新服务器凭据。
- 限制最多两个并发传输、200 个任务记录，可清除已结束任务继续加入。
- 下载使用随机 `.part` 文件，完成后独占预留目标文件名并重命名发布，已有同名文件自动编号。
- Windows 本地文件名拒绝路径穿越、设备名称、备用数据流和不受支持的字符。
- 使用文本 DOM 渲染远端文件名、路径和错误，不渲染远端 HTML/README，不加载远端脚本。
- 客户端权限按钮只提供交互提示，权限授权始终以服务端校验为准。

## 验证范围

自动化测试覆盖密码哈希/OTP、子路径和分页请求、HTTP/业务错误、API 重定向隔离、包含中文和特殊字符的上传下载、非根账号路径、截断下载、下载重名保护、取消清理、并发约束、配置替换和本地文件名检查。桌面构建使用 Wails CLI。

## 后续可选方向

1. 使用系统凭据管理器实现安全的持久登录和多服务器配置。
2. 支持暂停/续传、任务持久化、限速与重试；先确定各存储的 Range 和上传能力。
3. 目录递归上传下载、移动/复制、服务端索引搜索和文件预览。
4. 如需同步本机目录，应先设计冲突策略、删除策略、忽略规则和本地数据库，再引入文件系统监听。
5. 托盘、通知、安装包、自动更新和其他桌面平台构建。
