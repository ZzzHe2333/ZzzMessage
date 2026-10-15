# Android 厂商系统通知（Go API）

后端支持 Xiaomi 与 vivo 两个独立的通知通道，由环境变量开启：XIAOMI_APP_SECRET、XIAOMI_PACKAGE_NAME、VIVO_APP_ID、VIVO_APP_KEY、VIVO_APP_SECRET。所有服务端密钥只保存在 Go 后端环境变量内。

需要登录 Bearer Token 的接口：
- GET /api/native-push/config：获取厂商配置状态。
- PUT /api/native-push/device：请求体 JSON 包含 provider（xiaomi/vivo）和 token（厂商 regId）。
- DELETE /api/native-push/device：请求体相同，取消该账户的设备绑定。
- POST /api/native-push/test：向当前账户已绑定设备发送测试提醒（限制测试频率）。

Go 通过 Xiaomi 正式 API /v2/message/regid 发送通知；vivo 先进行 /message/auth 鉴权，使用 MD5(appId+appKey+timestamp+appSecret) 签名，再向 /message/send 发送通知。认证结果缓存在内存中，减少重复鉴权请求。所有发送目标地址在代码中固定，客户端无法控制 URL。

通知始终只显示「收到一条新消息」，不包含任何聊天正文、昵称或群聊名称。服务器只保存账号、群元数据和设备令牌，不保存消息正文。离线时可以收到通知，但消息正文不能在恢复连接后补发。

这部分代码尚未构建或连接厂商正式推送服务测试。厂商平台可能需要应用审核、消息模板、包名及签名一致等额外条件。详见 android/README.md。
