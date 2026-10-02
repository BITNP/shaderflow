# ShaderFlow Post-MVP 实现状态

更新日期：2026-10-01

## 自动化实现完成

- PlayerCodeSession：重连恢复 Accepted Shader Version，本地 draft/invalid/fallback 隔离。
- CaptureSession：冻结请求、目标尺寸隔离重渲染、Render Snapshot 存档。
- RoundJournal：关闭生命周期、写失败回滚、runtime degraded/fail-closed。
- RenderParameterTimeline：暂停冻结和恢复时最新 pending state 原子提升。
- ShaderContract：WGSL ABI、binding、metadata、workgroup、输出写入验证。
- RoundArchive：schema v1、事件、blob、checkpoint、Catalog、reducer、视觉回放、诊断/修复、orphan GC。
- 权威状态：External Input、Camera Program、TimeControl、Render Snapshot。
- GPU：direct presentation、缓存、telemetry、device-lost 单次恢复、27-profile benchmark 与 QualificationRecord。
- 生产身份：opaque cookie session、logout/expiry/persistence、单玩家单设备、Origin policy、bcrypt、rate limit、secret refs。
- 生产存储：Preflight、TLS/publicUrl/OIDC 校验、Archive health/degraded、Windows 启动与健康检查脚本。

## 需要人工确认

### 回放产品语义

当前正式保证：任意 Round Event 边界的逻辑和视觉重建。尚未定义事件间任意 wall-clock 时刻的 Shader/Input 阶跃和 Camera/time 插值规则。若没有产品需求，可保持事件边界语义并关闭该事项。

### GPU 与浏览器矩阵

必须在真实 RTX 5070 和目标浏览器执行：

- 1/5/10 节点
- 960×540、1920×1080、3840×2160
- rgba8unorm、rgba16float、rgba32float
- 每个 profile 至少三次

输出 QualificationRecord 后决定最低 FPS 与 supported/conditional/unsupported。

### 生产部署

必须用真实环境验证：

- CA/内部 CA certificate 与 publicUrl hostname
- OIDC discovery/login/callback/logout
- env/file secret mount 权限
- dataDir 无权限、只读和磁盘配额故障
- 进程重启后的 session/catalog/archive 恢复

## 当前发布判断

代码和自动化测试阶段可进入 Release Candidate。不能在完成人工矩阵前宣称生产环境、RTX 5070 或全部 Chrome/Edge/Firefox/Safari 已正式支持。
