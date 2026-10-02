# ShaderFlow GPU 基准执行说明

## 目标矩阵

在目标 RTX 5070 机器的同一浏览器版本、同一电源模式下，完成 27 个 profile：

- 节点：1、5、10
- 尺寸：960×540、1920×1080、3840×2160
- 格式：rgba8unorm、rgba16float、rgba32float

## 执行

1. 使用管理员页面创建指定节点数、尺寸和格式的 Round，并开始运行。
2. 等待所有节点 shader 安装且画面稳定。
3. 在“GPU 基准采样”点击“采样当前配置”。工具预热 60 帧并采样 300 帧。
4. 每个 profile 至少采样三次；避免窗口遮挡、系统更新、录屏或其他 GPU 工作负载。
5. 完成后导出 JSON。导出包含 userAgent、adapter info、WebGPU limits/features、估算纹理字节、平均/P95/P99 帧提交时间和 FPS。
6. 把结果保存到 `benchmarks/<device>/<date>/`，不要提交机器用户名、token、源码或 secret。

## 解释

该数据测量当前 Renderer 的 CPU encode + queue submit 等待范围，不等同于可中断的 GPU dispatch 时间。`passTimeoutMs` 仍只是 advisory budget。

建议按三次采样的中位数定义 profile 基线，并记录浏览器版本、驱动版本、电源策略、显示器刷新率。最低现场 profile 和最低 FPS 必须由人工验收后写入 issue；在此之前不要宣称 RTX 5070 矩阵已经通过。
