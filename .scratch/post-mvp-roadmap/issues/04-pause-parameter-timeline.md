# RenderParameterTimeline：暂停和 pending 状态

Status: resolved

## Goal

统一管理员和玩家的暂停语义：暂停冻结当前活动状态，但允许继续编辑 pending 状态。

## Requirements

- `pause()` 创建不可变 active snapshot。
- 暂停期间允许编辑代码、输入、相机和控制参数，只更新 pending state。
- `resume()` 原子提升最新 pending state。
- 管理员 manual camera 在暂停期间不能改变已显示 metadata。
- 玩家输入在暂停期间不应被禁止。

## Acceptance

- 暂停后修改输入、相机和代码，渲染画面保持不变。
- 恢复后一次性使用最新 pending 状态，不重放中间编辑。
- 管理员和玩家的行为测试使用同一状态模型。

## Comments

已实现共享 `RenderParameterTimeline`：暂停冻结 active metadata，编辑只更新最新 pending metadata，恢复原子提升。玩家暂停期间允许上传输入并缓存到恢复时应用；管理员保留既有 pending input，并冻结 manual/script camera metadata。共享模块有独立行为测试。
