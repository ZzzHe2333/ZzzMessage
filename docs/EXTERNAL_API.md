# ZzzMessage 外部 Key 免登录发送接口

## 行为

供 Hermes Agent、Shell 脚本、CI 等自动化工具使用。调用方无需执行 ZzzMessage 注册或登录流程，也无需登录会话 token。调用时只需要专用 API Key。

- 仅允许 POST /api/external/send，使用 HTTPS。
- HTTP Header: Authorization: Bearer <EXTERNAL_SEND_KEY>。
- Content-Type: application/json。
- 请求：{"recipient":"cjz","text":"你的提醒正文"}。
- 机器人发送人的用户名从服务端 EXTERNAL_SEND_SENDER 读取，不接受来自请求的 sender/roomId 自定义字段，避免任意身份伪造。
- 收件人必须是已注册的用户名。若首次发送，系统自动创建机器人与此用户的专用私聊；随后消息持久进入收件人的离线队列，在线则通过 WebSocket/HTTPS 客户端接收，通知按现有 VAPID 设置发送。
- 成功返回 201 JSON，包括 ok、messageId、roomId、recipient、createdAt；Key 不会出现在响应中。
- API Key **只能发送**，不能使用 GET /api/me、GET /api/messages/pending 或任何会话认证操作。
- 不配置 EXTERNAL_SEND_KEY 和 EXTERNAL_SEND_SENDER 时接口保持关闭（404）。

## 第一次配置（Windows .env 或 Docker）

1. 在聊天网页通过现有邀请码正常注册一次专用账号，例如 hermes_bot；以后外部调用无需登录该账号。
2. 在服务器上本地生成新的独立随机 Key（不要重复使用 SESSION_SECRET、邀请码或 VAPID_PRIVATE_KEY）：

    openssl rand -hex 32

也可 Windows PowerShell 在本机生成随机 Key（从安全随机字节转为十六进制）：

    $bytes = New-Object byte[] 32
    $rng = [Security.Cryptography.RandomNumberGenerator]::Create()
    $rng.GetBytes($bytes)
    $rng.Dispose()
    $key = -join ($bytes | ForEach-Object { $_.ToString('x2') })
    $key

3. 服务器 .env 添加：

    EXTERNAL_SEND_KEY=请填写上一步生成的64位十六进制密钥
    EXTERNAL_SEND_SENDER=hermes_bot
    EXTERNAL_SEND_RECIPIENTS=cjz

EXTERNAL_SEND_RECIPIENTS 可填写多个用户名，用英文逗号分隔，例如 cjz,alice；不填写代表允许发送给任意**已注册账号**。设置后重启 Go 后端。Docker compose 版本已透传这三个变量。

4. 调用示例（PowerShell，避免中文被旧控制台字符集错误转换）：

    $key = $env:ZZZMESSAGE_SEND_KEY
    $uri = 'https://jinlangofficeb.ddnsto.com/api/external/send'
    $json = @{ recipient = 'cjz'; text = 'Hermes 自动提醒：测试消息' } | ConvertTo-Json -Compress
    Invoke-RestMethod -Uri $uri -Method Post -Headers @{ Authorization = "Bearer $key" } -ContentType 'application/json; charset=utf-8' -Body ([Text.Encoding]::UTF8.GetBytes($json))

其中 $env:ZZZMESSAGE_SEND_KEY 是调用方的环境变量，应通过安全的密钥管理途径分发，不必使用与服务器完全相同的变量名。实际域名应替换为你的 HTTPS 内网穿透地址。

5. 预期响应示例：

    {"ok":true,"messageId":"1a2b...32位...","roomId":"dm_...","recipient":"cjz","createdAt":"2026-10-15T00:00:00Z"}

## 错误代码

| HTTP | 含义 |
|---|---|
| 201 | 消息已写入持久化待投递队列，继续尝试实时推送/系统通知 |
| 400 | 用户名/消息正文无效，或带了不支持的 JSON 字段 |
| 401 | 未带 Bearer Key、Key 错误；普通登录 Token 不能代替此 Key |
| 403 | 目标不在 EXTERNAL_SEND_RECIPIENTS 授权名单内 |
| 404 | API 未启用，或目标用户不存在 |
| 415 | 请求不是 application/json |
| 429 | 超出服务端发送频率限制（当前每个 API 发送账号最多 25 条/10秒） |
| 500 | 创建私聊或持久化消息失败 |

## 安全与现有限制

- **只通过 HTTPS 使用**。不要把 Key 放进 URL、网页源代码、日志、GitHub 或聊天截图。泄露后在服务端更换 Key 并重启。
- 建议每套 Hermes/自动化系统使用一个专用机器人账号，按需配置收件人白名单；此版本配置的是单个 Key/发送人，不能在 API 调用中改变身份。
- 限流仅用于保护单一小型服务器，并不能代替公开入口的反向代理 IP 访问控制、防爆破、速率限制与审计。
- 返回 201 仅保证**服务器已保存并开始投递**，不表示收件人手机已经收到或查看了通知。收件人的离线队列受 14 天和最多 500 条限制，客户端写入 IndexedDB 后 ACK。
- 目前没有 requestId 幂等键，调用方在超时后盲目重试可能产生两条消息；重试需要自行控制。
- 如果前端从浏览器直接调用此接口，Origin 仍必须在 ALLOWED_ORIGINS 中；不建议在浏览器前端代码中存放 Key。

本功能当前在 main 源码实现，既有 GitHub Release 的 EXE 必须重新构建后才能使用。


## 个人发送 Key 管理页（2026-10-16）

登录用户可直接前往 `/api-key`，或从「设置 → 管理我的 API Key」跳转。**该页面不需要操作 Windows 的 .env**：用户可以用自己的登录账号建立独立 Key，且不依赖全局管理员 EXTERNAL_SEND_KEY。

管理 API：

| 方法 | 路径 | 权限 | 功能 |
|---|---|---|---|
| GET | /api/keys | 登录用户 Bearer Session | 列出当前账号的 Key 元数据（永不返回明文或哈希） |
| POST | /api/keys | 登录用户 Bearer Session | 创建 Key，JSON：label、recipients（至少一位）、validDays（30、90、365） |
| DELETE | /api/keys/{id} | Key 所有者的登录 Session | 立即撤销 Key |
| POST | /api/external/send | 个人 API Key 或原有管理员 Key | 无需登录，JSON：recipient、text |

个人 Key 以 `zzm_` 开头，共 68 字符，采用加密安全随机源生成。服务端在 `data/state.json` 中仅保存 SHA-256 摘要；**明文只在创建响应中显示一次**。一个账号最多 5 个 Key，每个必须限定 1-30 个已注册的收件人，不能向自己发送。外部请求以 Key 拥有者身份显示消息，不能在请求中覆盖 sender；跨账号查看/撤销被拒绝。

请求示例（测试）：`POST https://jinlangofficeb.ddnsto.com/api/external/send`，请求头 `Authorization: Bearer zzm_...`，请求正文 `{"recipient":"cjz","text":"Hermes 测试通知"}`。服务器按账号执行原有的每10秒最多25条消息限流，返回201表示写入待投递队列，不保证系统通知已展示。

**安全约束：** 只在 HTTPS 管理和传送 Key。不要将 Key 复制到网页前端项目、GitHub、截图或日志；泄露时撤销并生成新 Key。服务器备份 `state.json` 会包含密钥哈希和账号，因此必须妥善保护该文件。此版本没有历史 API 访问权限、没有发送到未授权用户的权限。原有的 .env 管理员 Key 不变，仍可继续使用。

现有用户无需重新注册账号；升级服务端 EXE 与 web 后登录即可使用本页面。
