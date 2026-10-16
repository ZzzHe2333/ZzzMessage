# ZzzMessage 管理员后台与后端更新

## 功能

- 登录后通过 `/admin` 访问管理员后台（普通用户无法访问 API，前端也不显示入口）。
- 查询原生后端编译版本、构建时间、操作系统、最近一次更新结果。
- 查询 **ZzzHe2333/ZzzMessage 官方 GitHub Release**，只显示具备 Windows x64 安装包及 SHA256SUMS 的版本。测试版会单独标注。
- Windows 原生版经人工确认后，自动下载、校验 SHA-256、暂存、备份当前程序与前端、切换程序、重启并健康检查。失败时尝试回滚旧程序与旧网页。
- **不接受用户上传任意程序、任意下载 URL、Shell 命令或任意版本标签**，不会通过网页直接执行用户提供的命令。
- Docker / Linux：可查看状态和版本，但**暂不支持后台直接替换运行中的服务**，需要使用各自部署流程更新。

## 首次启用（Windows）

先在 ZzzMessage 注册你自己的管理员账号，例如 `admin_owner`。然后在原有安装目录的 `.env` 中加入：

```dotenv
ADMIN_USERNAME=admin_owner
```

保存后重启 ZzzMessage。**必须使用已经注册的用户名**，否则为了防止未认领用户名被人注册后夺取管理员权限，服务端会拒绝启动。留下空值代表管理员功能完全关闭。

使用该账号登录，打开菜单里的「管理员后台」，也可以访问 `https://jinlangofficeb.ddnsto.com/admin`。管理员接口需要已有 Session Token，普通账号无法执行。

Windows 目录中必须有 `ZzzMessage.exe`、`ZzzMessage-Updater.exe`、`web/`，且配置 `WEB_DIR` 指向安装目录下的 `web`。新 ZIP 中包含更新辅助程序；**旧 ZIP 没有该程序时必须先手动更新一次**。

## 更新操作

1. 先备份整个 `data`、`.env` 目录（包括 `data/uploads`）。
2. 管理员后台点「刷新状态与版本」获取官方发布版本。
3. 选择版本、确认 Release 页面及发布日期，输入明确的 `UPDATE v...` 确认短语。
4. 服务器只会下载 `https://github.com/ZzzHe2333/ZzzMessage/releases/download/<版本>/ZzzMessage-Windows-amd64.zip` 和同版 `SHA256SUMS.txt`，严格检查 URL、文件总大小、ZIP 文件路径、PE 文件头以及 SHA-256 校验。
5. 更新辅助程序等待旧服务器关闭，将旧 `ZzzMessage.exe`、`web` 移入 `data/updates/backups/<时间戳>/`，另存 `state.json` 快照，安装新后端与前端并自动启动。用户连接会短暂中断。
6. 新服务健康检查失败时，更新辅助程序尝试将旧版文件原位恢复并重新运行旧服务。结果写入 `data/update-result.json`，重新登录后台即可查看。

**无法降级：** 本构建包含编译时间，早于该时间的 Release 会被拒绝。当前仓库曾发布过 v0.2.0-beta.1 与 v0.3.0-beta.1；若它们比当前本地编译时间旧，页面可能仍列出，但**点击更新会明确报错并拒绝降级**。等后续发布较新的 Release 才可以使用自动更新。

## 安全提醒

- 仅用 HTTPS 进入互联网可访问的管理员页面；保持 `SESSION_SECRET`、管理员密码足够强，并确保 `ALLOWED_ORIGINS` 只包含预期域名。
- 官方 Release 与校验文件由同一 GitHub 项目提供。SHA-256 可检验传输/包一致性，但不等于独立发布者代码签名。请在安装前确认发布来源与内容可信。
- 更新过程可能因 Windows 文件占用、磁盘空间或防火墙等失败，回滚也是尽力操作，不能替代自己的离线备份。
- **不会覆盖** `.env`、`data/state.json`、`data/uploads`；状态快照含账号散列、离线消息等敏感数据，需要限制目录访问。
- 本次更新不支持 Docker 容器内自动重建、Linux systemd 服务重启或在管理员界面更新 MCP 独立程序。
- 自动更新只消费 GitHub 已发布的 Release。仅提交 GitHub main 或 GitHub Actions 的临时 artifact，不会被视为可安装的后端更新。

## API

- `GET /api/admin/me`：所有登录账号可查看 `{"admin":false|true}`，用于显示菜单。
- `GET /api/admin/status`：管理员读取平台、版本、构建时间、支持情况和最近更新结果。
- `GET /api/admin/releases`：管理员查询官方 Release。
- `POST /api/admin/update`：管理员发送 JSON `{"tag":"v0.4.0","confirmation":"UPDATE v0.4.0"}`，经严格校验后启动更新并关闭原服务，返回 HTTP 202。

