# ShaderFlow Post-MVP Roadmap

Status: needs-info

## Objective

把已经通过首版验收的 ShaderFlow，从“现场可运行”推进为“正确恢复、完整存档、稳定协议、可诊断、可回放和可生产部署”的系统。

## Critical path

1. 首版正确性稳定化。
2. Round Archive 与版本化 Round Event。
3. 权威 External Input、Camera Program 与时间控制同步。
4. WGSL ABI 合同。
5. GPU 性能、容量和 device-lost 诊断。
6. Render Snapshot 与离线回放。
7. 生产身份、存储和跨浏览器验收。

## Architecture direction

优先建立以下深模块及接缝：

- `PlayerCodeSession`
- `RenderParameterTimeline`
- `CaptureSession`
- `RoundJournal` / `RoundArchive`
- `ShaderContract`
- `TrajectoryCatalog` / `ActiveTrajectory`
- `RenderScheduler` / `FrameTelemetry`

调用方不直接管理这些模块内部的版本、持久化顺序、GPU 资源和失败恢复。

## Hard gates

- 不得以自动生成模板覆盖服务端 Accepted Shader Version。
- 本地非法 WGSL 不得上传；玩家本地必须显示 GPU fallback black。
- 保存必须基于冻结 Render Snapshot 并按目标尺寸重新渲染。
- Round 终止后不得继续向旧 Round Archive 写事件。
- recorder/archive 错误不得被静默忽略。
- 后续回放只读取 Round Archive，不依赖浏览器本地状态。

## Open decisions

1. 回放要求是事件级视觉一致，还是任意时间点视觉一致？
2. External Input 保存原图、纹理尺寸像素，还是两者都保存？
3. WGSL 保留完整文本编辑并严格校验，还是切换为受保护的玩家代码区域？
4. 大屏最低设备、分辨率、格式、节点数和最低 FPS 是什么？
5. 生产会话继续使用 bearer token，还是切换 HttpOnly opaque session？

## Reference

详细里程碑、依赖和验收标准见 `docs/post-mvp-roadmap.md`。
