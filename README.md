# ShaderFlow

多人 WebGPU shader 活动软件：Go 后端、管理员/大屏端与玩家端。

## 本地运行

```powershell
go run ./cmd/shaderflow -config server.dev.example.json
```

默认监听 `127.0.0.1:8080`，静态资源来自 `web/`。管理员登录入口为 `/admin/`，玩家入口为 `/player/`。

生产配置示例见 `server.example.json`，本机开发示例见 `server.dev.example.json`。局域网部署必须配置 HTTPS/WSS 证书；仅本机 `localhost` 开发可使用 HTTP。生产部署前运行 `go run ./cmd/shaderflow -config server.example.json -preflight`，确认静态资源、TLS keypair、证书 hostname 和 Archive 存储全部通过。OIDC 未配置时，本机开发配置会显式启用 `enableTestPlayerLogin`，玩家页会出现“测试玩家登录”按钮；生产配置默认关闭此入口。

本机快速启动：

```powershell
go run ./cmd/shaderflow -config server.dev.example.json
```

## Post-MVP 状态

自动化实现状态与剩余人工验收见 `docs/post-mvp-implementation-status.md`。生产部署见 `docs/windows-deployment.md`，GPU 与浏览器验收见 `docs/gpu-benchmark.md` 和 `docs/browser-qualification.md`。

## 首版边界

- 大屏是完整 DAG 的权威渲染端；玩家端仅运行自己的节点调试图。
- WebGPU 要求浏览器通过实际 storage-texture pipeline probe；`readonly_and_readwrite_storage_textures` feature 名称是可选报告项，不作为唯一拒绝依据。管理员诊断页与基准导出记录设备能力。
- shader 编译和一帧验证在浏览器完成；服务端校验身份、活动状态、代码长度、版本和协议，不运行 WGSL。
- 默认代码上传以客户端版本号为准；服务端同时记录接收时间和玩家节点。
- PNG/JPEG 导出首版支持；EXR 留作后续扩展。
