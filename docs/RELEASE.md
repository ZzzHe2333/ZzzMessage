# 跨平台发行包说明

Go 后端支持 Windows x64、Linux x64、Linux ARM64 原生程序，以及 Docker Compose。前端 Web 和 iPhone PWA 均是独立编译的 Vue 静态文件。默认端口为 58463。

## 下载

打开仓库 Actions 页面，选择 Package (on request)，进入成功的一次运行，在 Artifacts 中下载 ZzzMessage-cross-platform-packages ZIP，解压后按操作系统选择二级安装包。

包含：ZzzMessage-Windows-amd64.zip、ZzzMessage-Linux-amd64.tar.gz、ZzzMessage-Linux-arm64.tar.gz、ZzzMessage-Web-static.zip、ZzzMessage-iPhone-PWA.zip、ZzzMessage-Docker-source.tar.gz、SHA256SUMS.txt。

## Windows

解压 Windows ZIP，进入 ZzzMessage-windows-amd64 文件夹；复制 .env.example 为 .env，用记事本填写至少 12 位 INVITE_CODE 与至少 32 位随机 SESSION_SECRET；双击 `start.bat`（仅本机），或双击 `start-lan.bat`（允许局域网访问）；PowerShell 仍可运行 `start.ps1`。

浏览器访问 http://127.0.0.1:58463 。Windows 包已经包含 Vue 前端静态文件，Go EXE 通过 WEB_DIR 直接托管网页，不需要安装 Node.js。默认仅监听本机回环地址。

## Linux

解压对应 CPU 架构的 tar.gz，进入 ZzzMessage-linux-amd64 或 ZzzMessage-linux-arm64，复制 .env.example 为 .env，填写邀请码与强随机 SESSION_SECRET，执行 ./start.sh。

Linux 包默认也只监听 127.0.0.1:58463，外部访问请通过安全隧道/HTTPS 反向代理，不要直接公网暴露 Go 的 HTTP 端口。

## Docker

解压 ZzzMessage-Docker-source.tar.gz，复制 .env.example 为 .env，填写 INVITE_CODE、SESSION_SECRET 和 ALLOWED_ORIGINS，然后由管理员自行运行 docker compose up -d --build 。

Docker 版本继续使用 Go 服务 + Caddy Vue 前端两容器架构，并将宿主机 127.0.0.1:58463 转发至前端。

## iPhone

ZzzMessage-iPhone-PWA.zip 是独立 ios-pwa/ 编译的静态网页。将 ZIP 内容部署至 HTTPS 站点，Safari 添加到主屏幕后才能使用 iPhone Web Push。后端必须允许该前端 origin，并配置 VAPID 密钥。

## 配置与安全

原生 Go 可以直接读取当前工作目录的 .env 文件；进程环境变量优先。.env 不包含在发行包中，只有 .env.example，所有密钥需管理员自己配置。

Go 的 WEB_DIR 环境变量用于指定已编译的 Vue dist 目录。自带 start.ps1/start.sh 自动指定为解压包下的 web 文件夹。数据默认保存在 ./data/state.json 中，包含账号、群组、推送订阅，以及每账号最多500条未 ACK 的**明文离线聊天正文**（14天补发窗口）。

Android 厂商推送需要开发者资质及官方 SDK，Android APK 和鸿蒙客户端不包含本次后端与 PWA 发行包。

## 构建策略

现有 CI workflow 仍为手动触发。Package workflow 只在显式请求构建的 build-request/** 分支推送或手动运行时执行。main 日常提交不会触发打包。

本次跨平台 Go 编译可以在 Linux GitHub Runner 上完成，但不等于已经在真实 Windows 系统、国产手机推送环境做过完整测试。

## Windows BAT 局域网测试

- 将发行包内的 `start.bat` 和 `start-lan.bat` 放在 `ZzzMessage.exe` 同级目录（旧版发行包可从仓库 `scripts/` 下载 BAT 文件后重命名）。
- `start.bat` 默认只绑定 `127.0.0.1:58463`；`start-lan.bat` 绑定 `0.0.0.0:58463`。BAT 不受 PowerShell 的 .ps1 执行策略影响。
- 在 `.env` 的 `ALLOWED_ORIGINS` 中加入你实际的局域网 IP，例如 `http://192.168.0.192:58463`；重启程序后生效。
- 以管理员身份运行 CMD，并为专用网络添加仅限本地子网的入站防火墙规则（先确认本机网络配置文件为 Private）：

  ```cmd
  netsh advfirewall firewall add rule name="ZzzMessage LAN" dir=in action=allow protocol=TCP localport=58463 profile=private remoteip=localsubnet
  ```

- 然后其他设备访问 `http://你的电脑局域网IP:58463/api/health`，若成功再访问 `http://你的电脑局域网IP:58463`。
- 局域网 HTTP 可用于短期聊天连通性测试，**不支持 iPhone PWA 的安全上下文/系统推送**，正式对外使用应配置可信 HTTPS。

## main 新增特性（尚未构建/发行）

新增 `/login`、`/message`、`/friend`、`/setting` SPA 路由和离线补发；Windows 可使用 `start-lan.bat 43555` 自选端口，Docker 可使用 `APP_PORT=43555` 和 `BIND_HOST=指定局域网IP`。详见 [OFFLINE_AND_ROUTES.md](OFFLINE_AND_ROUTES.md)。v0.2.0-beta.1 发行包仍为旧版。

本次修复中，Apple 推送返回非 2xx 时会在日志中显示 Apple 的安全错误原因（如 BadJwtToken），前端也会查询发送状态；Edge 本地注册需确认 localhost 与 127.0.0.1 均在精确 ALLOWED_ORIGINS 白名单中。详见 [PUSH.md](PUSH.md)。


## 外部 Key 免登录消息 API

main 源码增加 `POST /api/external/send`，需要独立 `EXTERNAL_SEND_KEY`（至少32位随机密钥）、`EXTERNAL_SEND_SENDER`（已注册专用机器人账号），可选 `EXTERNAL_SEND_RECIPIENTS` 限制发送目标。启动时检查配置，未配置时禁用。Docker Compose 同步透传以上三个环境变量。参见 [EXTERNAL_API.md](EXTERNAL_API.md)。

**该接口在重新编译/打包新版程序前不可使用；旧 Windows EXE 不包含该路由。**


## Telegram 风格 UI、群聊修复和附件（未发布源码）

Windows/Linux/Web/iPhone PWA 源码增加搜索、会话资料、Emoji 和群成员同步；文件上传接口为 `POST /api/files/upload`，按 multipart/form-data 提交 `roomId` 与 `file`，下载使用 `GET /api/files/{id}` 并在请求头携带会话用户的 Bearer Token。默认 10 MiB/文件，数据在 `DATA_DIR/uploads` 中持久保存，备份时不能只复制 state.json。群创建后成员列表通过 /api/messages/pending 中的 rooms 同步。

Windows Edge Web Push WNS 地址 `*.notify.windows.com` 已列入允许域名；更换过服务端或 VAPID 密钥的设备需要先通过应用按钮取消旧订阅，再开启新订阅。详情见 [TG_UI_FILES.md](TG_UI_FILES.md)。

开发版新增会话消息摘要、未读/置顶、本地搜索与草稿、持久引用回复、跨设备 Emoji 回应以及服务器持久化的会话静音（/api/rooms/mutes）。图片/音视频预览需从登录客户端授权取回附件，不暴露静态文件路径。新的 Web 与 iPhone PWA 已在 npm test 中加入状态逻辑回归测试，部署前需运行完整构建和设备实测。详情见 TG_UI_FILES.md。
