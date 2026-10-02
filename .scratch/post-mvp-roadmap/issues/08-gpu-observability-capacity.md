# GPU 性能、容量与 device-lost 诊断

Status: ready-for-human

## Goal

让 RTX 5070 和现场设备的性能、显存和 GPU 错误可测量、可解释、可降级。

## Requirements

- adapter features/limits、纹理数量、绑定数和估算 VRAM 统一报告。
- 缓存 steady-state 的 texture views、bind groups 和 metadata bind group。
- 报告 compute encode、queue submit、呈现和 device lost。
- passTimeoutMs 明确为 advisory budget；连续超预算有可见 fallback，不宣称可中断单个 dispatch。
- 管理员 device lost 可重建 graph、inputs、shaders 和 presentation targets。

## Open decision

最低大屏设备、分辨率、纹理格式、节点数和最低 FPS；RTX 5070 是否只是首要基线。

## Acceptance

- 1/5/10 节点、540p/1080p/4K、三种格式有基线报告。
- 稳态帧中不反复创建 bind group。
- 模拟 device loss 后可恢复一次或清晰降级。

## Comments

已实现第一阶段：`Renderer.diagnostics()` 报告 features/limits、节点/纹理数、尺寸/格式、估算纹理显存和 device-lost；稳态帧缓存 texture views、每节点 texture bind group 和 metadata bind group；管理员显示 submit FPS、平均帧提交时长、容量与连续 passTimeoutMs advisory 超预算；device lost 自动重建 graph、Accepted Shader Versions、权威输入和 presentation targets 一次。已增加管理员内置 `GpuBenchmarkSession`：当前 profile 预热 60 帧、采样 300 帧，输出平均/P95/P99、FPS、adapter info、limits/features 和显存估算；矩阵助手跟踪 27 个目标 profile 并可导出 JSON，执行说明见 `docs/gpu-benchmark.md`。导出现在同时生成脱敏 QualificationRecord（browser/OS/GPU/profile/result/checks/evidence，明确 `credentialsIncluded:false`），并可用 `qualificationMatrixStatus` 列出浏览器×profile 缺失证据；人工步骤见 `docs/browser-qualification.md`。仍需人工在 RTX 5070 和目标浏览器上每个 profile 至少运行三次并确定最低 FPS/profile，因此保持 ready-for-human。
