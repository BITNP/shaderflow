# 生产身份与存储加固

Status: ready-for-human

## Goal

在非本机部署前消除 URL/localStorage token、CSRF、凭证和磁盘失败风险。

## Requirements

- 优先 HttpOnly Secure SameSite opaque session；不在 URL hash/localStorage 返回 OIDC token。
- logout、绝对/空闲过期、session rotation。
- cookie mutation 使用 CSRF token 或严格 Origin policy。
- 登录限速、安全 headers、request/body/time limits。
- 管理员密码支持 hash 或 secret reference；明文仅开发模式。
- production config 拒绝 test login、plain HTTP 和默认 secret。
- archive disk-full/permission-loss 策略明确且可测试。

## Open decision

是否引入 SQLite 保存 session/catalog，blob 和 archive 仍使用文件系统。

## Acceptance

- token 不出现在 URL、localStorage 或日志。
- logout/expiry/CSRF/fixation/cookie flags 测试通过。
- disk fault injection 不产生“内存成功、存档静默失败”。

## Comments

已完成第一阶段浏览器身份加固：管理员、OIDC 玩家和测试玩家登录创建 256-bit opaque HttpOnly SameSite=Strict session，不在响应、URL hash、sessionStorage/localStorage 或 WebSocket query 暴露 credential；OIDC callback 直接重定向 `/player/`；实现 12h absolute/2h idle expiry、logout revocation、mutation Origin 检查和基础安全响应头。legacy bearer/HMAC 已移除；管理员支持 bcrypt hash，登录失败按客户端地址每分钟最多 5 次；`environment: production` 拒绝 test login、无 TLS、明文密码、非 bcrypt hash 和缺失 OIDC。opaque session 现原子持久化到 `dataDir/sessions.json`，重启恢复且 logout 撤销持久；损坏 session 文件明确降级为内存 Adapter 并记录错误。Archive 增加可写 health probe，ListenAndServe 在存储不可写时 fail closed，`/api/health` 报告 ready/degraded；HTTP 增加 read/write/idle timeout。运行期任一 Archive 写失败会被 Adapter 永久记忆为 degraded，Game admin state 暴露 `degraded/storageError` 并阻断之后权威 mutation，`/api/health` 同步返回 503。输入或 Snapshot blob 写入后若事件提交失败会立即删除该 blob。Archive catalog 现持久化到 `dataDir/catalog.json`，原子替换，记录 active/complete 状态与 event/checkpoint/blob 数量和字节数；缺失或损坏时从 manifest 扫描重建。通用 orphan blob GC 已实现：默认由调用方明确 dry-run，支持最小年龄，扫描 start state 和 durable input/snapshot events，跳过 active Round，删除运行同步修复 Catalog blob 统计；管理员路由为 `POST /api/admin/archives/gc`。新增只读 Archive Diagnose 与显式 Repair：检测 JSONL 截断、序列间隙和 blob hash 损坏；CLI `cmd/shaderflow-archive` 默认诊断，`-repair` 仅截断无效尾部、关闭 Round、更新 manifest/catalog，绝不伪造缺失事件。production secret reference 已实现：管理员 bcrypt hash 与 OIDC client secret 支持单选 env/file 引用，缺失、空值、双重来源均 fail closed，生产禁止 inline secret。production OIDC 现强制完整 resolved client secret、HTTPS issuer 与 HTTPS redirect URL。新增 `shaderflow -preflight` 机器可读部署检查，验证 landing/admin/player/style 静态资产、TLS cert/key 文件、证书私钥匹配、有效期与 DNS names，以及 Archive 可写性。该 issue 的已划定代码阶段完成，后续主要是真实证书/OIDC/secret mount 部署验证与外部 secret manager Adapter（如有需要）。
