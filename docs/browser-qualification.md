# ShaderFlow 浏览器与 GPU 验收记录

## 支持声明原则

ShaderFlow 的支持声明必须由实际设备证据产生，不根据 User-Agent 猜测。首要浏览器为最新版 Chrome/Edge；Firefox/Safari 必须分别记录 WebGPU 能力和实际结果后才能列入支持矩阵。

## 记录矩阵

每个浏览器至少记录 27 个 GPU profile：

- 1/5/10 节点
- 960×540、1920×1080、3840×2160
- rgba8unorm、rgba16float、rgba32float

记录内容包括：

- 浏览器名称和版本
- 操作系统版本
- GPU adapter info
- WebGPU features/limits
- 节点、尺寸、格式
- 平均/P95/P99 帧时间和 FPS
- direct presentation 与 device-lost recovery 检查结果
- 脱敏的证据文件名

导出 JSON 不得包含 token、cookie、secret 或 Shader 源码。`credentialsIncluded` 必须保持 `false`。

## 人工步骤

1. 运行 `shaderflow -preflight` 并保存报告。
2. 在同一设备、浏览器、电源模式下完成 `docs/gpu-benchmark.md` 的 27 个 profile。
3. 每个 profile 至少执行三次，保存导出 JSON。
4. 检查 direct GPU presentation、输入上传、输出预览、保存和点击全屏。
5. 模拟一次 device lost，确认自动恢复一次；再次失败时确认界面给出明确降级提示。
6. 使用 `qualificationMatrixStatus` 检查浏览器/profile 缺失项。
7. 只有完整证据经过人工审核后，才把浏览器/GPU组合标记为 supported。

## 结果状态

- `supported`: 全部要求 profile 和关键路径验收通过。
- `conditional`: 部分格式/分辨率通过，明确限制和最低 FPS。
- `unsupported`: WebGPU 能力、TLS/浏览器行为或稳定性不满足要求。
- `untested`: 没有足够证据，不能推断支持。
