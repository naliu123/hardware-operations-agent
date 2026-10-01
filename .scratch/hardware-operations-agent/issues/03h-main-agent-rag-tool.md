# 03h: 主 Agent 自主调用 RAG 子 Agent

**Task:** MAIN-AGENT-RAG-TOOL

**Blocked by:** 03e（已完成）

**Status:** done

## 交付行为

主问答 Agent 将现有 `retrieve_hardware_knowledge` AgentTool 注册给模型，
由模型决定检索问题与后续调用；通过 Eino Graph 表达模型、工具及最终校验循环。
RAG 子 Agent 保持查询改写、召回、适用性过滤、证据选择及引用构造职责。

## 约束

- 模型适配器支持 OpenAI 兼容 tools、tool_calls、tool_call_id 及不可变 WithTools。
- 工具参数只接受自包含 request，设备快照由可信上下文注入。
- 每次问答最多三次不同的知识检索；累计证据不超过 48 KiB，沿用一分钟总时限。
- 工具失败、未知工具、无效参数和超出调用预算明确失败，保留已产生的调用记录和用量。
- 最终答案仍需原文、发布状态、适用性与设备快照校验；无检索证据不得发布知识结论。
- 确定性外部 HTTP 模型和临时文件存储验收标记 REPLAY，真实模型联调单列。

## 验收条件

- [x] 公开 HTTP 问答经过模型 tool-call、RAG 子 Agent、工具结果回传及答案校验。
- [x] 覆盖不检索、改写后再检索、非法参数、未知工具、重复调用和预算耗尽。
- [x] 覆盖可信设备快照、伪造引用、并发撤回与失败后的观测保留。
- [x] 模型协议契约测试、公开 HTTP 回归、race、vet、构建通过。
- [x] 更新技术设计、实施计划及使用说明，记录真实模型联调范围。

## 完成证据（2026-10-01）

- `internal/adapters/chatmodel/tools.go`：不可变工具schema绑定、完整工具消息与调用ID、`auto`/`none`/强制工具选择；纯工具回复支持`content:null`。
- `internal/einoflow/qa.go`、`tools.go`：主模型自主决策，工具返回后继续模型循环；工具参数严格校验、三次上限、累计48 KiB、重复证据只回传ID、答案修正禁止新工具调用。
- `Response.knowledge_tool_calls`保存逐次查询、证据、缺口、错误与子Agent观测；最终失败仍保留已消耗的主模型和证据选择用量。查询改写用量仍在独立LLM trace中，未纳入响应汇总。
- `test/acceptance/main_agent_test.go`：原生外部HTTP模型协议与真实临时文件存储，覆盖先无结果后改问成功、直接结束、伪造设备参数、未知工具、重复查询/ID、单次及批量预算耗尽、累计证据预算/去重、未提供片段不可引用、后续模型故障及记录重启回读。结果均为REPLAY。
- QA-01/QA-02原有HTTP回归继续覆盖生成期间知识撤回、设备更新、持久化和型号版本匹配；`model_fixture_test.go`只在外部模型桩中适配既有断言。
- `test/contracts/chatmodel_tools_test.go`验证工具schema不污染基础模型，纯工具回复、usage及调用ID经过HTTP完整往返。

验证命令均通过：

```sh
./scripts/go.sh test -race ./... -count=1
./scripts/go.sh vet ./...
./scripts/go.sh build -o bin/ ./cmd/...
git diff --check
```

真实PostgreSQL及ES/embedding测试因未配置对应测试环境变量显式跳过；不计为此次外部接入验收。

## 真实模型联调（LIVE，单独记录）

```sh
# 使用README中的LIVE环境变量；不输出密钥
HWOPS_TEST_LIVE_MODEL=1 ./scripts/go.sh test ./test/acceptance \
  -run '^TestLiveMainAgentKnowledgeTool$' -count=1 -v
```

`deepseek-v4.1-flash`经OpenCode Go完成：`ANSWERED`、1次知识工具调用、2次主模型调用、2808 total tokens；测试耗时4.99秒。通过公开HTTP导入并发布合成手册、创建会话、提交问题、轮询结果及回读原文哈希，使用独立临时文件存储。

此次烟测使用本地词项检索，未启用查询改写、证据选择或ES；验证真实模型工具协议和引用闭环，不替代CCE全量回答质量、生产设备或资产系统验收。未变更历史评测指标。
