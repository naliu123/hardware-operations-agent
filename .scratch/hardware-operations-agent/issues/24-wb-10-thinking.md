# 24: WB-10 默认开启模型思考并展示独立思考区

**目标：** 默认启用 DeepSeek 思考，用户可以在回答上方查看实时思考内容，并在刷新或重开会话后恢复。

**Blocked by:** 19 (WB-05), 23 (WB-09)

**Status:** done

## 已确认需求

2026-10-07 用户明确要求：“默认开启思考，页面实现思考区展示”。
本要求更新早期工作台“推理内容不展示”的约定。思考与正文分区，
思考不作为已校验答案、知识证据或后续历史摘要。

## 交付行为

- DeepSeek 请求默认 `thinking.type=enabled`；读取 buffered/streaming
  响应的 `reasoning_content`，工具循环保留并回传 assistant 的思考字段。
- 用独立持久化事件交付思考增量与完成状态，沿用用户/会话归属及停止规则。
  当前状态、历史、终态和 SSE 重放一致，最终保存不能覆盖已接收思考。
- 同一回复的多次模型调用按段累计；失败、中断、取消时保留已经收到的内容，
  明确标记状态。只有思考而无正文/工具输出不能被误判成成功回答。
- 思考区生成时展开、完成后折叠，可手动展开/收起；安全渲染 Markdown。
  无思考内容的模型不生成虚假的思考文本。正文、引用、工具与取消继续可用。
- 思考内容有大小上限；不写入会话检索、模型历史摘要或默认外部追踪。

## 协议依据

[DeepSeek Thinking Mode](https://api-docs.deepseek.com/guides/thinking_mode)
于 2026-10-07 核对：`thinking.type=enabled`；输出 `reasoning_content`
与 `content` 同级，工具调用后续请求需要完整回传相应 assistant 思考字段。
思考模式下 temperature/top_p 不生效。OpenCode Go 当前实际端点另做 LIVE 验证。

## 验收条件

- [x] 外部模型 Adapter：默认开关、原生分片、buffered 响应、工具回传、大小限制。
- [x] 真实临时 PostgreSQL + 公开 HTTP/SSE：先思考后正文、重连、终态一致。
- [x] 失败/取消/重启保留思考，迟到增量及跨用户读取被拒绝。
- [x] 浏览器可看到真实增量思考，展开/折叠、刷新后恢复，正文独立完整。
- [x] 真实 DeepSeek 思考与原生工具调用通过；REPLAY 与 LIVE 结果分别记录。
- [x] 相关回归、race、构建通过，本地持续预览更新。

## 交付证据

[验收报告](../../../reports/workbench-wb10-20261007/README.md)。
公开 HTTP/SSE 与真实临时 PostgreSQL 覆盖正常流、工具循环、重放、取消、
断流、重启和超限；相关 acceptance/contracts race 回归通过。
Chrome 思考区专项测试 1/1 通过；LIVE DeepSeek 身份与原生知识工具两场景通过。
全仓测试、vet、Go 服务及前端构建完成，`http://localhost:8080` 已更新。
