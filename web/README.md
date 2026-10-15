# ZzzMessage Web 客户端

本目录是通用 Vue 网页客户端，也被 Android 原生 WebView 通过 HTTPS 加载。

iPhone PWA 独立源码位于 ../ios-pwa/ 。
Android Kotlin / SDK 工程位于 ../android/ 。
HarmonyOS 目前只预留了 ../harmonyos/README.md 。

今后修改聊天核心功能请同时检查 web/ 与 ios-pwa/ 的对应代码；目前两者为物理独立副本。

本客户端已经添加 `/gift` 礼物商城、`/money` 三币种钱包、同账号历史分片同步。历史始终在各设备的 IndexedDB 中独立持久化；从其他在线设备经由 Go 中继恢复。服务端不会永久留存完整聊天历史。
