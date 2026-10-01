# 硬件运维知识助手

QA-01、QA-02 及主 Agent 工具调用已实现：提交并发布资料 → 确定设备型号版本 → 主 Agent 按需调用 RAG 子 Agent → 根据证据继续检索或生成答案 → 校验并保存答案与设备快照 → 打开引用原文。

当前版本是单实例 HTTP 后端，支持通用知识和适用设备资料。演示采用合成资料和明确标记的 `REPLAY` 摘录，不代表模型效果或真实设备状态。

## 内部知识检索工具

`internal/agents/knowledge` 将查询改写、混合召回、适用性复核、证据选择、引用构造和缺口分类封装为 Eino `adk.Agent`，并通过 `adk.NewAgentTool` 提供 `retrieve_hardware_knowledge`。工具输入只有 `request` 字符串；设备快照由主流程通过可信上下文注入，不能由模型参数覆盖。

工具返回 `FOUND`、`NEEDS_CONTEXT`、`NOT_FOUND` 或 `FAILED` 及结构化证据、引用、实际检索问题、适用性检查、trace、数据模式和模型usage。主 Agent 通过 Eino `ToolCallingChatModel.WithTools` 注册该 AgentTool，由模型决定是否检索、查询内容及是否继续检索。问答 Graph 为 `agent → tools → agent` 循环，模型输出答案后进入 `validate`；无证据时只能返回具体缺口。

每次问答最多调用三次知识工具；大小写和空白归一化后的重复查询、未知工具或非法参数均明确失败。累计提供给主模型的证据 JSON 不超过 48 KiB，重复片段只返回已提供的 ID。响应的 `knowledge_tool_calls` 保存每次请求、状态、片段 ID、缺口和错误，失败后也保留。最终答案生成、原文与引用复核、设备快照更新检查由主 Agent 和业务层完成。

## 本地运行

需要 Go 1.25 以上，建议使用已验证的 Go 1.26.8。`scripts/go.sh` 优先使用 PATH 中的 Go；此工作区未安装系统 Go 时，使用 `.tools/go1.26.8/go`。依赖锁定在 `go.mod` 和 `go.sum`，缓存位于 `.cache/`。

终端一：

```sh
export HWOPS_API_TOKEN='local-demo-token'
export HWOPS_MODEL_MODE=REPLAY
./scripts/go.sh run ./cmd/hwopsd
```

终端二：

```sh
HWOPS_API_TOKEN='local-demo-token' ./scripts/go.sh run ./cmd/hwops-demo
```

演示程序会写入并发布 `testdata/manual-general.md`，询问“蓝灯表示什么？”，输出答案、引用地址和对应原文。每次运行新增一份修订。引用接口需要同一 Bearer token，直接在未带令牌的浏览器中打开会返回 401。

本地 LIVE 模式当前配置为 OpenCode Go 的 `deepseek-v4.1-flash`。密钥保存在被 Git 忽略的
`.local/rag/model.key`，权限必须为 `0600`：

```sh
export HWOPS_API_TOKEN='local-demo-token'
export HWOPS_MODEL_MODE=LIVE
export HWOPS_MODEL_ENDPOINT='https://opencode.ai/zen/go/v1/chat/completions'
export HWOPS_MODEL='deepseek-v4.1-flash'
export HWOPS_MODEL_API_KEY="$(<.local/rag/model.key)"
./scripts/go.sh run ./cmd/hwopsd
```

服务默认监听 `127.0.0.1:8080`，数据保存在 `.local/state.json`。Ctrl+C 优雅停止；已保存的答案仍可读取，未完成任务重启后在原 60 秒总预算内继续。服务重启后无需重新导入知识。

QA-02 演示使用 `testdata/qa02-devices.json`，在同一会话对两种型号、各两个版本询问“蓝灯表示什么？”，逐一核对回答、设备快照和引用原文。先停止旧服务，再用独立的新存储启动，以免已有通用演示资料影响结果：

```sh
# 终端一，沿用前述 HWOPS_API_TOKEN
HWOPS_MODEL_MODE=REPLAY HWOPS_STATE_PATH=.local/qa02-demo.json ./scripts/go.sh run ./cmd/hwopsd
# 终端二
HWOPS_API_TOKEN='local-demo-token' ./scripts/go.sh run ./cmd/hwops-demo -qa02
```

每次演示使用独立测试设备和型号命名空间；重复运行会保留新增资料和答案。

## 配置

| 环境变量 | 默认值 / 作用 |
| --- | --- |
| `HWOPS_API_TOKEN` | 必填；本地单操作者令牌 |
| `HWOPS_LISTEN_ADDR` | `127.0.0.1:8080` |
| `HWOPS_STATE_PATH` | `.local/state.json`；开发文件存储 |
| `HWOPS_DATABASE_URL` | 设置后改用 PostgreSQL，不回退到文件 |
| `HWOPS_MODEL_MODE` | `LIVE`；显式设置 `REPLAY` 才启用演示摘录 |
| `HWOPS_MODEL_ENDPOINT` | 完整的 OpenAI 兼容 `/chat/completions` URL；当前本地配置使用 OpenCode Go |
| `HWOPS_MODEL` | 上游模型名称；当前本地配置为 `deepseek-v4.1-flash` |
| `HWOPS_MODEL_API_KEY` | 上游需要时使用；通过 Bearer header 发送 |
| `HWOPS_ES_URL` | 可选 Elasticsearch 地址；未设置时使用本地词项检索 |
| `HWOPS_ES_INDEX` | `hwops-fragments-v1`；原文修订另外保存在同名前缀的 `-revisions` 索引 |
| `HWOPS_EMBED_URL` | ES 模式必填；本地 `/v1/embeddings` 服务基地址，512 维 |
| `HWOPS_RETRIEVAL_STRATEGY` | ES 默认 `hybrid`；可选 `bm25`、`dense` |
| `HWOPS_RRF_CONSTANT` | 默认 `60`，允许 `1..1000`；混合检索融合参数，03c实验显式使用`1` |
| `HWOPS_EVIDENCE_SELECTION` | 默认 `false`；启用额外模型证据选择，ES每路20候选加最多8个相邻片段，最终仍至多5条/48 KiB |
| `HWOPS_PHOENIX_ENDPOINT` | 可选完整 OTLP/HTTP 地址，如 `http://127.0.0.1:16006/v1/traces` |
| `HWOPS_PHOENIX_PROJECT` | `hwops`；Phoenix 项目名称 |

真实主模型通过 Eino `ToolCallingChatModel` 适配器调用，上游须支持 OpenAI 兼容的 `tools`、`tool_calls` 和 `tool_call_id`。设置 `HWOPS_MODEL_MODE=LIVE`、模型地址和模型名即可接入；单次调用上限 30 秒，问答总预算 60 秒（含排队、工具循环和一次输出修正）。未配置或调用失败返回 `MODEL_UNAVAILABLE`，不自动切换为演示模式。`LIVE` 表示真实模型调用路径；当前版本尚未接入实时设备观测。

模型请求显式设置 `temperature=0`，`deepseek-*` 另设 `thinking.type=disabled`。主模型使用 `tool_choice=auto`、`parallel_tool_calls=false`；答案修正时禁止调用工具。工具绑定不改变 RAG 改写和证据选择使用的基础模型。当前本地 LIVE 配置通过 OpenCode Go 的 `https://opencode.ai/zen/go/v1/chat/completions` 调用 `deepseek-v4.1-flash`；请求携带项目专用 `User-Agent`，同一业务会话使用稳定的 `x-opencode-session`。主模型及证据选择已返回的实际 usage 随响应保存，包括后续校验失败的请求。Phoenix 仅在显式配置后导出问题、知识片段和答案，不导出认证 header 或密钥。

启用证据选择时每次 RAG 工具检索增加一次模型调用（输出无效可修正一次）：候选预览整体最多256 KiB，最终完整证据JSON最多48 KiB。响应的`evidence_selection`记录最后一次检索的选择结果，逐次记录位于`knowledge_tool_calls[].evidence_selection`；公共搜索接口继续遵守原有top_k限制。引用及原文接口的`document_context`保留文档头中的目录和更新时间，最终答案按引用修订列出来源范围。

PostgreSQL 使用 `pgx/v5`，启动时应用 `migrations/001_core.sql` 和 `002_devices.sql` 的幂等建表语句，后者添加设备快照和不可变版本规则。该版本尚未引入迁移版本管理；后续表结构演进需增加正式迁移。数据库需允许建表和建索引，连接池至少两个连接；一个连接持有实例锁。同一数据库只运行一个应用实例，暂不支持分布式 worker 或运行租约故障恢复。文件存储也通过文件锁限制单实例，兼容已有 QA-01 文件。

本地令牌对应固定的 `local-operator`，知识发布和查询共用该权限；团队身份、专家角色权限和生产部署尚未接入。

## HTTP 接口

除 `/healthz` 外，所有接口需要 `Authorization: Bearer <token>`。JSON 请求体只接受支持的字段。

| 方法与路径 | 行为 |
| --- | --- |
| `GET /healthz` | 进程存活检查，不代表模型和数据库可用 |
| `POST /v1/knowledge/revisions` | 接收 `title`、`source`、`content`、`applicability`，返回 `DRAFT` 修订 |
| `POST /v1/knowledge/search` | `query`、可选 `device_id`、`strategy`、`top_k`（默认 5，最多 8）；返回适用片段、出处、得分和 trace_id |
| `POST /v1/knowledge/revisions/{id}/publication` | `{"decision":"PUBLISH"}` 或 `{"decision":"WITHDRAW"}` |
| `GET /v1/knowledge/revisions/{id}` | 修订原文、适用范围、片段和当前发布状态 |
| `GET /v1/knowledge/revisions/{id}/fragments/{fragment}` | 引用原文、章节、行号、哈希、来源与当前发布状态 |
| `POST /v1/knowledge/version-policies` | 登记绑定型号的版本顺序，返回不可变规则与 `id` |
| `GET /v1/knowledge/version-policies/{id}` | 读取版本顺序及来源 |
| `PUT /v1/devices/{id}` | 完整替换设备当前快照；观察时间必须更新，否则返回 409 |
| `GET /v1/devices/resolve?q=名称或编号` | 精确解析已登记 ID、名称或别名，返回 `RESOLVED`、`AMBIGUOUS` 或 `NO_RECORD` 及候选 |
| `POST /v1/conversations` | 请求体 `{}`，创建会话 |
| `POST /v1/conversations/{id}/messages` | `{"text":"蓝灯表示什么？","device_id":"rack-a"}`，也可用互斥的 `device_query`，返回 HTTP 202 和响应标识 |
| `GET /v1/responses/{id}` | 轮询保存的处理状态和最终结果 |

响应从 `QUEUED`、`RUNNING` 进入 `ANSWERED`、`PARTIAL`、`UNRESOLVED` 或 `FAILED`；设备有歧义或缺少必要版本时返回 `NEEDS_CLARIFICATION`。有依据的结论与具体缺口并存时返回`PARTIAL`；无适用知识返回 `UNRESOLVED` 和缺口。输出或引用经一次修正仍无效返回 `INVALID_MODEL_OUTPUT`；重复知识查询或超过三次调用返回 `TOOL_BUDGET_EXCEEDED`；总时限到达返回 `DEADLINE_EXCEEDED`。最终答案在校验后一次性保存。

## 设备与版本匹配

设备快照录入示例（`PUT /v1/devices/rack-a`；观察时间替换为实际采集时间）：

```json
{
  "name": "一号机柜设备",
  "aliases": ["机柜A"],
  "model": "fixture/Atlas",
  "firmware": "R10",
  "driver": "D2",
  "hardware_revision": "H1",
  "source": "fixture://inventory",
  "observed_at": "2026-09-19T00:00:00Z",
  "data_mode": "REPLAY"
}
```

`source`、`observed_at`、`data_mode` 必填；模式必须与服务相同。缺失型号或版本保留未知，PUT 中省略的字段会清空。录入不等于实时采集；当前尚未接入资产或监控系统。

提问可传 `device_id`、精确的 `device_query`，或在 `text` 中写已登记设备名。英文标识按完整词元匹配，中文名称按文本出现匹配；不做模糊推断。多个候选返回澄清，文字与显式设备选择冲突返回 `device_resolution.status=CONFLICT`。每条消息独立选择设备，暂不自动沿用上次设备或解析“它”（QA-05）；未选择设备时仍可回答通用知识，无适用通用资料则返回缺口。

知识的 `applicability` 支持：

```json
{"scope":"GENERAL"}
```

```json
{"scope":"DEVICE","model":"fixture/Atlas","firmware":"R10","driver":"D2","hardware_revision":"H1"}
```

只校验资料声明的条件；固件、驱动、硬件修订分别精确匹配，缺版本不会默认采用最新版。`GENERAL` 禁止附带型号或版本限制。

版本范围先通过 `POST /v1/knowledge/version-policies` 登记产品规则：

```json
{
  "model": "fixture/Atlas",
  "source": "fixture://Atlas/version-order",
  "firmware_order": ["R2", "R10", "R3"],
  "driver_order": ["D2", "D10", "D3"]
}
```

知识引用返回的规则 ID：

```json
{
  "scope": "DEVICE",
  "model": "fixture/Atlas",
  "policy_id": "上一步返回的id",
  "firmware_range": {"min":"R2","max":"R3"}
}
```

范围包含两端，严格按登记顺序判断，不猜测字典序或语义版本。支持 `firmware_range`、`driver_range`；同一维度不能同时指定精确值和范围。规则不可修改，修正规则需创建新 ID 并发布引用它的新知识修订。缺规则、规则型号不符、未登记版本或无法解释的范围返回 `UNKNOWN`；任一已知条件不符优先返回 `MISMATCH`。

响应的 `applicability_checks` 保存候选修订的三态及逐字段理由，只有 `MATCH` 片段进入模型和最终引用。`device_context` 固定本次快照，`context_revision` 等于其 `snapshot_id`；设备资料引用携带同一快照 ID。生成后复核知识发布状态和适用性，保存答案时原子核对设备快照；期间设备更新则返回 `UNRESOLVED`，提示重新提问。历史答案仍保留原始上下文。

## 验证

普通终端：

```sh
./scripts/go.sh test -race ./...
./scripts/go.sh vet ./...
./scripts/go.sh build -o bin/ ./cmd/...
```

当前 TRAE 沙箱限制测试结束时删除临时目录，验证时保留构建与测试产物：

```sh
HWOPS_KEEP_TEST_ARTIFACTS=1 ./scripts/go.sh test -work -race ./... -count=1 -v
./scripts/go.sh vet -work ./...
./scripts/go.sh build -work -o bin/ ./cmd/...
```

保留的文件均在忽略版本管理的 `.cache/` 中；普通测试仍使用 `t.TempDir()` 自动清理。

验收通过真实 HTTP、Eino 图和临时文件存储验证：发布与引用、无设备上下文时的设备资料排除、撤回与历史原文、模型故障和伪造引用、生成期间撤回、重启恢复，以及鉴权和输入校验。QA-02 增加两型号各两版本、产品版本顺序与三态、名称歧义与冲突、独立版本维度、同会话并发切换、生成期间设备更新、旧文件升级及规则持久化。型号版本对照和并发测试经过外部 HTTP 模型适配器，确定性结果标记 `REPLAY`。

模型协议测试只替换外部 HTTP 端点，覆盖请求格式、失败、重定向、异常响应和读取超时。主 Agent 的 REPLAY 验收另覆盖原生 tool-call、首次无结果后换问题重查、不检索、非法工具和设备参数、重复调用、调用次数与累计证据预算、伪造引用及失败记录持久化。

真实模型工具调用烟测使用隔离临时文件存储，通过公开 HTTP 发布合成手册、提问并核对引用：

```sh
# 沿用前述 LIVE 模型环境变量，不连接运行中服务或其知识库
HWOPS_TEST_LIVE_MODEL=1 ./scripts/go.sh test ./test/acceptance -run '^TestLiveMainAgentKnowledgeTool$' -count=1 -v
```

2026-10-01 的 `deepseek-v4.1-flash` 联调通过：1次知识工具调用、2次主模型调用，返回`ANSWERED`和可回读引用。该烟测使用本地词项检索，未启用查询改写或证据选择，不代表 CCE 全量质量评测；记录见[03h工单](.scratch/hardware-operations-agent/issues/03h-main-agent-rag-tool.md)。

PostgreSQL 测试需要专用测试数据库，会建表并保留测试记录：

```sh
# 先在环境中设置 HWOPS_TEST_DATABASE_URL
./scripts/go.sh test ./test/acceptance -run TestPostgreSQL -v
```

没有该变量时显式跳过。真实 PostgreSQL、内网设备模型及资产系统尚未联调验收。外部 DeepSeek 与指定 CCE 文档的独立 RAG 评测见下节，不等同于生产硬件故障诊断验收。

## Elasticsearch / Phoenix 文档 RAG 实验

03a 质量实验的源文档为华为云 CCE《高危操作一览》。运行与复现步骤见 [RAG 运行说明](scripts/rag/README.md)，数据集在 `testdata/rag/cce/`，实验和统计产物在 `reports/rag-cce-20260920/`。

本次真实实验已完成：开发集Recall@5由BM25的97.83%提升至Dense的100%，固定独立验收集为**93.42%**（38道可回答题，目标60%）。真实DeepSeek生成与评审各66次，Phoenix保存2个数据集、6个实验和607个span。完整指标、CSV、统计图、错误案例及资源释放记录见[统计报表](reports/rag-cce-20260920/report.md)。LLM正确性均分91.46%，属于同模型评审，不能当作人工准确率。

03b 已将该页面左侧“用户指南”全目录采集并发布到独立索引：757 个导航节点中有 116 个分类节点、641 篇实际文档，共生成 641 个不可变修订和 3539 个检索片段。导航、正文和整库哈希均已保存，BM25 与 Dense 的 10 次跨目录公共接口抽样全部命中。稳定别名为 `hwops-cce-manual` 和 `hwops-cce-manual-revisions`，语料位于 `testdata/rag/cce-manual/`，证据见[全目录入库报告](reports/rag-cce-manual-20260920/report.md)。此次批量发布标为 REPLAY，向量由真实本地 BGE 推理生成，不计作 DeepSeek 答案质量评测。

03c 正在进行全目录回答准确率迭代：已冻结[111题评测集](testdata/rag/cce-manual-eval/README.md)，75题开发、36题独立验收。DeepSeek凭据已配置并实际使用`deepseek-flash`；真实回答基线经Agent证据复核为48/75（64%），仅调整RRF后为52/75（69.33%），LIVE-R03为58/75（77.33%）。后续证据选择与完整原文修复正在评测，**95%回答准确率尚未验收**。逐轮bad case、已证实根因、未采纳方向和继续步骤见[迭代记录](reports/rag-cce-manual-eval-20260920/iterations.md)。

03f 已增加表格与图片专项处理。说明型表格按行改写，数据型表格生成并复核摘要；
1095张唯一图片由`deepseek-v4.1-flash`实际读取，视觉描述按原位置加入检索和回答
上下文，原始Markdown及引用哈希保持不变。最终`hwops-cce-manual-v3-r1`包含
5585个片段和42571个nested向量。未使用的20题专项保留集Recall@5为95%，
真实问答经模型评审和固定证据复核为20/20，引用完整性100%。完整过程、限制及
复现见[专项报告](reports/rag-cce-manual-assets-v3-r1-20260925/report.md)。

03g 已将历史 RAG 评测统一迁移到 Phoenix。当前可回读 10 个 dataset、33 个
experiment、1633 个 run 和 9987 个 evaluation；后续全目录评测默认同步 Phoenix，
服务不可用时明确失败。1个确定性Code Evaluator已绑定全部10个dataset。迁移清单与核验见
[Phoenix评测报告](reports/rag-phoenix-evaluations/README.md)。

ES 模式将原文、片段和向量写入 Elasticsearch，业务存储继续持有发布状态和不可变知识事实。发布必须等待向量与索引就绪；检索先过滤已发布且适用的修订，再应用每路 20 条候选预算。BM25 使用内置 CJK 分词，向量为本地 `BAAI/bge-small-zh-v1.5`，混合策略在 Go 中执行 RRF，不依赖 ES 企业版排名功能。每次工具检索返回最多 5 条已核对片段，多次调用累计受主 Agent 的 48 KiB 预算限制。

启用实际 ES/embedding 的 HTTP 集成测试：

```sh
HWOPS_TEST_ES_URL=http://127.0.0.1:19200 \
HWOPS_TEST_EMBED_URL=http://127.0.0.1:18765 \
HWOPS_KEEP_TEST_ARTIFACTS=1 ./scripts/go.sh test -work -race ./test/acceptance -run TestElasticsearchPublicFlow -v
```

该测试实际访问 ES 和本地 embedding，生成部分使用明确标记的 `REPLAY`；DeepSeek 真实生成及模型评审由评测脚本单独运行。测试使用独立 ES 索引并在结束后删除。

## 当前范围

- 每次默认本地检索取至多 8 个片段，可选 ES 取至多 5 个片段。主 Agent 支持最多三次自主检索，累计证据最多 48 KiB；完整的冲突处理和 QA-03 综合验收仍为后续工作。
- 引用校验保证片段存在、属于召回集合、内容一致且资料适用；引用是否足以支持自然语言结论仍需人工或模型评测。
- 模型输入保留文档标题、章节和来源。可确认的部分答案保留引用并返回`PARTIAL`及具体缺口；设备快照在保存前变化时同样废弃旧指导。缺口和部分答案均可重启恢复。
- 知识HTTP接口只接收文本或Markdown，不直接接收PDF、扫描件或图片文件；CCE
  离线入库流程可读取Markdown引用的远程图片并生成视觉描述。实时监控、多轮指代、
  幂等请求、SSE和持续诊断按后续工单交付。
- 配置指导只返回文本，当前问答入口没有设备命令执行通道。

设计与任务：[技术设计](docs/design/hardware-operations-agent-technical-design.md) · [实施计划](docs/design/hardware-operations-agent-implementation-plan.md) · [QA-01 工单](.scratch/hardware-operations-agent/issues/01-qa-01.md) · [QA-02 工单](.scratch/hardware-operations-agent/issues/02-qa-02.md) · [03b 工单](.scratch/hardware-operations-agent/issues/03b-cce-full-manual-ingestion.md) · [03e 工单](.scratch/hardware-operations-agent/issues/03e-rag-knowledge-subagent.md) · [03g 工单](.scratch/hardware-operations-agent/issues/03g-phoenix-evaluation-platform.md) · [03h 工单](.scratch/hardware-operations-agent/issues/03h-main-agent-rag-tool.md)。
