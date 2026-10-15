# 跨平台发行包说明

Go 后端支持 Windows x64、Linux x64、Linux ARM64 原生程序，以及 Docker Compose。前端 Web 和 iPhone PWA 均是独立编译的 Vue 静态文件。默认端口为 58463。

## 下载

打开仓库 Actions 页面，选择 Package (on request)，进入成功的一次运行，在 Artifacts 中下载 ZzzMessage-cross-platform-packages ZIP，解压后按操作系统选择二级安装包。

包含：ZzzMessage-Windows-amd64.zip、ZzzMessage-Linux-amd64.tar.gz、ZzzMessage-Linux-arm64.tar.gz、ZzzMessage-Web-static.zip、ZzzMessage-iPhone-PWA.zip、ZzzMessage-Docker-source.tar.gz、SHA256SUMS.txt。

## Windows

解压 Windows ZIP，进入 ZzzMessage-windows-amd64 文件夹；复制 .env.example 为 .env，用记事本填写至少 12 位 INVITE_CODE 与至少 32 位随机 SESSION_SECRET；在 PowerShell 运行 .\start.ps1。

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

Go 的 WEB_DIR 环境变量用于指定已编译的 Vue dist 目录。自带 start.ps1/start.sh 自动指定为解压包下的 web 文件夹。数据默认保存在 ./data/state.json 中，只保存账号、群组、推送设备/订阅元数据，不保存聊天正文。

Android 厂商推送需要开发者资质及官方 SDK，Android APK 和鸿蒙客户端不包含本次后端与 PWA 发行包。

## 构建策略

现有 CI workflow 仍为手动触发。Package workflow 只在显式请求构建的 build-request/** 分支推送或手动运行时执行。main 日常提交不会触发打包。

本次跨平台 Go 编译可以在 Linux GitHub Runner 上完成，但不等于已经在真实 Windows 系统、国产手机推送环境做过完整测试。
