# ShaderFlow Glossary

## Round

一次从开放报名开始，到管理员终止或结束为止的活动周期。一个 Round 包含活动设置、报名名单、执行图、节点分配、代码版本、权威输入和时间轴事件。

## Draft Graph

Round 正式开始前，由系统生成并允许管理员修改的执行图。Draft Graph 通过合法性检查并开始 Round 后，成为 Execution Graph。

## Execution Graph

Round 运行期间固定的有向无环计算图。每个 Shader Node 对应一名玩家和一个 compute pass；所有外部输入、端子和节点都必须位于通向唯一 Final Output 的路径上。

## Shader Node

Execution Graph 中由一名玩家负责的计算节点。一个 Shader Node 拥有一个或多个 Input Terminal 和 Output Terminal，并在每一帧对完整纹理范围执行一次 compute pass。

## Terminal

Shader Node 的纹理绑定位置。Input Terminal 只接收一个上游 Texture；Output Terminal 产生一张 Texture，并可被多个下游 Input Terminal 共享读取。

## External Input

由管理员上传或绘制、作为 Execution Graph 起点的权威输入图像。玩家本地调试图像不是 External Input。

## Final Output

Execution Graph 唯一的最终输出 Texture，也是大屏主画布显示和活动结果导出的默认对象。

## Player Sandbox

玩家浏览器中只包含自己 Shader Node 的本地调试环境。Player Sandbox 使用本地测试输入，不执行完整 Execution Graph，也不是 Round 的权威渲染结果。

## Authoritative Renderer

管理员大屏中执行完整 Execution Graph 的渲染器。活动现场看到的结果、回放所需的状态和保存快照以 Authoritative Renderer 使用的版本为准。

## Accepted Shader Version

玩家端编译并完成一帧验证后，被后端接受和持久化的代码版本。新代码失败时，Authoritative Renderer 继续使用上一个 Accepted Shader Version。

## Round Event

Round 中影响未来回放结果的有序事实，包括状态变化、Execution Graph、节点分配、Accepted Shader Version、External Input、Camera Program、时间重置、暂停和保存快照。

## Round Archive

按 Round 持久保存的数据集合。它必须足以在未来重建该 Round 的 Execution Graph，并按时间顺序重放所有 Round Event。

## Camera Program

根据 time 计算相机位置、Y-P-R 欧拉角和 FOV 的安全脚本。欧拉角和 FOV 使用度，time 使用毫秒。

## Render Snapshot

在一个明确时间点冻结 Execution Graph、Accepted Shader Version、External Input、Camera Program 结果和纹理设置后得到的可重复渲染状态。
