# ZzzMessage

**跨平台发布：** Windows / Linux 原生 Go 二进制可以直接运行，并使用可选 WEB_DIR 在同一个端口提供编译后的 Vue 页面；Docker Compose 原有双容器部署方式保持兼容。安装与下载见 [docs/RELEASE.md](docs/RELEASE.md)。此版本的独立 iPhone PWA 另行打包，Android APK 不在本次构建范围内。


## 多端源码布局

- Android 原生 APK 工程：[android/](android/)（Xiaomi、vivo 厂商推送，尚未编译）
- iPhone PWA 独立源码：[ios-pwa/](ios-pwa/)
- 普通网页版：[web/](web/)
- 鸿蒙占位目录：[harmonyos/](harmonyos/)（没有客户端实现）
- Go Android 通知协议说明：[docs/ANDROID_NATIVE_PUSH.md](docs/ANDROID_NATIVE_PUSH.md)


一个供朋友使用、可自行选择后端的开源私人聊天 PWA。**Go 1.23 + Vue 3 + WebSocket + IndexedDB**。

> **早期可测试版本（MVP），尚未进行外网生产安全审计。**
>
> **现在服务器会临时持久化未 ACK 的聊天正文（明文），并不是端到端加密（E2EE）。** 每账号最多500条，14天补发期；客户端完成本地存储并 ACK 后，删除该账号的待投递副本。

## 已实现

- 邀请码注册，bcrypt 密码哈希，7 天 HMAC 签名登录令牌。
- 可手动填写 HTTPS 后端地址，校验后自动记住，下次打开继续使用。
- Go WebSocket 实时聊天：好友大厅、两人私聊、最多 30 人的邀请制小群。
- 按用户/服务器/房间隔离本地 IndexedDB 消息记录；服务器还会持久保存**尚未 ACK 的聊天正文**。
- 离线用户重连时自动补发待投递消息，浏览器持久保存到 IndexedDB；同账号在一台设备上 ACK 后，其他离线设备可能无法再补收。
- 断开后自动重连；Web PWA 支持 iPhone/Android 添加到主屏幕。
- 可选 Web Push 系统通知：iPhone 主屏幕、Android 与桌面 Web，按账号保存推送订阅，使用 VAPID 密钥验证，通知只发送通用提醒。
- Origin 白名单、JSON 请求限制、邀请制注册、简单的认证请求频率限制。
- Docker Compose 运行，提供测试接口和 Go 测试。

未实现：服务器永久云端历史及无在线旧设备时的完整历史恢复、文件/图片/语音、原生 APNs 通知（PWA 已支持 Web Push）、消息端到端加密、客户端本地静态加密、登录会话吊销、管理员管理后台、审核/举报系统、机器人 API。**不要把当前 MVP 当成已满足公开运营或应用商店上架要求的产品。**

## 快速启动（Docker，Ubuntu / Windows Docker）

```bash
git clone https://github.com/ZzzHe2333/ZzzMessage.git
cd ZzzMessage
cp .env.example .env
# 编辑 .env：邀请码和 SESSION_SECRET 必须替换！
# 可以使用 openssl rand -hex 32 生成随机密钥和邀请码
docker compose up -d --build
```

本机访问 `http://localhost:58463`，填写同一地址或直接点「验证并固定服务器」。第一次使用选择邀请码注册。

Docker 默认仅绑定 `127.0.0.1:58463`（`.env` 中 `APP_PORT` 可改端口、`BIND_HOST` 可绑定特定局域网网卡；Go 容器内部仍使用 58463），**不会直接向整个公网开放**。其他设备必须通过你设置的 HTTPS 反向代理/私有组网访问，并将公网前端来源（如 `https://chat.example.com`）加入 `.env` 中的 `ALLOWED_ORIGINS`，修改后重建容器：`docker compose up -d --force-recreate`。

公网 TLS 可以由你自己的 Caddy/Nginx/Tailscale Serve/Cloudflare Tunnel 等提供。**iPhone PWA 与浏览器跨域连接需要有效的 HTTPS 证书**；纯 IP HTTP 不适合远程 PWA。使用反向代理时，将站点请求转发到宿主机本地 `127.0.0.1:58463`；公开的只应是 HTTPS 站点，不要同时绕开 TLS 暴露后端 58463 端口。

## 本地开发

```bash
# 终端1（在项目根目录，先设置随机强密钥）
export INVITE_CODE="replace_with_your_secret_invite"
export SESSION_SECRET="replace_with_at_least_32_random_characters"
export ALLOWED_ORIGINS="http://localhost:58464,http://localhost:58463"
go run ./cmd/server

# 终端2
cd web
npm install
npm run dev
```

打开 `http://localhost:58464`，输入后端 `http://localhost:58463`。

## 后端 API

| 方法 | 路径 | 功能 |
|---|---|---|
| GET | `/api/health` | 版本、兼容性检测 |
| POST | `/api/auth/register` | 邀请注册：`username,displayName,password,inviteCode` |
| POST | `/api/auth/login` | 登录：`username,password` |
| GET | `/api/me` | 当前用户 |
| GET | `/api/users` | 服务器其他用户 |
| GET | `/api/rooms` | 当前用户参与的会话 |
| POST | `/api/direct` | 与用户私聊：`username` |
| POST | `/api/rooms` | 创建小群：`name,members` |
| GET | `/api/ws` | WebSocket 连接，第一帧 `{ "type":"auth", "token":"..." }` |

REST 使用 `Authorization: Bearer <token>`。WS 发消息：

```json
{"type":"message","roomId":"lobby","text":"你好"}
```

客户端收到：

```json
{"type":"message","id":"server-generated-id","roomId":"lobby","sender":"alice","text":"你好","createdAt":"2026-10-15T00:00:00Z"}
```

服务器仅广播给**当前在线、具有房间权限**的连接；不会在重连时重放消息。私聊成员由后端检查，不能通过修改前端的 roomId 绕过授权。

## 数据与安全限制

- `DATA_DIR/state.json` 保存用户名、昵称、bcrypt 哈希和房间成员数据；没有任何聊天正文持久化逻辑。状态文件权限 `0600`，目录默认 `0700`。备份此目录时仍要保护账户隐私。
- 前端 IndexedDB 会持久化**明文**聊天记录；同一设备上可以从浏览器开发者工具查看。个人设备启用系统锁屏和磁盘加密。若要存敏感信息，后续必须实现审计过的端到端加密和本地加密方案。
- 服务器可见连接 IP、消息内容（转发期间）及房间成员。零历史存储 **≠** 匿名、零元数据、端到端加密。
- 令牌在当前 MVP 存在浏览器 localStorage，存在 XSS 后被盗取的风险；使用 CSP、避免第三方脚本与富文本渲染，公开互联网使用前应强化会话管理。
- 不支持 iPhone 原生离线推送；关闭网页/设备离线期间没有消息补发。
- 邀请码是服务端部署者共享的秘密，别放进 GitHub。公开部署时还应配置反向代理限流、监控、HTTPS、安全更新与定期备份。

## 测试

```bash
go test ./...
cd web && npm run build
```

项目开源并不等于已取得在中国大陆公众提供即时通讯服务所需的备案/经营资质。本项目首先定位为少量熟人自托管测试用途。

## 端口与构建约定

- **默认宿主机端口：58463/TCP**（回环地址 127.0.0.1；可在 `.env` 设置 `APP_PORT`），Go 容器端口：58463，Caddy 容器内部端口：80（仅 Docker 内部使用），Vue 开发服务器：58464（不用于生产）。
- 端口采用高位私有/动态范围，降低与常见开源项目的默认端口冲突概率；不能保证当前宿主机绝对未占用。启动前执行 `ss -ltn '( sport = :58463 )'` 检查本机 TCP 监听，或者用 `APP_PORT` 改端口。
- 已有部署请检查旧 `.env` 中 `ALLOWED_ORIGINS`，把本地默认 8080/5173 更正为新的 58463/58464；如果对外使用独立 HTTPS 域名，域名来源照旧保留。
- **GitHub Actions 仅支持手动 `workflow_dispatch`，push/PR 不会触发 Go 或 Vue 构建**。此仓库不会仅因代码提交而自动构建镜像或产物。README 中的构建/启动命令仅供部署人员主动执行。

## Web / Android / iPhone 系统通知

完整配置步骤（含 iPhone 安装、VAPID 密钥、网络放行、测试方法和安全限制）请看 [docs/PUSH.md](docs/PUSH.md)。

## Android 原生系统通知

首批 Xiaomi 与 vivo 的设备注册及厂商发送代码已加入，SDK AAR、开发者凭证需要由部署者自行取得，尚未编译和真机验证。配置见 [android/README.md](android/README.md)。除非用户明确要求，仓库不自动执行 Android、Go 或 Vue 构建。

## 新版 main：离线补发与页面路由

- `/` 自动跳转 `/login`；登录后 `/message`，联系人 `/friend`，设置 `/setting`，礼物 `/gift`，钱包 `/money`。
- Go 单进程与 Docker Caddy 都支持四个路径的直接访问和刷新，API 与 WebSocket 依旧走 `/api/*`。
- 私有网段 Origin 通过 `LAN_ORIGIN_CIDRS` 放行；DDNSTO 公网 HTTPS 域名仍通过 `ALLOWED_ORIGINS` 精确指定。
- 服务端明文待投递消息至多每账号500条、14天补发期，客户端 IndexedDB 保留本地已收到的聊天正文。
- Windows 新版 BAT 支持 `start-lan.bat 43555`，Docker 支持 `APP_PORT=43555`。
- 查看 [离线补发及路由部署指南](docs/OFFLINE_AND_ROUTES.md)。

**已发布的 v0.2.0-beta.1 是旧版，新代码尚未编译、测试或重新发行。**

## 多设备本地历史同步与虚拟礼物（main 最新源码）

同账号设备的 Web / Android WebView / iPhone PWA 建立 WebSocket 连接后，会发起 **点对点历史同步请求**，由 Go 内存中继将旧设备的 IndexedDB 历史记录分片传给新设备，新设备本地持久化并确认每个分片。服务端不建立永久聊天历史数据库。**如旧设备没有在线、其本地记录被删除或其账户消息已经超过临时离线队列期限，则无法完整补齐历史。** 原有 500 条/14 天临时待投递机制继续有效。更多说明见 [设备同步与三币种礼物系统](docs/HISTORY_AND_ECONOMY.md)。

新增 `/money` 三币种虚拟钱包：金币、银币、铜币。每天首次成功登录自动随机获得 **1 枚货币：金币 1%、银币 9%、铜币 90%**。日期以 UTC 为准；同账号跨设备或反复登录，当日只发放一次（与现实货币无关）。`/gift` 可使用对应币种直接购买礼物并赠送站内其他用户，包含防重复扣费的请求 ID、交易记账和收礼通知；当前没有充值、提现、真实支付或现金价值。

**这些新功能目前仅在 GitHub main 源码中，尚未重新构建、测试和发布；已发布的 v0.2.0-beta.1 不包含它们。**


## 外部 API Key 直接发送私聊（Hermes / 定时任务）

支持独立接口 `POST /api/external/send`。调用方不必登录，只需提交 `Authorization: Bearer <EXTERNAL_SEND_KEY>`、JSON 的 `recipient` 与 `text`。服务器固定使用 `EXTERNAL_SEND_SENDER` 指定的**已注册机器人账号**发送私聊，自动创建与收件人的私聊房间，并走正常的 IndexedDB/离线队列、WebSocket 和通知投递。此 Key **仅可发送消息，不能调用登录态 API 或查询聊天历史**。

将独立随机密钥及发送人添加到 .env（可通过 `EXTERNAL_SEND_RECIPIENTS` 限定收件人），未配置时接口返回 404、不影响原服务。详见 [外部发送接口使用指南](docs/EXTERNAL_API.md)。这些设置当前仅在 main 源码实现，原先打包的 EXE 不自动升级。


## Telegram 风格界面、群聊同步与文件附件（main 开发版）

桌面版改为可展开的三栏布局：会话搜索/菜单、当前会话、右侧会话资料；手机端会话列表与聊天内容分屏切换。浏览器标签会随会话名称变化。新增 Unicode 表情选择器和会话文件（单个最多 10 MiB）。

**重要：** 文件字节保存在服务器 `DATA_DIR/uploads`，附件元数据存在 `DATA_DIR/state.json`；下载必须携带登录令牌，服务端验证请求人属于相应会话。上传消息使用现有离线队列/系统通知通道，老纯文本客户端仍能看到 `[文件] 文件名`。本地存储清理不删除服务器上的已上传文件。当前附件容量上限为整个服务器 2000 个文件（不是总磁盘配额），请定期备份文件目录与 state.json。

Windows Edge 推送订阅现允许经安全 HTTPS 的 Microsoft WNS `*.notify.windows.com`；Chrome Android 的 FCM 及 Firefox/Apple 推送端点继续支持。浏览器更换 VAPID 公钥后需在设置中先「重置本机推送」，再点击「开启本设备通知」，不建议清空整个站点数据，因为 IndexedDB 的聊天历史可能因此丢失。

群聊创建成功会自动进入群，群成员在 WebSocket 无法正常升级时也会通过 HTTPS 消息轮询获得最新群列表，成员文字和文件消息都进入离线队列。创建群这一动作本身不发送系统通知。

详见 [TG 界面、附件与跨浏览器推送说明](docs/TG_UI_FILES.md)。**main 代码不同于已发布的 v0.3.0-beta.1；在打包/部署新的测试版前，原有 EXE 不会自动获得这些功能。**

本地消息搜索、未读/置顶、引用回复、文字转发、Emoji 回应和服务器同步的会话通知静音已接入 main。详见 [Telegram 风格功能对照与限制](docs/TG_UI_FILES.md)。

手机端已增加专用 Telegram 风格布局、底部导航与 PWA 版本更新检查；更新服务器 web/ 后在手机版设置页核对 UI 2026.10.16-mobile.1。详情见 [移动端部署与缓存更新说明](docs/TG_UI_FILES.md)。

新增个人 API Key 管理页 `/api-key`，登录用户可直接生成并撤销独立免登录消息发送 Key，配置允许发送的收件人。详见 [外部发送接口说明](docs/EXTERNAL_API.md)。
