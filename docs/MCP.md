# ZzzMessage MCP：AI 使用登录账号代理发送消息

本功能是**独立 stdio MCP 服务端**，不需要把 AI 接入 ZzzMessage 进程，也不需要原有 EXTERNAL_SEND_KEY。支持 Windows x64、Linux x64、Linux ARM64。适用于 Hermes Agent 以及支持本地 stdio MCP 的 AI 客户端。

## 工作机制

1. 在 ZzzMessage 中注册专门供 AI 使用的账号，例如 `hermes_bot`。这可以避免让 AI 使用你的主账号密码。
2. MCP 进程从本地进程环境读取 `ZZZMESSAGE_URL`、`ZZZMESSAGE_USERNAME`、`ZZZMESSAGE_PASSWORD`。**模型的工具参数不能设置或读取密码**。
3. MCP 第一次发起操作时向 ZzzMessage 的 `POST /api/auth/login` 登录并缓存 Session Token。会话失效（HTTP 401）时自动重新登录一次。
4. `zzzmessage_list_users`：通过 `GET /api/users` 获取当前账号可见的注册用户名称。启用白名单时只展示允许的收件人。
5. `zzzmessage_send_private_message`：向 `POST /api/direct` 建立或复用两人私聊，随后通过 `POST /api/messages/send` 发送消息。消息以登录账号身份发出，使用既有离线队列、WebSocket/HTTPS 与 iPhone/Android 推送。

工具不提供读取聊天正文、好友历史、管理账号、发送群消息或设置消息来源的入口。MCP 账号本身可通过普通 ZzzMessage 网页登录。

## 环境变量

| 变量 | 是否必需 | 用途 |
|---|---|---|
| `ZZZMESSAGE_URL` | 必需 | ZzzMessage 站点根地址，例如 `https://jinlangofficeb.ddnsto.com`。远程连接只允许 HTTPS。 |
| `ZZZMESSAGE_USERNAME` | 必需 | 专用 AI 登录账号，例如 `hermes_bot` |
| `ZZZMESSAGE_PASSWORD` | 必需 | 上述账号的登录密码，仅保存在 AI 客户端本地配置/秘密环境中 |
| `ZZZMESSAGE_MCP_ALLOWED_RECIPIENTS` | 建议 | 英文逗号分隔的允许用户名，例如 `alice,bob`。留空则可向任意已注册的其他用户发送，建议私人部署显式填写 |

注意：只有 `http://localhost` 或 `http://127.0.0.1` 的本机开发环境允许 HTTP。**不支持**通过 `http://192.168.x.x` 发送账号密码，请使用 DDNSTO 的 HTTPS 域名或配置 HTTPS。

## Linux / Hermes Agent（通用 stdio MCP 配置）

在下载的 Linux 发布包中解压 `zzzmessage-mcp`，赋予可执行权限：

```bash
chmod 700 /opt/zzzmessage/zzzmessage-mcp
```

在支持 `mcpServers` 的 AI 客户端配置中添加（具体配置文件位置按 Hermes/客户端版本为准）：

```json
{
  "mcpServers": {
    "zzzmessage": {
      "command": "/opt/zzzmessage/zzzmessage-mcp",
      "args": [],
      "env": {
        "ZZZMESSAGE_URL": "https://jinlangofficeb.ddnsto.com",
        "ZZZMESSAGE_USERNAME": "hermes_bot",
        "ZZZMESSAGE_PASSWORD": "在此配置真实密码，不要交给 AI 对话",
        "ZZZMESSAGE_MCP_ALLOWED_RECIPIENTS": "alice"
      }
    }
  }
}
```

如 Hermes 使用不同的 MCP 配置结构，应将可执行程序与四个环境变量填入对应的本地 stdio MCP 服务配置；不要误设为远端 HTTP MCP URL。

## Windows

使用 `ZzzMessage-MCP.exe`，设置为 MCP 本地命令：

```json
{
  "mcpServers": {
    "zzzmessage": {
      "command": "D:\\ZzzMessage\\ZzzMessage-MCP.exe",
      "args": [],
      "env": {
        "ZZZMESSAGE_URL": "https://jinlangofficeb.ddnsto.com",
        "ZZZMESSAGE_USERNAME": "hermes_bot",
        "ZZZMESSAGE_PASSWORD": "你的机器人账号密码",
        "ZZZMESSAGE_MCP_ALLOWED_RECIPIENTS": "alice"
      }
    }
  }
}
```

**不要直接双击 MCP.exe**：它使用 MCP stdio JSON-RPC，需要由支持 MCP 的 AI 客户端启动并保持进程连接。MCP 协议响应会写入 stdout，诊断信息写 stderr。

## 暴露给 AI 的工具

`zzzmessage_list_users` 不接收参数，返回允许发送的 `{username, displayName}` 列表。

`zzzmessage_send_private_message` 接收 JSON：

```json
{ "recipient": "alice", "text": "Hermes 自动提醒：今天的任务已完成" }
```

成功返回 messageId、roomId、from、to、createdAt。201 只证明 ZzzMessage 接受并持久保存消息，**不是已读回执、也不保证系统通知显示**。原有每账号 25 条／10 秒的发送限制同样适用。

## 权限与安全

- **建议新建专用低权限机器人账号**，而不是输入主账号密码。密码用于获取拥有普通用户权限的 Session Token，但 MCP 服务端只暴露两项工具。
- MCP 工具调用可能被 AI 主动提出；在 Hermes 或 MCP 宿主中为发消息工具配置**每次发送前确认**，否则登录账号可被 AI 自主使用。工具描述中明确要求用户确认，但代码不能替 MCP 宿主强制弹出确认。
- 通过 `ZZZMESSAGE_MCP_ALLOWED_RECIPIENTS` 严格限制接收账号。当前白名单由本地 MCP 进程执行，并非 ZzzMessage 服务器的强制角色权限；保护好运行主机及环境变量。
- 不要把密码写在 README、GitHub 仓库、聊天消息或普通日志里；使用 AI 客户端受保护的秘密配置。修改密码后更新 MCP 环境并重启服务。
- 发送接口只支持纯文本私聊，每条最多 4000 个 Unicode 字符。没有幂等键：网络超时后的自动重试可能导致重复消息。
- `/api-key` 是此前的**API Key 免登录方案**，此 MCP 是**账号密码登录方案**。两者独立，可同时使用；从安全权限最小化角度，单纯发消息的自动化通常优先用 API Key，必须使用账号登录的 AI 客户端再用此 MCP。

## 验证

使用 AI 客户端连接后，先让 AI 调用 `zzzmessage_list_users`，确认查询到预期账号；随后明确批准一次 `zzzmessage_send_private_message`，再到目标账号登录查看消息及推送。MCP 程序自身无网页 UI。

对应源码：`cmd/zzzmessage-mcp/main.go` 与 `client.go`。发布包会同时含主服务程序、MCP 可执行程序与此文档。
