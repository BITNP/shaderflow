# ShaderFlow 首版之后的实现路线

> 本文基于首版已通过现场测试的当前代码、已确认需求和当前已知缺口制定。路线采用“小而深的模块”原则：调用方只依赖少量稳定接口，复杂性集中在对应模块内部；测试跨越同一接口，而不是绕过接口测试内部实现。

## 0. 当前基线

首版已经具备：

- Go 后端活动状态、报名、节点分配、DAG 生成与校验、代码缓存、WebSocket 同步和轮次基础存档。
- 管理员/大屏端完整 DAG 渲染、GPU canvas 全屏三角形显示、输入图上传/画板、节点预览、相机和导出。
- 玩家端单节点 Player Sandbox、WGSL 本地编译和一帧验证、成功代码上传、输入图调试、相机脚本和 AI 提示词。
- WebGPU 纹理格式、metadata、GPU canvas 呈现和 PNG/JPEG 导出。

首版通过不等于所有后续目标已经完成。当前路线的首要目标是：**把现场可运行的系统变成可重复、可诊断、可演进的活动平台。**

## 1. 优先级原则

优先级顺序：

1. 不丢失活动事实，不破坏现场活动。
2. 在同一台设备上可诊断性能、GPU 能力和故障恢复。
3. 让玩家代码、图结构和相机语义成为稳定协议。
4. 再做回放、导出格式和跨浏览器扩展。
5. 最后做体验增强和大规模部署。

不建议下一步直接加入新视觉功能；先把会影响数据正确性和现场恢复的模块做深。

## 2. Milestone 0：首版正确性稳定化

> 这是新增的前置门槛。审计发现以下问题会直接破坏首版已承诺的行为，因此必须先修复，再扩展 Round Archive。

### P0：玩家代码恢复与本地代码状态

建立 `PlayerCodeSession` 深模块，至少区分：

- `serverAccepted`：服务端最后接受的版本和源码
- `localDraft`：玩家当前编辑内容
- `localStatus`：`valid`、`invalid`、`fallback-black`
- `uploadState`：未提交、提交中、已接受、被拒绝

行为要求：

- 重连、换浏览器或清空 localStorage 后，先恢复服务端最后成功代码，不得自动生成模板并上传覆盖。
- 首次没有服务端代码时才允许把模板作为编辑草稿；模板不能因为自动检查而隐式成为活动代码。
- 本地编译失败时，玩家本地输出立即使用黑色 fallback；大屏继续使用服务端最后成功版本。
- 本地 invalid 草稿不能触发 `/api/code`。
- 不依赖 WebGPU canvas 的 2D context 清黑；清黑必须是 renderer 的 GPU fallback 呈现路径，或单独的 fallback canvas。

### P0：冻结保存与暂停快照

建立 `RenderSnapshot` / `CaptureSession` 深模块：

- 在捕获开始时冻结 graph、Accepted Shader Version、External Input version、Camera Program、time、pause state、format 和目标尺寸。
- 按请求尺寸重新渲染，不能只把当前 live texture 用 2D canvas 缩放。
- 后台代码、输入或相机更新不能改变正在进行的保存结果。
- 暂停时允许编辑输入、代码和相机，但活动画面继续使用不可变 frozen snapshot；恢复时一次性提升 pending state。
- 管理员和玩家使用同一套 pause/commit 语义，不能由两个页面各自解释。

### P0：Round 生命周期隔离

建立 `RoundJournal` 深模块：

- `Begin(snapshot)`、`Record(event)`、`StoreBlob(blob)`、`Close(outcome)` 是最小外部接口。
- Round terminate 后立即关闭当前 journal；后续 registration/configuration 不能写入旧 Round 目录。
- 持久化错误必须可见：接受状态变更前记录失败，或将 Round 标记为 degraded 并阻止关键写入；不能静默丢失 `Append` 错误。
- 两次 Round 必须拥有互不重叠的 event sequence 和目录。

### P0：首版稳定化验收

- 既有服务端代码 + 空 localStorage 重连：不发生自动覆盖。
- 成功版本后编辑非法 WGSL：玩家画面黑屏、大屏仍运行成功版本、服务端版本不变。
- 保存时同时修改输入/代码/相机：保存结果仍对应捕获开始时的状态。
- 暂停期间修改输入/代码/相机：画面不变；恢复后一次性显示最新修改。
- Round A 终止后创建 Round B：A 的事件文件不增长。
- 注入 recorder 写入失败：客户端收到明确失败或 degraded 状态，服务端日志有结构化错误。

### 依赖

无。该里程碑是所有后续功能的安全前提。

## 3. Milestone 1：权威 Round Archive 与事件模型

### 目标

让任何一次 Round 的未来回放所需事实都来自 Round Archive，而不是当时浏览器的 localStorage 或内存。

### 新增模块与接缝

- `RoundEvent`：不可变、带服务端序号和时间的领域事实。
- `RoundArchive`：小接口，例如 `StartRound`、`AppendEvent`、`StoreBlob`、`ReadRound`；文件布局、原子写入、命名和校验隐藏在实现中。
- `AuthoritativeState`：从事件折叠出当前 Round 状态。

事件至少覆盖：

- Round 设置和开始时的 Execution Graph
- 玩家身份快照和节点分配
- Accepted Shader Version 全文
- External Input 的上传/绘制结果、尺寸模式和应用时间
- Camera Program、预设、FOV、time origin、暂停/恢复
- 纹理尺寸/格式等固定参数
- 保存请求、Render Snapshot 元数据和失败结果
- 终止、重置和错误状态

### 验收标准

- 删除服务内存后，仅用轮次目录重建设置、图、玩家、代码和权威输入。
- 事件序号单调；重复写入或服务重启不会产生重复版本。
- 每个大文件有文件引用、大小和 hash；读取时校验失败会明确报告。
- 首版既有历史目录仍可读取，缺少新事件时以“不可回放字段”显式标识，而不是静默伪造。

### 依赖

无。它是所有未来回放、审计和恢复能力的基础。

## 4. Milestone 2：管理端权威状态同步与轨迹发布

### 目标

把管理员端目前只存在本地的 External Input、Camera Program、时间控制和保存状态变成服务端权威状态，并允许玩家临时使用管理员轨迹。

### 新增模块与接缝

- `RoundControl`：管理员命令的少量入口，负责权限、phase 和事件记录。
- `CameraProgramStore`：保存/发布/撤销当前 Camera Program。
- `AuthoritativeInputStore`：上传图像和画板 blob，返回稳定 input version。
- WebSocket 的 role-aware state projection：管理员看完整状态，玩家只看自身节点和允许共享的轨迹。

### 验收标准

- 管理员在一个浏览器修改 External Input，重新连接的大屏可恢复同一版本。
- 玩家可以看到“临时使用管理员轨迹”，但看不到其他玩家代码或敏感身份信息。
- 管理员重置 time、暂停、恢复后，所有允许同步的客户端收到同一个控制版本。
- 断线重连使用快照和序号，不重复应用输入或相机事件。

### 依赖

Milestone 1 的事件和 blob 存储。

## 5. Milestone 3：WGSL ABI 与运行安全

### 目标

把玩家 shader 的资源布局和输出语义从“约定”变成可验证的协议，避免能编译但不写输出、绑定错误或危险资源声明的代码进入 Authoritative Renderer。

### 建议接口

`ShaderContract` 只暴露少量高层操作：

- `BuildTemplate(nodeShape, format, blockSize)`
- `ValidateSubmission(source, nodeShape)`
- `InstallAcceptedShader(nodeID, validatedShader)`

内部可使用 WGSL tokenizer/parser 或模板区域重建，但调用方不需要知道语法检查细节。

### 规则

- 固定 group/binding、入口名称、workgroup size 和 metadata ABI。
- 玩家逻辑只允许出现在明确的编辑区域，或对完整文本做结构校验后接受。
- 禁止额外 storage buffer、sampler、纹理和未授权入口。
- 验证必须确认所有输出 binding 都有写入路径；必要时每帧清零输出作为防御性兜底。
- 大屏必须重新编译和验证服务端缓存的代码，不能信任玩家端成功标记。

### 验收标准

- 删除所有 `textureStore` 的代码无法作为“成功 shader”进入大屏。
- 改 binding、改入口、增加未授权资源、错误 output 数量都会被拒绝。
- 编译失败或运行验证失败不会替换上一版 Accepted Shader Version。
- 合法模板在三种格式和目标浏览器基线中通过。

### 依赖

Milestone 1（记录验证结果和拒绝原因）；可与 Milestone 2 并行设计。

## 6. Milestone 4：性能与 GPU 诊断模块

### 目标

把“帧率低”从主观现象变成可定位数据，并建立 RTX 5070 与普通设备的能力基线。

### 新增模块与接缝

- `RenderScheduler`：负责 rAF、GPU 提交防重入、暂停和降级策略。
- `GpuCapabilityReport`：统一收集 adapter、limits、features、格式和绑定上限。
- `FrameTelemetry`：只在本地开发或管理员诊断模式开启，记录 compute 编码、提交、呈现、device lost 等时间。

### 验收标准

- 大屏显示 GPU submission FPS、呈现 FPS、节点数、纹理数、分辨率和格式。
- 可区分 compute 过慢、canvas 呈现过慢、CPU 主线程阻塞和 device lost。
- 在目标 RTX 5070 测试机上，记录 960×540、1920×1080、3840×2160 和三种格式基线。
- 诊断日志不记录 token 或完整源码，满足当前日志策略。
- 当资源创建失败时，显示可操作原因并允许管理员降低配置重试。

### 依赖

当前 GPU canvas 方案已提供基础；可在 Milestone 3 后做正式基准。

## 7. Milestone 5：Render Snapshot、离线回放和导出

### 目标

把“保存一张图”扩展为可追踪、可复现的 Render Snapshot，并提供未来回放播放器的基础。

### 分阶段交付

1. 保存前冻结版本并写入 snapshot manifest。
2. 保存指定纹理的 PNG/JPEG，并关联 Round Event、shader 版本、图版本、输入版本、相机和 time。
3. 实现只读回放核心：按事件时间线重建 Authoritative State。
4. 再实现回放客户端；不要在首版继续把回放逻辑混入实时大屏。
5. 后续再加入 EXR/HDR 导出。

### 验收标准

- 同一个 snapshot manifest 能明确说明它使用了哪些版本。
- 回放到指定事件序号时，图和代码选择正确。
- 回放客户端不需要玩家在线或当前服务内存。
- 对允许的浮点差异，输出满足视觉一致性验收。

### 依赖

Milestone 1、2、4。

## 8. Milestone 6：生产运维与跨浏览器验收

### 目标

将现场单机部署变成可重复部署，并明确浏览器支持矩阵。

### 内容

- 配置校验、密钥不落日志、OIDC 回调检查和管理员会话策略。
- HTTPS/WSS 证书与内网访问说明。
- Windows 单机启动脚本和健康检查页面。
- Chrome/Edge 首要基线；Firefox/Safari 逐项验证，不在未验证前宣称完整支持。
- WebGPU device lost、断线、服务重启和活动终止的恢复演练。

### 验收标准

- 一份脱敏配置可以在新机器启动，并通过健康检查。
- OIDC、测试登录、管理员登录的启用范围明确。
- 现场断电/进程重启后的数据损失边界写入文档。
- 浏览器矩阵列出 GPU、驱动、WebGPU feature、格式和结果；统一 QualificationRecord schema 与人工步骤见 `docs/browser-qualification.md`。

### 依赖

Milestone 1、4；生产 OIDC 参数和测试设备。

## 9. 推荐实施顺序

推荐顺序调整为：

1. **Milestone 0 首版正确性稳定化**：先修复代码恢复、invalid fallback、冻结保存、暂停和 Round journal 生命周期。
2. **Milestone 1 Round Archive + RoundEvent**：让事实可保存、可校验、可读取。
3. **Milestone 2 服务端权威输入/相机/控制同步**：解决管理员轨迹和回放输入来源。
4. **Milestone 3 WGSL ABI 合同**：保护活动运行结果和 GPU 资源；可在 Milestone 1 后与 Milestone 2 并行。
5. **Milestone 4 性能诊断与资源能力报告**：建立 RTX 5070、分辨率和格式基线；性能测量可从 Milestone 0 开始。
6. **Milestone 5 Render Snapshot + 回放核心**：使用前四项提供的稳定事实。
7. **EXR/HDR 与高级导出**：在 snapshot 稳定后加入。
8. **Milestone 6 生产运维和跨浏览器矩阵**：将现场经验固化为部署能力。

Milestone 0 是硬门槛；Milestone 1 完成接口冻结后，Milestone 2/3/4 可以并行推进，但每个里程碑都必须通过自己的验收，不以“前端看起来能用”代替数据或协议验收。

## 10. 当前需要确认的路线选择

### R1. 回放范围

- [ ] 只回放事件、代码和图结构，不保证逐帧像素重现。
- [ ] 回放到任意 Round Event，保证视觉一致。
- [ ] 回放到任意时间点，尽量保证视觉一致。

### R2. 权威输入存储

- [ ] External Input 上传原图/画板 PNG，并在客户端拉伸。
- [ ] 服务端保存已经变换到纹理尺寸的像素数据。
- [ ] 两者都保存，以便诊断颜色和拉伸差异。

### R3. WGSL 编辑策略

- [ ] 玩家只能编辑模板标记区域。
- [ ] 玩家可以编辑完整文本，但提交时严格解析并校验 ABI。
- [ ] 首版模板不变，先只加入输出写入检查。

### R4. 性能目标

- [ ] 以 RTX 5070 + Chrome/Edge 作为唯一首要基线。
- [ ] 另指定最低活动设备和最低 FPS。
- [ ] 允许低端设备只作为玩家端，不能作为大屏端。

### R5. 生产与测试身份

- [ ] 测试玩家登录只保留开发配置。
- [ ] 增加管理员可创建临时测试玩家的受保护入口。
- [ ] 测试环境也接入 OIDC，不保留模拟登录。

## 11. 与当前审计发现的首版缺口对应关系

| 审计发现 | 处理里程碑 |
| --- | --- |
| 重连自动模板覆盖最后成功代码 | Milestone 0 / `PlayerCodeSession` |
| 本地非法代码仍继续运行旧 shader | Milestone 0 / `PlayerCodeSession` |
| WebGPU canvas 不能使用 2D context 清黑 | Milestone 0 / renderer fallback |
| 保存不是冻结快照且未按目标尺寸重渲染 | Milestone 0 / `CaptureSession` |
| Round terminate 后旧目录继续写入、Append 错误静默 | Milestone 0 / `RoundJournal` |
| 管理员轨迹未进入服务端 state | Milestone 2 / `TrajectoryCatalog` |
| 暂停时输入/相机语义不一致 | Milestone 0 / `RenderParameterTimeline` |
| p 未按配置范围随机生成 | Milestone 1 / `GraphGenerator` |
| 全屏纹理查看 click handler 缺失 | Milestone 0 后的 UI 修复 |
| 单玩家多设备并发上传未限制 | 已完成：同一 OIDC subject 新建玩家 session 时原子撤销旧 session |
| 管理员 device lost 未恢复 | Milestone 4 |
| `passTimeoutMs` 只有配置没有运行语义 | Milestone 4 |
| landing stylesheet 路径错误 | 已修复为 `/style.css` |
| 前端行为测试依赖 source regex | Milestone 0 后持续替换为模块/浏览器测试 |

## 12. 每个后续里程碑的交付格式

每个里程碑完成时必须同时交付：

1. 实现代码和模块接口文档。
2. 领域状态/事件或协议变更说明。
3. 自动化测试，测试穿过公开接口和真实 Adapter。
4. 人工验收步骤与已知限制。
5. 兼容旧轮次/旧配置的说明。
6. 如涉及不可逆选择，新增一份 ADR，而不是只在代码注释里解释。
