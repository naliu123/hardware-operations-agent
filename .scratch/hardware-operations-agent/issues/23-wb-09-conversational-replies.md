# 23: WB-09 自我介绍与澄清回复正文

**目标：** “你是谁”、问候及澄清能得到面向用户的正文，完成流式显示、持久化和多轮衔接。

**Blocked by:** 19 (WB-05), 21 (WB-07)

**Status:** done

## 故障证据

2026-10-07，本地真实模型将“你是谁”及后续“？”保存为 `UNRESOLVED`，
`answer` 为空，只有第三人称的缺口说明。公开 SSE 已交付 `answer` 终态事件，
浏览器已停止处理；本次 `net::ERR_ABORTED` 不是正文缺失的原因。
旧协议只允许有证据引用的 claims，缺少会话性回复的表达方式。

## 交付行为

- 增加有明确类型的 `reply`：`INTRODUCTION`、`GREETING`、`CLARIFICATION`，
  只用于身份、问候和向用户澄清需求；硬件事实、实时状态及工具结果仍走原有校验。
- 回复由模型生成，真实流提取 `reply.text`，最终正文与终态原子保存。
- 该字段不能与 claims、observations 或 conflicts 混用；格式错误仍须修正或失败。
- 多轮历史保留已保存的介绍与澄清正文；内部分析和缺口字段不作为流式正文。
- 无正文的未解决结果在页面明确标为“暂时无法回答”或“需要补充信息”。

## 验收条件

- [x] 真实临时 PostgreSQL、外部 REPLAY 模型与公开 HTTP/SSE 回归通过。
- [x] 模型结束前收到正文增量；定稿、续读、多轮上下文和澄清终态一致。
- [x] 非法类型、空文本、超长文本及绕过引用的混合输出被拒绝。
- [x] 本地真实模型及浏览器复测“你是谁”和后续追问，得到实际正文。
- [x] 更新本地预览，保留已有会话。

## 验证记录

- 修复前 `go test ./test/acceptance -run '^TestWorkbenchConversationalReply'`
  在 `answer_delta` 等待处失败，重现没有正文；修复后同一命令通过（4.060秒）。
  三轮覆盖介绍、澄清、问候及真实数据库历史；七种非法输出均返回
  `INVALID_MODEL_OUTPUT`。测试依赖通过 `HWOPS_TEST_DATABASE_URL` 注入，
  每次创建独立临时库，不使用预览业务库充当测试库。
- REPLAY 扩展回归：`go test -race ./internal/... ./test/contracts ./test/acceptance
  -run '^(TestWorkbenchConversationalReply|TestWB05|TestWB02|Test.*Answer|TestQA|TestMainAgent|TestVisibleText)'
  -count=1 -timeout=180s` 通过，公开验收部分耗时113.638秒。
  无筛选的 Eino/应用/domain/contracts 检查、相关 `go vet` 和后端构建通过。
- 前端 `npm run build`（TypeScript + Vite）通过，退出码0；刷新真实浏览器后，
  原失败记录显示明确的“暂时无法回答”，两轮新回复全文仍在且无处理中状态。
- LIVE：本地 `deepseek-v4.1-flash` 经真实浏览器提交“你是谁”和“？”；
  两轮分别完成为 `ANSWERED`、`NEEDS_CLARIFICATION`，均有非空正文和
  33个 `answer_delta`，随后交付对应终态事件。真实 usage 分别为
  2406/95/2501 和 2516/101/2617（输入/输出/总计），每轮一次模型调用。
  公开响应回读与事件重放核对一致；浏览器两轮均退出处理状态。
- 旧失败记录保留，新回复追加到原会话；本地服务已从新二进制重启。
  本次不重新声明全量知识库质量、生产部署或 Python 沙箱的验收结果。
