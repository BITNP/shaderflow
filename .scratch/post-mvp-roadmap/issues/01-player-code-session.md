# PlayerCodeSession：恢复、草稿和最后成功版本

Status: resolved

## Goal

防止重连时自动模板覆盖 Accepted Shader Version，并统一玩家本地 valid/invalid/fallback 行为。

## Requirements

- 玩家 state projection 返回自己的 Accepted Shader Version 源码和版本，不能泄露其他玩家源码。
- 进入已分配节点时，优先恢复服务端 Accepted Shader Version。
- localStorage 只作为 local draft；不得自动上传模板或草稿。
- 只有用户草稿通过本地验证时才允许显式/防抖提交。
- invalid 草稿立即切换玩家本地 GPU fallback black；大屏继续使用 Accepted Shader Version。
- GPU WebGPU canvas 不使用 2D context 清黑。

## Acceptance

- 已有 v1、空 localStorage 重连：编辑器恢复 v1，不 POST 新代码。
- 已有 v1、本地 invalid 草稿：本地黑屏、服务端仍为 v1。
- 首次无代码：显示模板草稿，但不因初始化自动上传。
- 玩家快照只包含自己的 accepted code。

## Comments

已实现第一阶段：玩家 snapshot 返回自己的 `ownCode`；初始化恢复服务端 Accepted Shader Version；首次模板不再自动上传；本地非法草稿切换 renderer GPU fallback black。后续仍可进一步把上传状态和冲突诊断全部收敛进 `PlayerCodeSession`。
