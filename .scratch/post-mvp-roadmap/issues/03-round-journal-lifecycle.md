# RoundJournal 生命周期与持久化错误

Status: resolved

## Goal

隔离每个 Round 的持久化生命周期，并消除静默丢失事件的问题。

## Requirements

- 深模块接口：`Begin(snapshot)`、`Record(event)`、`StoreBlob(blob)`、`Close(outcome)`。
- terminate 后关闭 journal，后续报名和配置不能写入旧目录。
- 两个 Round 使用独立 durable sequence 和目录。
- Append/StoreCode/Start 失败具有明确策略：拒绝状态变更，或进入可观察 degraded 状态并阻止关键写入。
- 旧首版目录被识别为 legacy。

## Acceptance

- Round A 终止后执行报名/配置，A 的文件不增长。
- Round B 不引用 A 的代码或事件序号。
- 注入写入失败，调用方收到稳定错误，日志不包含 token/完整源码。

## Comments

已完成首阶段生命周期修复：`RoundJournal.Close()` 在 terminate 时关闭磁盘目录；后续非运行期事件不再进入旧目录。`emit` 现在返回并传播持久化错误，注册、配置、图更新、开始和代码更新在写入失败时回滚内存状态。完整的 manifest、durable sequence 和 degraded policy 留给 Round Archive 里程碑。
