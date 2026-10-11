# 03j：工具预算耗尽后收尾

**Task:** TOOL-LIMIT-FINALIZATION

**Blocked by:** 03h、03i、WB-05（均 done）

**Status:** done

## 交付行为

用户反馈工具超限直接中止体验差。停止重复或超预算的工具调用后，在原请求预算内
禁止工具并生成收尾答案；已返回的证据继续经过原文、语义、适用性与保存事务校验。
资料充分可回答，部分充分返回 PARTIAL，无依据返回具体缺口与补充建议。

## 边界

- 知识仍最多执行三次，证据仍最多 48 KiB；批量请求只执行预算内的有效前缀。
- 重复查询及超限请求保留实际参数与拒绝原因；每个 assistant tool_call 都有对应
  tool 消息，拒绝的调用不得声明执行成功或成为事实依据。
- 收尾禁止所有工具；若模型继续调用工具，仍明确失败。收尾生成与一次答案修正、
  语义复核共用原有时间和模型预算，不新增重试循环。
- 未知工具、重复调用 ID、无效参数、工具服务失败、模型故障、复核故障及取消
  保留原有失败行为。草稿不直接提升为最终答案。
- 不接入或改写知识库，不改历史回答，不增加设备执行权限。

## 验收

- [x] 公开 HTTP + 外部模型 REPLAY 重现两次检索后批量再申请两次，执行不超过三次。
- [x] 重复查询、无证据、批量超限都有正常收尾；伪造引用、非法工具及收尾再调用工具不放行。
- [x] 已有证据、实际用量、拒绝记录与最终状态重启可回读。
- [x] 收尾继续语义复核，真实 PostgreSQL/SSE 草稿撤回与终态正确。
- [x] 针对性 race、vet、构建与真实模型烟测通过。
- [x] 更新运行规则并部署到已有本地工作台。

## 完成证据（2026-10-11）

- `TestToolLimitFinalizesThroughPublicHTTP`：原版六场景稳定失败，变更后九场景通过；
  真实临时文件库与外部 HTTP 模型 Adapter，包含 2+2、重复、无证据、批量4、
  完整回答、禁止工具不遵守、伪造引用、模型失败及预算外非法参数。结果为 REPLAY。
- `TestToolLimitWorkbenchFinalizationKeepsGroundingAndDraftLifecycle`：真实临时
  PostgreSQL 与原生 SSE，重复查询后关闭工具，错误草稿经 UNSUPPORTED 撤回、
  一次修正再复核后 PARTIAL；2个草稿、1次 SUPERSEDED、1次 FINALIZED、1次 answer。
- `TestToolLimitWorkbenchPrivateReadsFinishWithoutMoreTools`：12次私有读取后
  第13次模型调用关闭工具并报告缺少附件，原请求模型预算仍生效，结果为 REPLAY。
- 针对性 race 覆盖 MainAgent、OnlineGrounding、WB05、Observation、模型协议、
  全部 internal/einoflow 与 test/contracts；`go vet ./...`、hwopsd 构建及
  `git diff --check` 通过。未执行无关全量容量和隔离测试。
- 在确认无活跃问答后更新本地 launchd 工作台。真实 DeepSeek 对磁盘操作问题
  执行3次知识检索后正常返回 UNRESOLVED、具体缺口及补充建议，3次返回 usage 的
  模型调用、9092 tokens；无工具超限错误。响应 ID：24L54TXK5BJWYWM4SG77YFMNRZ。
- 独立临时文件库 `TestLiveMainAgentKnowledgeTool`：ANSWERED、1次知识工具、
  4次返回 usage 的模型调用、8169 tokens、1次 VERIFIED 原文复核。
  真实模型烟测不代替文档库全量质量或真实磁盘操作正确性验收。
