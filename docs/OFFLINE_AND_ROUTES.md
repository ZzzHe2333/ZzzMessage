# ZzzMessage：离线补发、局域网和独立页面路径

## URL 与反向代理

- / ：自动导航到 /login（已有有效登录时进入 /message）
- /login ：设置后端、注册与登录
- /message ：聊天页面和本地消息记录
- /friend ：联系人及发起私聊
- /setting ：个人设置和系统推送
- /gift ：礼物商城、选择收礼人和赠送历史
- /money ：三币种钱包和每日自动登录奖励（金币1%／银币9%／铜币90%，每次仅1枚）

这些页面是 Vue SPA 的客户端路由，而不是四个独立服务。Go 通过 WEB_DIR 提供静态 index.html，Docker 由 Caddy 提供 index.html，WebSocket 和 REST 都仍然通过 /api/* 转发给 Go。DDNSTO 只需代理一个站点及全部路径，不需要建立四个穿透入口。

例： http://192.168.0.192:43555/login、/setting、/message、/friend 都能直接输入或刷新。未登录访问需要授权的页面会跳转 /login。

## Windows 局域网与端口

Windows 新版 BAT：start-lan.bat 43555；本机模式 start.bat 默认端口 58463。LAN 模式监听 0.0.0.0:43555。将 .env 的 LAN_ORIGIN_CIDRS 设置为 10.0.0.0/8,172.16.0.0/12,192.168.0.0/16，可识别私网浏览器来源。公网 DDNSTO 域名仍需精确加入 ALLOWED_ORIGINS，例如 https://jinlangofficeb.ddnsto.com。

Windows 管理员 CMD 仅对 Private 网络中的 LocalSubnet 开启目标端口：

    netsh advfirewall firewall add rule name="ZzzMessage LAN 43555" dir=in action=allow protocol=TCP localport=43555 profile=private remoteip=localsubnet

测试：先访问 http://192.168.0.192:43555/api/health，再访问 http://192.168.0.192:43555/login。请替换为真实局域网 IP。LAN HTTP 的密码和聊天正文没有 HTTPS 传输加密，不适合不可信网络，也无法支持 Web Push 的安全上下文要求。

## Docker

默认仍绑定 127.0.0.1:58463。如需局域网直接访问，.env 设置 BIND_HOST=192.168.0.192、APP_PORT=43555；只在这个 IP 存在时才会正常启动。容器内 Caddy 监听 80，反代 /api/* 至 Go 容器:58463。公网使用可信 HTTPS 隧道/反向代理，避免直接暴露明文 HTTP。

## 离线正文与客户端留存

1. 用户向 Go WebSocket 发送 type=message。Go 为房间每个账户在 state.json 中暂存相同 ID 的消息正文；写盘失败则拒绝消息发送。
2. 在线客户端收到时、离线用户重新登录时都由 WebSocket 推送消息；重连会发送仍未 ACK 的队列。客户端按消息时间排序。
3. Web/iPhone PWA 首先将正文写入本机 IndexedDB，并以服务器、账号、房间和消息 ID 为键去重；写盘成功后才回发 {"type":"ack","id":"消息ID"}。
4. 服务器 ACK 只删除当前账户的待投递副本。每账户最多500条，14天补发期限。超过500条会淘汰最旧条目，超期不再补发；过期明文会在后续队列写操作中清理。
5. 网络中断后未确认的消息会重放，客户端按 ID 去重。多设备目前仍是账户级 ACK，因此同账号某台设备 ACK 后，其他离线设备可能无法获得该消息。
6. IndexedDB 聊天历史留在本机；主动清空当前会话、清理站点数据、无痕模式或卸载客户端可能删除它们。

隐私变化：state.json 包含未 ACK 的明文聊天正文，磁盘及备份都需要访问保护。当前不支持 E2EE 或云端永久聊天历史。

## 版本

本页描述 GitHub main 开发源码。此次按照用户先前约定未构建 Go/Vue、未运行测试，也未发布新版本；既有 v0.2.0-beta.1 不包含这些变更。

## 同账号设备间历史同步

新客户端登录后自动向其他在线的同账号设备请求 IndexedDB 历史记录；也可以在设置手动同步。历史逐片中转，每片收妥确认后继续，客户端按消息 ID 去重。本机制不同于云端永久存档：如果唯一拥有旧聊天记录的设备不在线，就必须等它上线；设备全丢失无法凭服务器恢复超过临时队列的历史。同账号一台设备接收并 ACK 实时消息，也不保证其他离线设备自动补收到消息，但设备同时上线后可相互补齐。

新增 /gift 和 /money 的经济系统详情参见 [HISTORY_AND_ECONOMY.md](HISTORY_AND_ECONOMY.md)。

## DDNSTO HTTP 消息兜底

确保单个穿透域名完整代理 /api/*：其中新增 GET /api/messages/pending、POST /api/messages/send、POST /api/messages/ack。该 HTTP 兜底不依赖 WebSocket Upgrade。详见 HISTORY_AND_ECONOMY.md。
