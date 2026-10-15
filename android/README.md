# ZzzMessage Android APK 客户端

这是单独的 Android Studio / Kotlin 工程。聊天页面通过 WebView 连接用户选择的可信 HTTPS Vue 前端（web/），而系统推送通过原生厂商 SDK 完成。首批支持 Xiaomi/Redmi 与 vivo/iQOO。

目前只有源码，没有构建 APK，没有完成厂商 SDK 下载、应用推送开通和真机验证。

## 需要准备

1. 在 Xiaomi 和 vivo 开放平台注册应用，开通厂商推送，确认包名、签名与应用资料满足厂商要求。
2. 项目 applicationId 当前为 io.github.zzzhe2333.zzzmessage；如果更换，请同步修改 Android Gradle、Go XIAOMI_PACKAGE_NAME 与厂商控制台。
3. 从官方平台下载 MiPush Android AAR、vivo Push AAR，放入 android/app/libs/。这里不分发第三方闭源 SDK。未放置两个 AAR 时当前 Kotlin 工程不能完整编译。
4. 在 android/local.properties 配置 Android SDK 路径及以下客户端字段：

    XIAOMI_APP_ID=...
    XIAOMI_APP_KEY=...
    VIVO_APP_ID=...
    VIVO_APP_KEY=...

5. 在 Go 后端部署时，在受保护的 .env 中配置以下**服务器专用**变量：

    XIAOMI_PACKAGE_NAME=io.github.zzzhe2333.zzzmessage
    XIAOMI_APP_SECRET=...
    VIVO_APP_ID=...
    VIVO_APP_KEY=...
    VIVO_APP_SECRET=...

不要把 XIAOMI_APP_SECRET 或 VIVO_APP_SECRET 写进 Android APK 或 GitHub！

6. 将 web/ 部署到 HTTPS 域名，在 Android App 第一次启动时输入这个网址（输入的是 Vue 前端地址）；之后仍可在 Vue 中自行选择 Go 后端。Go API 默认监听 58463，发布到外网前需要 HTTPS 反向代理。

## 运行与系统推送

Android Studio 打开 android/，待 SDK、密钥和 Gradle 项目配置齐全后，由开发者**主动**点击 Build APK。当前没有 Gradle wrapper，也不会因提交 GitHub 触发构建。

手机安装 APK 后，输入 HTTPS 聊天网站地址，使用邀请码注册并登录。进入聊天设置，主动点击「启用小米 / vivo 系统通知」，同意系统权限；Android 13+ 需要 POST_NOTIFICATIONS 授权。

Xiaomi Push 注册成功后，MiPushReceiver 获取 regId；vivo 推送通过 PushClient 初始化、turnOnPush、getRegId 获得 regId。WebView 仅传厂商类型和 regId 给 Vue 页面，再以当前登录用户身份调用 Go API 注册设备。后端不保存聊天正文，仅保存账户相关设备令牌并发出通用提醒。

vivo SDK 需要在初始化前向用户告知其数据处理情况并取得相应同意。当前接入是点击按钮后才初始化，而不是 App 启动即初始化。请在正式使用前完善隐私政策与用户授权界面。

这不是完全离线 APK：Vue 页面仍然从 HTTPS 网站载入。也不是已经签名和通过厂商审核的正式发行版本。厂商推送可能有审核、模板、签名、频次和配额限制。

官方参考：
- Xiaomi SDK：https://dev.mi.com/xiaomihyperos/documentation/detail?pId=1544
- Xiaomi HTTP API：https://dev.mi.com/xiaomihyperos/documentation/detail?pId=1559
- vivo 推送：https://developers.vivo.com/product/d/messagePush
- vivo 隐私说明：https://developers.vivo.com/doc/d/23807c559e844cbeb06049ee69e71833
