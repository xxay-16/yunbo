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
| 分片初始化 | `POST /api/fs/multipart/init` | `File-Path`, `X-File-Size`, `X-Chunk-Size`, `X-File-Sha256`, `Last-Modified`, `Overwrite=false` |
| 分片上传 | `PUT /api/fs/multipart/chunk` | `X-Upload-Id`, `X-Chunk-Index`；流式发送，409/429 后查询状态并退避重试 |
| 会话查询 | `GET /api/fs/multipart/status` | `upload_id` 或 `path` + `size`；继续前核对服务器已确认的分片 |
| 完成/取消 | `POST /api/fs/multipart/complete`, `POST /api/fs/multipart/abort` | 完成需等待存储驱动确认，暂停不调用 abort |
| 下载 | `GET /d/<账号完整路径>?sign=...` | 不发送账号令牌，跟随存储重定向 |
| 新建目录 | `POST /api/fs/mkdir` | `path` |
| 重命名 | `POST /api/fs/rename` | `path`, `name`, `overwrite=false` |
| 删除 | `POST /api/fs/remove` | `dir`, `names`；客户端确认后才请求 |

API 既检查 HTTP 状态，也检查响应 JSON 中的 `code`；HTTP 200 并不意味着操作成功。API 请求禁止重定向，避免凭据发送到代理登录页或其他域名。目录读取和普通操作有 35 秒超时，传输使用会话取消上下文和响应头超时。

## 会话与文件安全

- 连接成功必须通过当前用户读取与起始目录读取两个检查。新连接失败时保留原有会话。
- 登录密码、令牌不返回界面；只在 Go 会话中持有令牌。配置类型没有凭据字段。
- 切换服务器取消旧会话的网络请求，可续传任务保留。继续时必须匹配服务器与账号，工作请求使用对应客户端实例。
- 限制最多两个并发传输、200 个任务记录，可清除已结束任务继续加入。
- 下载使用随机 `.part` 文件，完成后独占预留目标文件名并重命名发布，已有同名文件自动编号。
- Windows 本地文件名拒绝路径穿越、设备名称、备用数据流和不受支持的字符。
- 使用文本 DOM 渲染远端文件名、路径和错误，不渲染远端 HTML/README，不加载远端脚本。
- 客户端权限按钮只提供交互提示，权限授权始终以服务端校验为准。

## 拖放与文件夹上传

界面继续使用 Wails。`EnableFileDrop` 开启系统文件路径解析，前端通过 `runtime.OnFileDrop` 接收真实路径。只有显示中的可写文件列表区域允许拖放，弹窗和传输准备期间不接受新的拖放。文件选择、文件夹选择与拖放最终共用 `internal/clientapp.Service.UploadPaths`，由 Go 读取文件及创建传输任务。

上传预检先遍历本机目录，检查符号链接、目标重名和队列容量，再创建远端目录与任务。父目录先于子目录创建，保留空目录；远端目录创建失败时跳过其子树并报告错误。文件内容通过分片或普通流式接口上传，拒绝覆盖同名文件。退出时队列停止接收任务，保留可续传任务的断点并取消普通传输。

## 速度、断点与恢复

分片协议按 [OpenList v4.2.9 上传处理](https://github.com/OpenListTeam/OpenList/blob/v4.2.9/server/handles/multipart.go)及[会话实现](https://github.com/OpenListTeam/OpenList/blob/v4.2.9/internal/multipart/session.go)核对。登录后通过只读 status 请求区分 API 返回的会话不存在与 HTTP 路由不存在，并读取公开配置确认启用状态；旧服务端回退普通上传。

上传先计算完整 SHA-256。恢复后重新校验本机大小、修改时间及完整哈希，再根据服务端 `received` 跳过已确认分片，处理请求成功但应答丢失的情况。Windows 使用仅允许共享读取的源文件句柄。若会话过期、状态失败或源文件变化，报错并停止继续，不会悄悄重新从零上传。服务端会话仅驻留内存，默认空闲有效期 30 分钟；客户端连接期间每 5 分钟查询暂停会话保活。

下载使用随机临时文件的实际长度作为偏移，每次刷新签名链接、校验源文件元数据，续传请求携带 `Range` 和可用的 `If-Range`。只有起点、终点和总长度吻合的 206 响应才能追加；忽略 Range 或校验标识变化时拒绝拼接。完成后同步并关闭文件，再发布到目标名称。

任务元数据存入与连接配置同目录的 `transfers.json`，使用临时文件、同步和替换保存。密码、令牌、目录密码及签名链接只在内存中保留。恢复任务先显示暂停，登录原服务器和账号后才允许手动继续；受保护下载重新索取密码。相同配置目录使用单实例锁，避免多个新版程序同时写记录。

传输状态为 queued/running/pausing/paused/cancelling/cancelled/failed/completed。两个并发槽位覆盖校验与传输。最近约 5 秒的字节增量用于速度和 ETA，恢复时重置基线，停顿超过 5 秒显示零速度。前端复用同一任务行，避免轮询时丢失按钮焦点。

## 验证范围

自动化测试覆盖密码哈希/OTP、子路径和分页请求、HTTP/业务错误、API 重定向隔离、包含中文和特殊字符的上传下载、非根账号路径、截断下载、下载重名保护、取消清理、并发约束、配置替换和本地文件名检查。桌面构建使用 Wails CLI。

## 后续可选方向

1. 使用系统凭据管理器实现安全的持久登录和多服务器配置。
2. 传输限速、网络恢复后的自动重试和更多存储驱动实测。
3. 目录递归下载、移动/复制、服务端索引搜索和文件预览。
4. 如需同步本机目录，应先设计冲突策略、删除策略、忽略规则和本地数据库，再引入文件系统监听。
5. 托盘、通知、安装包、自动更新和其他桌面平台构建。
