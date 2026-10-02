# Windows 部署与现场验收

## 开发

```powershell
go run ./cmd/shaderflow -config server.dev.example.json
```

开发配置只绑定 `127.0.0.1`，允许 test player login，不应作为局域网生产配置。

## 生产启动

1. 准备 `server.example.json`。
2. 设置 `SHADERFLOW_ADMIN_PASSWORD_HASH`，或把 bcrypt hash 放入 `adminPasswordHashRef.file` 指向的受限文件。
3. 挂载 OIDC client secret。
4. 放置与 `publicUrl` hostname 匹配的 CA/内部 CA TLS certificate 和 private key。
5. 确认 `oidc.redirectUrl` 与 `publicUrl` origin 完全一致。
6. 运行：

```powershell
.\scripts\Start-ShaderFlow.ps1 -Config server.example.json
```

脚本默认先运行 `-preflight`。Preflight 失败时不会启动服务。

## 构建后启动

```powershell
.\scripts\Start-ShaderFlow.ps1 -Config server.example.json -Build
```

## 健康检查

启动已有服务后：

```powershell
.\scripts\Test-ShaderFlowDeployment.ps1 -Config server.example.json -BaseUrl https://shaderflow.example.com:8443
```

脚本会先重新执行 Preflight，然后轮询 `/api/health`，直到 storage ready 或超时失败。

## 现场 GPU 验收

使用管理员页面的 GPU 基准采样完成 27 个 profile。保存脱敏 QualificationRecord；不要保存 cookie、token、secret 或 Shader 源码。具体矩阵和最低 FPS 决策见：

- `docs/gpu-benchmark.md`
- `docs/browser-qualification.md`

## 故障演练

至少执行一次：

- 只读/无权限 dataDir：Preflight 应失败；
- 磁盘写入失败：`/api/health` 应变为 503，Round 后续权威 mutation 应被阻断；
- 进程重启：sessions/catalog 可恢复；
- device lost：管理员只自动尝试一次恢复，二次失败显示明确降级。
