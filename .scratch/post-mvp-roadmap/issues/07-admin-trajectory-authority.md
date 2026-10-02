# 管理员 Camera Program 发布与玩家共享

Status: resolved

Blocked by: 03

## Goal

把管理员 Camera Program 和时间控制纳入权威 Round state，并允许玩家选择临时使用。

## Requirements

- active trajectory 包含 id/version/name/initSource/frameSource/validatorVersion/updatedAt。
- 管理员发布前本地安全解释器验证；服务端校验大小、版本和状态。
- WebSocket 和 snapshot 发布最新 active version。
- 玩家 opt-in 使用脚本内容但保留自己的 local time origin，不覆盖本地编辑器。
- 每个发布版本写入 Round Archive。

## Acceptance

- 两个玩家在 1 秒内收到发布；opt-out 不受影响。
- 断线重连恢复最新 active version。
- stale/oversized/invalid trajectory 被拒绝。

## Comments

已实现第一阶段权威轨迹：Game State 增加版本化 `Trajectory`，仅 active 版本投影给玩家，管理员可见 inactive 状态；`PUT /api/admin/trajectory` 校验 phase、版本、名称、大小和 validatorVersion，并记录 `trajectory.updated`。管理员应用 script camera 时发布，玩家既有 opt-in 路径直接使用 state.trajectory 且保持本地 time origin。目录预设持久化和服务端共享解释器语法验证留给后续。
