# ShaderContract：WGSL ABI 保护与规范化

Status: resolved

## Goal

将节点 ABI、metadata、绑定、入口、workgroup 和输出写入规则变成可验证协议。

## Requirements

- 固定 group/binding、格式、入口、workgroup 和 metadata layout。
- 提交源代码必须携带 ABI version/hash。
- 服务端拒绝 stale node/round/ABI submission。
- 浏览器和大屏对 accepted source 重新验证，不能信任客户端成功标记。
- 缺失输出写入、额外 binding、修改 protected declaration 等必须有稳定诊断。

## Open decision

完整文本编辑并严格解析/重写，还是改为受保护模板区域？当前路线推荐先使用受保护 body/helper 区域，保留完整源码导出。

## Acceptance

覆盖 wrong binding/order/access/format、入口/workgroup、metadata、额外资源、缺失输出写入和 stale ABI 的 mutation corpus。

## Comments

根据已确认的完整文本编辑要求，实现 ShaderFlow ABI v1 的浏览器结构校验。校验 bindings/access/format、metadata layout/uniform binding、main/workgroup、总资源数、write helper 和每个 output 的最终规范写入；先剥离 comments/strings，防止伪造。Renderer.validate 与 setShader 已接入，并返回 ABI version/hash。服务端 payload 携带 ABI hash、stale ABI 拒绝和更完整的 WGSL parser 仍属于下一阶段协议工作。
