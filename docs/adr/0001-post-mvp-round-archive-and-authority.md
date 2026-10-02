# ADR-0001: Round Archive 是回放事实来源，管理员大屏是运行时权威渲染器

- 状态：Proposed
- 日期：待路线决策确认

## Context

首版已经通过现场测试，但当前存档主要包含启动状态、事件和代码，External Input、Camera Program、时间控制和保存快照仍可能只存在浏览器本地。未来要支持任意 Round 回放，必须明确哪些数据构成事实来源；同时玩家 Player Sandbox 只能用于本地调试，不能成为活动结果的权威来源。

## Decision

1. 每个 Round 建立一个 Round Archive。
2. Round Archive 记录所有影响未来画面的 Round Event，并为大对象使用轮次目录内的独立文件：源码、输入图像、画板导出、Render Snapshot。
3. Event 按服务端序号和时间顺序追加；事件包含足以定位相关对象的版本和文件引用。
4. Authoritative Renderer 是管理员大屏；玩家端不上传本地调试图像作为活动输入，也不作为回放依据。
5. Round 开始后 Execution Graph、纹理尺寸和格式等结构性参数固定；代码和权威输入通过事件版本更新。
6. 回放模块未来通过读取 Round Archive 重建状态，不依赖当时的浏览器 localStorage 或当前服务内存。

## Consequences

- 后续必须定义 Round Event 的可序列化类型和 Archive 目录格式。
- 管理员输入、相机和时间控制必须经过后端记录，不能只改本地状态。
- 未来可以实现离线回放播放器和确定性的事件时间轴。
- 当前首版的浏览器本地导出仍可保留，但不应宣称它已经是可回放的活动存档。
