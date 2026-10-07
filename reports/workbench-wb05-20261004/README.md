# WB-05 验收记录

日期：2026-10-04。外部模型原生流、结构化正文安全提取、持久草稿版本、撤回、
终态原子提交、停止和断线恢复已完成。

## 行为与实现

- OpenAI 兼容 Adapter 的 `Stream` 直接消费上游 SSE，不再等待 `Generate`。支持
  正文、finish reason、usage、取消和首块前瞬态重试；已经输出正文后的异常结束不重试。
- 原生 tool-call 的 ID、名称和参数分片先按 index 有界组装，完整 JSON 校验后才交给
  Eino 图。部分参数不执行，也不进入正文事件。
- Eino 主 Agent 全部通过 `Stream` 调用模型。增量解析器只识别根
  `claims[].text`，覆盖 JSON 转义、Unicode、代理对和任意跨帧分片；reasoning、
  key、gaps、引用 ID、工具参数和内部 JSON 不发布。
- `draft_started`、`answer_delta`、`draft_retracted` 带稳定事件序号和
  `draft_version` 持久化。输出修正以同一事务撤回旧版本后开始新版本。
- 最终 Response 保存、活动草稿撤回和终态事件在一个 PostgreSQL 事务内完成。
  注入终态事件写失败时三者全部回滚，浏览器不会先收到最终承诺。
- 停止先持久化撤回，再取消实际模型流；迟到分块因响应不再是 RUNNING 而不能写入。
  `RUNNING` 残留不自动重跑整张图，重启原子标记 `INTERRUPTED`。`CANCELING`
  只轮询实际执行清理，不重新调用模型。
- SSE 断线只结束订阅。重连按 `Last-Event-ID` 重放；前端同时按事件 ID 和
  `draft_version` 去重，撤回后的旧增量不能复活。
- 浏览器以“生成中草稿”显示 Markdown 增量，终态答案替换草稿。修复了异步会话
  元数据使 `state_version` 倒退并造成重命名 CAS 冲突的问题。
- `response_events` 已有 JSONB 负载和每响应单调序号，新增字段无需变更关系结构，
  因此 WB-05 没有新增迁移。

## REPLAY 验收

公开 HTTPS/SSE、真实 PostgreSQL、Eino 图和外部流式 Adapter 完成以下场景：

- 上游未结束时，公开 SSE 和浏览器先后收到多个真实正文增量。
- tool-call 参数跨帧组装；JSON/Unicode 任意分片；usage、finish、异常结束和取消。
- 无效引用触发草稿版本 1 撤回，再开始版本 2 修正并定稿。
- 生成期间知识撤回或设备快照更新，草稿撤回且最终状态为 `UNRESOLVED`。
- 中途断线后从游标续读，增量只出现一次；撤回和最终答案收敛一致。
- PostgreSQL 终态事件故障注入验证最终答案与撤回事务回滚，不自动重跑。
- 停止传播到外部 HTTP 请求，已有草稿撤回，迟到内容不保存。

这些使用确定性合成资料和模型协议桩，标记为 **REPLAY**。

## 真实模型

`deepseek-v4.1-flash` 通过公开登录、真实临时 PostgreSQL、原生知识工具与原生
模型 stream 完成合成手册问答：

| 指标 | 结果 |
| --- | --- |
| 最终状态 | ANSWERED / LIVE |
| 主模型调用 | 2 |
| 公开正文增量 | 10 |
| Prompt / Completion / Total tokens | 3118 / 138 / 3256 |
| 最终来源 | 1 个已发布合成手册片段 |

这是协议与引用烟测，不代表真实硬件运维质量。真实视觉模型仍由 WB-04/WB-08
单独记录。

## 浏览器

Go 同域托管生产构建，Playwright/Chrome 在 1440x1000 和 390x844 下验证：

- 模型完成前出现“生成中草稿”和首段真实增量；
- 最终复核后草稿标识消失，显示最终正文；
- 刷新、多会话、停止、删除、附件流程继续通过；
- 页面无横向溢出、无运行时异常，凭据不进入 localStorage。

截图：[真实增量草稿](wb05-live-draft.png)。

## 回归结果

- `go test -race ./... -count=1`：通过；acceptance 361.781 秒，包含真实
  PostgreSQL、Linux/gVisor Python runner 和独立 parser runner。
- `go vet ./...`、`go build ./...`、`git diff --check`：通过。
- `npm run build`：通过；生产 JS 371.07 kB（gzip 115.86 kB）。
- Playwright Chrome：原多会话/附件流程及 WB-05 实时草稿场景通过。

