# RoundArchive 与回放核心

Status: ready-for-human

Blocked by: 03, 05

## Goal

建立版本化、可校验、可读取的 Round Archive 和纯 reducer 回放核心。

## Requirements

- manifest/schema versions、round ID、durable event sequence、时间戳和 hash。
- 覆盖设置、Execution Graph、分配、Accepted Shader Version、External Input、Camera Program、暂停/重置和 Render Snapshot。
- 内容寻址或稳定引用保存源码和 blob。
- 服务重启后可扫描、校验和列出历史 Round。
- 回放和 live Game 隔离，只读运行。

## Open decision

事件级视觉一致，还是任意时间点视觉一致？External Input 保存原图、纹理像素或两者？

## Acceptance

- golden Round 在每个事件边界折叠为相同逻辑状态。
- 缺失/损坏 blob 显示 partial archive 错误。
- 首版历史目录被标识为 legacy，不伪造缺失事实。

## Comments

已完成第一阶段：新增 schema v1 manifest、Filesystem RoundArchive adapter、complete lifecycle、目录列表、legacy/corrupt 检测、schema v1 读取、事件序号校验，以及纯 Replay reducer（Accepted Shader Version、Trajectory、terminate）。服务端已改用新 adapter，并增加 admin archives list/read/state routes。External Input blob、pause/time events、hash 已实现。管理员现可选择 Round/事件边界，将逻辑状态装入隔离 WebGPU Renderer，恢复全部 Accepted Shader Version、External Input、Camera Program 和记录 time，并在专用 canvas 渲染历史 Final Output；缺失 shader fact 时拒绝回放而不是显示 fallback。已增加上一/下一事件步进、按事件时间戳连续播放、暂停、0.5×/1×/2×/4×倍速和当前事件序号/类型/时间展示。已增加每 50 个 Round Event 的逻辑 checkpoint，并在状态路由中选择不晚于目标 sequence 的最近 checkpoint，只重放其后的事件；checkpoint 缺失或损坏时自动回退到 start snapshot 和完整事件流。视觉回放现按 graph/settings key 增量复用隔离 Renderer，并按 Accepted Shader Version / External Input version 只更新变化资源；相邻事件边界不再重建 device/graph。当前正式保证仍是事件边界视觉一致；任意 wall-clock 插值需要产品确认具体语义后再扩展，因此转为 ready-for-human。
