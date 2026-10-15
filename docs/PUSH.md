# ZzzMessage 跨平台 PWA 与 iPhone 系统通知

本项目统一使用 Vue PWA 客户端（Web、Android 和 iPhone 主屏幕），Go 后端负责账号、群聊信息、实时转发、推送订阅及离线消息待投递队列。当前不是原生 APK/IPA，尚无原生 APNs 证书或 iOS Xcode 工程。

## 配置后端推送

1. 在可信的管理员终端获取仓库源码，并主动执行生成器：

       go run ./cmd/vapid

2. 在部署目录的 .env 中填写输出的三个设置，务必把联系邮箱改为真实可用地址：

       VAPID_PUBLIC_KEY=生成器输出的公钥
       VAPID_PRIVATE_KEY=生成器输出的私钥
       VAPID_SUBJECT=mailto:admin@example.com

3. 保持 VAPID_PRIVATE_KEY 仅在后端环境变量或受保护的 .env 中；不要提交到仓库，不要放进 Vue 代码。私钥更换后，旧设备需要重新订阅推送。VAPID_SUBJECT 支持 mailto: 或 https://。
4. 为前端配置可信的 HTTPS 域名；在 ALLOWED_ORIGINS 中添加它的精确 origin（含协议、域名、非标准端口）。如 https://chat.example.com。若前后端分域名部署，后端必须允许前端站点跨域。
5. Go 服务器需要能主动访问 HTTPS 推送服务端点：iPhone 通常是 *.push.apple.com；Android Chrome 通常为 fcm.googleapis.com；Firefox 通常为 updates.push.services.mozilla.com。API 只允许这些端点，避免订阅链接造成任意服务器请求。
6. 由管理员按需主动部署更新的版本。本仓库已关闭 push/PR 自动构建，只能手动运行 CI。设置为空时，聊天功能继续可用，推送功能将显示未配置。

## iPhone 开启通知（无需苹果开发者年费）

1. iPhone 使用 iOS 16.4 或更新系统；Safari 通过 HTTPS 访问已部署的 PWA 前端。
2. 在 Safari 点「分享」→「添加到主屏幕」，在手机桌面生成 ZzzMessage 图标。
3. **从主屏幕图标进入**（不是 Safari 标签页），连接后端并登录。
4. 打开聊天界面的「设置」→「开启本设备通知」，在系统弹窗中允许通知。权限申请必须由用户主动点击按钮触发。
5. 点击「发送测试通知」，检查 iOS 设置 → 通知 → ZzzMessage；如未弹出，检查专注模式、网络、VAPID 密钥和后端访问 *.push.apple.com 的能力。

iOS 主屏幕 Web Push 经 Apple 的推送基础设施送达，但使用的是浏览器标准 VAPID 协议，无需 Apple Developer Program 年费、苹果 APNs p8 密钥或 APNs 证书。苹果官方要求收到 Web Push 时显示可见通知，本项目始终显示通用文本。

## Android 与 Web

- Android：Chrome/支持 Push API 的浏览器打开 HTTPS 站点，可添加到主屏幕；登录后在设置中点击开启系统通知，并允许网站通知。
- Windows/macOS/Linux Web：支持 Push API 的现代浏览器访问 HTTPS 站点，登录后在设置启用通知。
- 本地 Vite 开发服务器默认为 58464，生产 PWA 默认为通过宿主机 58463 的 HTTPS 代理访问。开发服务器默认并不注册生产 Service Worker；请使用 HTTPS 生产版联调真实推送。
- 无法在当前浏览器请求到 PushManager、Service Worker 或安全上下文时，按钮会禁用并提示原因。

## 非云端消息存储的限制

- 后端暂存尚未 ACK 的聊天正文（明文，每账号最多500条，14天补发期），另保存账号、群组和推送订阅信息。
- 消息发来时，Go 后端将非敏感、通用的提醒推送至订阅的设备。推送正文固定为「收到一条新消息」，不包含任何私聊正文、昵称或群组名称。
- 离线时可能收到系统通知；重新连接 WebSocket 后会补发队列中的聊天正文，并在 IndexedDB 保存成功后 ACK。点击通知只打开应用。
- 同一来源安装的 PWA 当前最多保留一个当前 PushSubscription。要切换到另一台使用不同 VAPID 密钥的后端，请在旧后端先关闭/重置推送，切换后再开启。若旧后端已无法连接，残留的服务器订阅可能要等推送服务返回失效结果后清理。
- 后台通知依赖操作系统、浏览器、推送提供商和网络环境，不能保证实时或始终到达；换设备前要单独在新设备开启权限。
- 当前消息还不是端到端加密，本地 IndexedDB 记录也未加密，不应当直接用于高敏感数据或未经验证的公网生产环境。

## 参考资料

- Apple 官方：https://developer.apple.com/documentation/usernotifications/sending-web-push-notifications-in-web-apps-and-browsers
- WebKit：https://webkit.org/blog/13878/
- MDN：https://developer.mozilla.org/en-US/docs/Web/API/PushManager/subscribe

## 多设备历史同步说明

同一账户多个客户端上线后，会通过 Go WebSocket 中继同步各客户端的本地 IndexedDB 记录，前提是至少一台旧设备仍保有记录且在线。服务器不会长期保存完整历史。推送服务仍只发送固定提醒，不包含礼物或聊天正文。


## 推送被 Apple 拒绝：HTTP 403

Apple Web Push 的 HTTP 403 是认证失败，不代表「已入队」就是发送成功。
修复版服务端会记录类似：
\`web push status 403 reason="BadJwtToken" apns-id="..." for user "..."\`
并且前端发送测试推送后会轮询 /api/push/test-status 显示结果，不再把已入队当作已送达。
- \`BadJwtToken\`：检查 VAPID_SUBJECT 是否为**真实有效的**邮箱或 HTTPS URL、服务器时间是否正确、VAPID 签名密钥是否与浏览器订阅的公钥匹配。
- \`BadVapidPublicKey\` / \`BadAuthorizationHeader\`：检查 VAPID 公钥编码及 HTTP 认证头。
- HTTP 404 / 410：旧订阅可能已失效，应在设备上重新开启通知。
- HTTP 201：Apple 接受推送请求，但设备是否展示通知仍受 iOS 通知权限和网络影响。

内网穿透使用 HTTPS 时，可将 VAPID_SUBJECT 配成你的**实际** HTTPS 站点（例如 \`https://jinlangofficeb.ddnsto.com\`），无需改公私钥。不要长期使用示例地址 \`mailto:admin@example.com\` 或 localhost 作为联系地址。仅修改 VAPID_SUBJECT 不要求重新订阅；如果**更换密钥**，需要清除旧订阅并重新在 iPhone PWA 上允许推送。

## Edge 本地访问无法注册

注册与登录 API 会校验请求的浏览器 Origin（协议 + 域名/IP + 端口）。
假如浏览器分别访问 localhost、本机回环和 DDNSTO，必须覆盖相应来源：

\`\`\`dotenv
ALLOWED_ORIGINS=http://localhost:58463,http://127.0.0.1:58463,https://jinlangofficeb.ddnsto.com
LAN_ORIGIN_CIDRS=10.0.0.0/8,172.16.0.0/12,192.168.0.0/16
\`\`\`

如果用户通过其他域名或端口登录，需根据实际浏览器地址栏补充，并重启 Go 后端；不要使用 \`*\` 或禁用来源校验。若注册提示「邀请码错误」或「用户名已存在」，应分别处理邀请信息和账号冲突；它们不是网络故障。

## 通知正文预览与 HTTP 消息兜底

新版默认在系统通知中展示发件人、消息摘要，可能显示在锁屏。用户可在 /setting 关闭预览。Web Push 服务提供商能够传输通知内容；当前不是端到端加密。即便 DDNSTO 的 WebSocket Upgrade 未正确连接，前台客户端仍可通过经过认证的 HTTP API 获取离线消息。详见 HISTORY_AND_ECONOMY.md。


## Edge/Android Push 订阅兼容性（main 开发版）

合法的 Web Push endpoint 会使用不同提供商的 HTTPS 域名：Edge 常见 `*.notify.windows.com`，Chrome/Android 使用 `fcm.googleapis.com`，Firefox 为 `*.push.services.mozilla.com`，Safari 为 `*.push.apple.com`。服务端现在校验这些特定域名后保存订阅，不开放任意公网或私网 URL，避免 SSRF。

如出现「开启失败：推送订阅无效或推送服务不受支持」，先确认浏览器地址为有效 HTTPS、系统通知权限已允许、后端 VAPID 公钥与本设备订阅一致，再在设置执行「重置本机推送」，随后手动点击「开启本设备通知」。重置本机推送不会清除 IndexedDB 聊天历史。此项不意味着普通 Android WebView 自动具备浏览器 PushManager；原生通知仍依赖对应 Android 推送桥/厂商 SDK。

群内聊天文字、附件在服务端创建完成后会走同一套系统通知逻辑（用户可关闭预览）；群创建本身不产生通知。
