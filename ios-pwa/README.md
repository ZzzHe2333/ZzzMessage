# iPhone PWA 客户端（独立源码目录）

这是独立的 Vue 3 + Vite + Service Worker / Web Push iPhone 客户端源码，已从原 web/ 初始复制，今后可以独立修改和部署。

- 使用 iOS 16.4+ 的 Safari 在 HTTPS 网站安装「添加到主屏幕」。
- 在首页填写自己的 Go 后端 HTTPS URL；持久保存在此设备的浏览器存储。
- 开启通知前，服务器需配置 VAPID 公私钥（详见 [../docs/PUSH.md](../docs/PUSH.md)）。
- Web Push 不需 Apple Developer 99 美元年费；不含 iOS 原生 IPA。
- 本目录与 web/ 是独立文件树；**以后修改共用功能时，需要人工同步或抽取公共模块**，不要只改其中一边。
- 若部署在独立域名，需将 iPhone PWA 的 origin 加入服务端 ALLOWED_ORIGINS。

手动开发：在 ios-pwa/ 执行 npm install、npm run dev；手动部署前执行 npm run build。按照项目约定，代码提交不会自动构建。
