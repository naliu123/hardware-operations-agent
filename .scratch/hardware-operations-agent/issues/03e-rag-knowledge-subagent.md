# 03e: RAG 内部知识检索子 Agent

**Task:** RAG-KNOWLEDGE-SUBAGENT

**Blocked by:** 03d（已完成）

**Status:** done

## 交付行为

将现有查询改写、混合召回、适用性复核、证据选择和引用整理收敛为一个内部知识检索子 Agent，并通过 Eino `AgentTool` 提供给主 Agent。主 Agent 只提交自包含的知识问题；设备快照从可信运行上下文注入，不能由模型参数伪造。

## 约束

- 子 Agent 只检索和返回已发布、适用、可核对的知识证据，不生成最终诊断结论，不执行设备操作。
- 工具结果必须包含结构化状态、证据与引用、适用性检查、实际检索问题、缺口、trace 和数据模式。
- 检索策略、候选规模、证据预算及模型选择过程留在模块实现内，不暴露给主 Agent。
- 外部模型和检索后端失败必须返回错误；不得静默降级并把结果冒充为完整检索。
- 现有公开 HTTP 搜索和问答契约保持兼容；缺少必要设备上下文时明确返回澄清状态。

## 验收条件

- [x] 自定义 Agent 实现 Eino `adk.Agent`，并由 `adk.NewAgentTool` 暴露为可调用工具。
- [x] 主问答 Graph 通过内部 AgentTool 获取知识，不再自行编排候选检索和证据选择。
- [x] 工具输入仅包含知识问题，设备上下文由可信 `context.Context` 注入并校验数据模式。
- [x] 使用真实临时文件存储和确定性外部依赖验证引用、适用性、缺口和错误语义。
- [x] QA-01 公开 HTTP 流程、全量 race、vet 和构建通过；确定性结果标记为 REPLAY。

## 完成证据

- `internal/agents/knowledge`实现结构化检索结果、自定义`adk.Agent`、`AgentTool`适配、
  可信设备上下文注入及LIVE/REPLAY一致性校验。
- 问答Graph通过`knowledgeagent.Invoke`调用工具；证据选择从主流程迁入子Agent，
  最终回答与引用二次校验仍由主流程负责。
- 工具证据携带不可变片段、来源、内容哈希、适用性及可回读URL；命中后重新读取
  权威修订，避免并发撤回资料进入工具结果。
- 结构化`FAILED`结果保留已消耗模型usage、证据选择记录和适用性检查，可信调用
  适配器再还原为Go error，失败结果不会进入答案生成。
- `internal/agents/knowledge/agent_test.go`使用真实临时文件存储验证工具schema、
  发布知识引用、缺少设备上下文、数据模式冲突及检索期间撤回。
- 2026-09-25执行`./scripts/go.sh test -race ./... -count=1`、
  `./scripts/go.sh vet ./...`、`./scripts/go.sh build ./cmd/...`及
  `git diff --check`均通过。公开HTTP验收使用真实临时文件存储和外部HTTP模型桩，
  确定性结果为REPLAY；本次未重跑真实DeepSeek质量评测。
