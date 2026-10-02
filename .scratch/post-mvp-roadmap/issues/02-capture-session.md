# CaptureSession：冻结并按目标尺寸保存

Status: resolved

## Goal

实现真正的 Render Snapshot，保证保存结果不受实时渲染循环中的更新影响。

## Requirements

- 捕获开始时冻结 graph、Accepted Shader Version、External Input version、Camera Program、time、pause state、format 和目标尺寸。
- 目标尺寸小于等于 live resolution 时，重新渲染到目标尺寸；不能只对 live texture 做 2D 缩放。
- 捕获期间的代码、输入和相机更新不改变结果。
- snapshot manifest 记录所有版本和时间信息。

## Acceptance

- shader 使用 canvasSize 时，指定尺寸产生对应尺寸结果。
- 捕获开始后立即修改代码/输入/相机，保存仍使用旧快照。
- PNG/JPEG 文件与 manifest 关联。

## Comments

已实现浏览器内 CaptureSession：调用时 structuredClone 冻结请求，并阻止并行捕获。Renderer.capture 使用同一 GPUDevice 创建隔离 Renderer，复制当前 graph、成功 shader source 和 External Input source/mode，在目标尺寸重新配置纹理、改写 metadata.canvasSize、执行一帧并读回编码；不再通过 Canvas 2D 缩放 live texture。管理员运行中保存现会将同一冻结请求和生成 blob 上传到 Round Archive，记录 `snapshot.created`，manifest 包含 texture、尺寸、time、源事件序号、Accepted Shader Version map、External Input version map、Camera Program 和 TimeControl 版本。
