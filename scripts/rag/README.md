# CCE 文档 RAG 实验与全目录入库

主应用仍为 Go + Eino。Python 负责采集、本地 embedding、Phoenix 和评测编排；所有知识发布、检索与生成请求通过 Go 的公开 HTTP 接口。

2026-09-20 的 03a 单页质量实验仅采集用户指定的《高危操作一览》，不递归跳转链接。实验结果与资源释放记录见[统计报表](../../reports/rag-cce-20260920/report.md)：固定验收集 Recall@5 为 93.42%，已达到 60% 目标。`report.py` 仅读取已保存产物，离线生成 Markdown、CSV、JSON 及 PNG/SVG 图，不会再请求模型。

03b 全目录任务以同一页面左侧“用户指南”导航树为边界：记录 757 个导航节点，其中 116 个分类节点不重复发布，实际采集并发布 641 篇文档。结果见[全目录入库报告](../../reports/rag-cce-manual-20260920/report.md)。

## 环境与存储

本次验证环境为 macOS arm64、48 GiB 内存，无 Docker。安装项隔离在工作区：

| 内容 | 位置 / 端口 |
| --- | --- |
| Go 1.26.8 | `.tools/go1.26.8/go` |
| Python 3.12.14 | `.tools/rag-python/python` |
| Python 依赖环境 | `.tools/rag-venv`；精确版本见 `requirements.lock` |
| Elasticsearch 9.5.4 | `.tools/elasticsearch-9.5.4`，HTTP `127.0.0.1:19200`，transport `19300` |
| BGE small zh v1.5 | `127.0.0.1:18765`，512 维，CPU 2 线程 |
| Phoenix 20.14.0 | `http://127.0.0.1:16006`；仅 HTTP，未开启 gRPC listener |
| Go API | `127.0.0.1:18080` |
| ES 数据 | `.local/rag/elasticsearch-data` |
| Phoenix SQLite | `.local/rag/phoenix/phoenix.db` |
| 业务事实文件 | `.local/rag/hwops-state.json` |
| 运行日志与进程清单 | `.local/rag/` |
| 源文档、标签与划分 | `testdata/rag/cce/` |
| 统计与逐题结果 | `reports/rag-cce-20260920/` |
| 全目录语料与清单 | `testdata/rag/cce-manual/` |
| 全目录业务事实文件 | `.local/rag/hwops-cce-manual-state.json` |
| 全目录入库与核验结果 | `reports/rag-cce-manual-20260920/` |

ES 原生发行包自带 Java，已按官方 SHA512 校验；地址和摘要见 `docs/research/rag-es-phoenix-apis.md`。ES Basic 许可足够；融合由 Go 计算，不调用企业版 RRF。embedding 使用真实 Sentence Transformers PyTorch CPU 推理，CLS pooling + L2 normalization，不是模拟向量，也不是未经验证的 ONNX 导出。

新机器需安装同版本的 macOS arm64 ES、Go、Python。Python 环境可用 uv 创建，再按锁文件安装：

```sh
.tools/uv/uv venv --python .tools/rag-python/python/bin/python3.12 .tools/rag-venv
.tools/uv/uv pip sync --python .tools/rag-venv/bin/python scripts/rag/requirements.lock
```

首次启动 embedding 会从 Hugging Face 下载固定 revision；其缓存位于 `.cache/rag/huggingface`。已下载机器可以复用缓存。源文件已采集保留，不需要重新访问被防护的页面。`prepare_source.py` 使用正常浏览器导出的表格，`prepare_dataset.py` 生成冻结数据集；正式评测时不要重生成或修改标签。

## 启动与复现

以下命令在仓库根目录执行。当前本地 LIVE 默认通过 OpenCode Go 调用
`deepseek-v4.1-flash`。通过终端环境或权限为 `0600` 的
`.local/rag/model.key` 设置凭据，不要把实际密钥写到命令历史或文档。

```sh
.tools/rag-venv/bin/python scripts/rag/services.py start
.tools/rag-venv/bin/python scripts/rag/services.py status
./scripts/go.sh build -work -o .local/rag/hwopsd ./cmd/hwopsd
.tools/rag-venv/bin/python scripts/rag/services.py app --strategy dense
.tools/rag-venv/bin/python scripts/rag/ingest.py
```

`start` 返回不代表模型已加载完成。健康检查为 ES 根地址、embedding `/healthz`、Phoenix `/healthz`、Go `/healthz`；启动应用前先等待前三项就绪。所有监听均限制 loopback；服务没有生产 TLS、团队身份或多租户权限配置。

`ingest.py` 通过 HTTP 创建并发布知识，保存不可变修订及片段映射，并在 Phoenix 创建/回读开发、验收两个数据集。已有 `ingestion.json` 时复用记录；切换为空数据库时需使用新的实验目录和新的导入清单，不能混用旧 ID。

## CCE 用户指南全目录入库

采集器只读取左侧“用户指南”树，不递归正文交叉链接，也不纳入 FAQ、最佳实践和 API 参考等其他手册。分类节点进入 `navigation.json`，仅具有实际页面链接的文档进入 `manifest.json` 和 ES。正文保留标题、目录路径、规范来源 URL、更新时间、表格、代码、图片及链接。

首次采集需要安装固定版本 Chromium，并在有界的可见浏览器会话中通过站点正常安全验证。采集脚本不会自动操作或绕过验证；验证页、空正文和哈希不一致都会失败。

```sh
PLAYWRIGHT_BROWSERS_PATH=.tools/playwright \
  .tools/rag-venv/bin/playwright install chromium
PLAYWRIGHT_BROWSERS_PATH=.tools/playwright \
  .tools/rag-venv/bin/python scripts/rag/capture_cce_manual.py
```

采集结果保存在 `testdata/rag/cce-manual/`。`capture-checkpoint.json` 支持逐页续跑，`navigation.json` 固定目录范围，`manifest.json` 保存每页 SHA256 和整库 SHA256。已保存的 641 篇 Markdown 可直接复用，不需要再次访问站点。

全目录发布使用独立索引和业务状态。发布过程使用 REPLAY 模式，不调用 DeepSeek；每个片段仍通过真实本地 `BAAI/bge-small-zh-v1.5` 计算向量。

```sh
.tools/rag-venv/bin/python scripts/rag/services.py start
./scripts/go.sh build -work -o .local/rag/hwopsd ./cmd/hwopsd
.tools/rag-venv/bin/python scripts/rag/services.py app \
  --mode replay \
  --strategy hybrid \
  --index hwops-cce-manual-v1 \
  --state-path .local/rag/hwops-cce-manual-state.json
.tools/rag-venv/bin/python scripts/rag/ingest_cce_manual.py
.tools/rag-venv/bin/python scripts/rag/verify_cce_manual.py
```

`ingest_cce_manual.py` 在 `reports/rag-cce-manual-20260920/ingestion.json` 中逐修订保存 checkpoint，内容哈希变化时拒绝复用。`verify_cce_manual.py` 重新核对导航、文件哈希、ES 修订和片段计数，通过 Go 公开检索接口执行跨目录 BM25/Dense 抽样，并在全部检查通过后切换 `hwops-cce-manual` 与 `hwops-cce-manual-revisions` 稳定别名。

## 03a单页原始实验流程

```sh
.tools/rag-venv/bin/python scripts/rag/evaluate.py --name dev-bm25-baseline --split development --strategy bm25
.tools/rag-venv/bin/python scripts/rag/evaluate.py --name dev-dense-v1 --split development --strategy dense
.tools/rag-venv/bin/python scripts/rag/evaluate.py --name dev-hybrid-v1 --split development --strategy hybrid
.tools/rag-venv/bin/python scripts/rag/evaluate.py --name dev-dense-answers --split development --strategy dense --answers
.tools/rag-venv/bin/python scripts/rag/judge.py dev-dense-answers
```

根据开发集选择并冻结配置到 `selection.json` 后，才运行验收：

```sh
.tools/rag-venv/bin/python scripts/rag/evaluate.py --name heldout-dense-final --split heldout --strategy dense
.tools/rag-venv/bin/python scripts/rag/evaluate.py --name heldout-dense-answers --split heldout --strategy dense --answers
.tools/rag-venv/bin/python scripts/rag/judge.py heldout-dense-answers
.tools/rag-venv/bin/python scripts/rag/verify.py
.tools/rag-venv/bin/python scripts/rag/report.py
```

同名且配置一致的实验会续跑未完成题，已完成题不重发收费模型请求。要独立重测请换新的实验名称；原始报表使用上述固定名称。Go 应用的启动策略必须与 `--answers --strategy` 一致；仅检索实验可在请求中选择策略。

Phoenix 的 Datasets 页面可查看两个数据集、实验指标与单题结果；`hwops-cce-rag` 项目中有 CHAIN、RETRIEVER、EMBEDDING、LLM、EVALUATOR 追踪。逐题结果包含真实 trace_id 并与实验关联。

## 03a单页指标约定

- 主指标：38 道可回答验收题的宏平均 Recall@5。负例不计入召回率，单独统计拒答；开发/验收按证据组划分。
- 固定 Top K 为 5，两路候选各 20；RRF 常数为 60。没有增大 k 来追求达标。
- Recall@1/3/5、Precision@5、Hit@5、MRR@5、nDCG@5 由稳定原文行 ID 计算；引用完整性通过 HTTP 回读原文核对。
- `citation_precision/recall` 是对冻结标签的证据命中，额外有用背景引用也可能降低该 precision，不能直接等同于幻觉率。
- DeepSeek 对正确性、忠实度、相关性的评分使用固定 rubric，标为 `annotator_kind=LLM`；空答案忠实度为不适用。生成器与评审器为同一模型，分数存在相关偏差，数据集也未经运维专家认证。
- 此次是 55 行单页知识的离线质量实验，不能据此证明生产故障诊断、实时资产系统或 PostgreSQL 已验收。

## 03c全目录回答准确率评测

题集为`testdata/rag/cce-manual-eval/`：111题、20目录、75篇源文档，75开发/36验收。逐轮结果、bad case、改动和被否定的方向见[迭代记录](../../reports/rag-cce-manual-eval-20260920/iterations.md)。真实回答基线复核48/75（64%），RRF=1对照复核52/75（69.33%），LIVE-R04和LIVE-R05开发集复核均为72/75（96%）。原v1独立验收为34/36（94.44%），查看失败后已退休；95%目标仍需全新未使用题集验收。

检索模式只调用公开搜索接口；真实回答通过公开会话/消息接口运行。实验名绑定数据集、代码、索引、模型及融合参数，变更后需使用新名称。继续运行已有索引即可，不要重新入库。

```sh
.tools/rag-venv/bin/python scripts/rag/prepare_manual_eval.py
.tools/rag-venv/bin/python scripts/rag/services.py start
# 等ES、BGE和Phoenix健康检查就绪
./scripts/go.sh build -o .local/rag/hwopsd ./cmd/hwopsd
# 真实模式须先配置HWOPS_MODEL_API_KEY，或本地私有.local/rag/model.key（权限0600）
.tools/rag-venv/bin/python scripts/rag/services.py app \
  --mode live --strategy hybrid --rrf-constant 60 \
  --index hwops-cce-manual-v1 --state-path .local/rag/hwops-cce-manual-state.json
.tools/rag-venv/bin/python scripts/rag/evaluate_manual.py run \
  --name live-dev-my-run --strategy hybrid --answers
.tools/rag-venv/bin/python scripts/rag/evaluate_manual.py judge --name live-dev-my-run --judge-version v2
```

`run`、`judge`和`audit`默认先检查Phoenix健康状态，并在本地阶段完成后将dataset、
experiment、run和evaluation同步到Phoenix。Phoenix不可用或远端数量不一致时命令
明确失败，不会把本地结果伪装成已入平台。只有明确进行离线诊断时才能添加
`--no-phoenix`；命令会输出`PHOENIX_SYNC=SKIPPED`。

每次改动使用未存在的新`--name`，不要用当前源码覆盖历史R00/R01实验。程序在开始运行时归档源码并记录二进制哈希；运行中不要更换应用程序、索引或配置。比较RRF=1时重新启动`app --rrf-constant 1`并使用新的`--name`。仅检索实验可用`--mode replay`启动应用，并去掉评测命令中的`--answers`；结果标为`REAL_RETRIEVAL_ONLY`，不生成答案质量分数。

LIVE-R04候选通过`services.py app --evidence-selection --rrf-constant 1`显式启用。ES每路仍20条，额外同修订相邻片段至多8条，模型选择预览输入至多256 KiB（每片段初始1600字符），最终完整证据JSON至多5条/48 KiB。每题增加一次模型调用，usage计入生成总量；不要与原方案只生成答案的token成本混为一谈。当前模型temperature固定0、thinking关闭，来源目录/时间由程序展示。启动后先确认`http://127.0.0.1:18080/healthz`返回200，再发评测请求。

`--judge-version v1`保留最初评分提示；默认v2增加逐项实际答案摘录及禁止项校准。历史评分不能原地换版本。v2仍可能漏判，全部失败和至少20%成功的证据复核不可省略。评审API响应和usage逐次保存在`*-judge-attempts.jsonl`，最终报告按绑定当前响应哈希的复核结果生成bad case。

自动语义评审按必答事实逐项判定，整题全对才通过，不使用原03a的0/0.5/1均分作为准确率。模型评审结果标为LLM；生成和评审同模型的相关偏差需保留说明。`summary.audit_required_ids`列出全部失败和按ID哈希固定抽取的至少20%成功题。逐条复核原文、实际上下文、答案和评分后，创建如下审核JSON数组：

```json
[{"id":"题目ID","passed":false,"reviewer":"AI或人员标识","reason":"根据原文的具体判断","evidence_checked":["来源/章节/原文"],"answer_sha256":"对该row.result按encoded函数序列化后的SHA256"}]
```

```sh
.tools/rag-venv/bin/python scripts/rag/evaluate_manual.py audit \
  --name live-dev-my-run --audit-file /absolute/path/to/audit.json
```

缺少必需复核时`answer_accuracy`为空，仅显示`llm_judged_answer_accuracy`。审核不能覆盖REPLAY、服务错误或无效引用。开发集达到95%且复核完成后，才冻结配置并使用验收集：

```sh
.tools/rag-venv/bin/python scripts/rag/evaluate_manual.py freeze --name FINAL_DEVELOPMENT_RUN
.tools/rag-venv/bin/python scripts/rag/evaluate_manual.py run \
  --name final-heldout-v1 --split heldout --strategy hybrid --answers
.tools/rag-venv/bin/python scripts/rag/evaluate_manual.py judge --name final-heldout-v1
```

`FINAL_DEVELOPMENT_RUN`替换为真实开发实验名。验收同样需要审核；36题至少35题完整正确达到95%点估计，同时报告Wilson区间。查看验收bad case后若继续优化，旧验收集转为回归，新建未使用验收版本。本任务不把保留题反复调到通过，不自动安装其他模型替代DeepSeek。

快速复现“把未问的配置细节当成缺口”：

```sh
.tools/rag-venv/bin/python scripts/rag/probe_manual_answers.py --name gap-probe-my-run
```

它通过真实公开HTTP运行开发题016/018/022，断言可完整回答且没有多余gaps，保留全部响应和usage。此探针标为`LIVE_DIAGNOSTIC_NOT_ACCURACY`，仅验证该症状，不替代完整语义评测。

## 03d语义拆分、多向量与多问题召回

新流程保留原始Markdown作为引用事实，LLM只决定连续原文块的语义边界。随后为
每个片段生成2至5个可能回答的问题，并为实际包含的表格和图片生成检索描述。
长片段另外生成不超过BGE输入上限的重叠`PASSAGE`表示。ES使用nested多向量，
任一表示命中后都归并回同一个原始片段。

预处理可断点续跑，checkpoint绑定语料、模型和prompt哈希：

```sh
.tools/rag-venv/bin/python scripts/rag/enrich_cce_manual.py \
  --report reports/rag-cce-manual-rag-v2-20260923
```

若外部模型不可用，可启动固定revision的本地模型继续缺失文档；checkpoint逐文档
保留模型归属：

```sh
HF_HOME="$PWD/.cache/rag/huggingface" \
  HWOPS_LOCAL_CHAT_MODEL=Qwen/Qwen2.5-3B-Instruct \
  HWOPS_LOCAL_CHAT_REVISION=aa8e72537993ba99e69dfaafa59ed015b17504d1 \
  .tools/rag-venv/bin/python scripts/rag/local_chat_server.py

HWOPS_MODEL_ENDPOINT=http://127.0.0.1:18766/v1/chat/completions \
  .tools/rag-venv/bin/python scripts/rag/enrich_cce_manual.py \
  --model Qwen/Qwen2.5-3B-Instruct --workers 1
```

构建、启动和发布v2索引：

```sh
./scripts/go.sh build -o .local/rag/hwopsd ./cmd/hwopsd
.tools/rag-venv/bin/python scripts/rag/services.py app \
  --mode live --strategy hybrid --rrf-constant 1 \
  --index hwops-cce-manual-v2 \
  --state-path .local/rag/hwops-cce-manual-v2-state.json \
  --query-rewrite --multi-vector \
  --model Qwen/Qwen2.5-3B-Instruct \
  --model-endpoint http://127.0.0.1:18766/v1/chat/completions
.tools/rag-venv/bin/python scripts/rag/ingest_cce_manual.py \
  --report reports/rag-cce-manual-rag-v2-20260923 \
  --index hwops-cce-manual-v2 \
  --enrichment reports/rag-cce-manual-rag-v2-20260923/enrichment.json
.tools/rag-venv/bin/python scripts/rag/verify_cce_manual.py \
  --report reports/rag-cce-manual-rag-v2-20260923 \
  --index hwops-cce-manual-v2 --alias hwops-cce-manual
```

本次唯一纯RAG评测使用如下命令，不带`--answers`，因此不会运行答案生成或评分：

```sh
.tools/rag-venv/bin/python scripts/rag/evaluate_manual.py run \
  --name rag-v2-development-once --strategy hybrid \
  --report-dir reports/rag-cce-manual-rag-v2-20260923
```

结果与限制见[v2报告](../../reports/rag-cce-manual-rag-v2-20260923/report.md)。

## 03f表格与图片专项处理

`enrich_cce_assets.py`复用03d已核验的连续原文边界，并在引用原文之外生成
资产检索表示。说明型表格按数据行由LLM改写为自包含段落；数据型表格生成摘要后
再做一次逐行事实复核。图片以实际图片字节作为多模态输入，内容按URL缓存，同一
图片不会重复调用视觉模型。派生描述记录对应原文行号，并在回答上下文中插回原
表格或图片Markdown之后。

原始`content`、行号和SHA256保持不变。Elasticsearch另存增强后的
`retrieval_content`用于BM25；dense输入会去除图片URL、Markdown链接目标和
结构符号。公开检索及引用接口仍返回带原始图片位置标记的Markdown。

```sh
.tools/rag-venv/bin/python scripts/rag/enrich_cce_assets.py \
  --base-enrichment reports/rag-cce-manual-rag-v2-20260923/enrichment.json \
  --report reports/rag-cce-manual-assets-v3-20260925

.tools/rag-venv/bin/python scripts/rag/ingest_cce_manual.py \
  --report reports/rag-cce-manual-assets-v3-r1-20260925 \
  --index hwops-cce-manual-v3-r1 \
  --enrichment reports/rag-cce-manual-assets-v3-20260925/enrichment.json
```

专项评测集通过`prepare_asset_eval.py`生成后冻结。开发集用于bad case迭代；
配置冻结后，保留集只运行一次。最终数据集位于
`testdata/rag/cce-manual-assets-eval-v3/`，结果见
`reports/rag-cce-manual-assets-v3-r1-20260925/report.md`。

`evaluate_manual.py`默认同步Phoenix。历史结果或显式`--no-phoenix`产生的离线
结果可以单独执行幂等发布：

```sh
.tools/rag-venv/bin/python scripts/rag/evaluate_manual.py publish \
  --name asset-v3-r1-heldout-answers-v3-final \
  --data-dir testdata/rag/cce-manual-assets-eval-v3 \
  --report-dir reports/rag-cce-manual-assets-v3-r1-20260925/evaluation-v3
```

统一发布清单保存在
`reports/rag-phoenix-evaluations/manifest.json`，绑定dataset SHA256、split、
本地报告和Phoenix的dataset/version/example/experiment/run/evaluation ID。
重复执行会回读校验并跳过已发布记录。

全量历史迁移器只识别状态完整、名称与文件一致、逐题ID与冻结split完全一致的
评测JSON，`badcases`、入库清单、探针和核验结果不会导入：

```sh
.tools/rag-venv/bin/python scripts/rag/migrate_phoenix_evaluations.py --dry-run
.tools/rag-venv/bin/python scripts/rag/migrate_phoenix_evaluations.py
```

当前回读结果为10个dataset、33个experiment、1633个run和9987个evaluation；
清单、盘点及逐实验核验见`reports/rag-phoenix-evaluations/`。

迁移器会同时注册`hwops-rag-runtime-contract-v1` Code Evaluator，并绑定全部
dataset；`evaluate_manual.py publish`也会为以后新增的dataset自动绑定。可单独
幂等执行：

```sh
.tools/rag-venv/bin/python scripts/rag/register_phoenix_evaluators.py
```

该Evaluator使用Phoenix自带的Monty Python sandbox，检查`LIVE`数据模式、处理
终态和回答引用存在性。纯检索run标为`not_applicable`。它是确定性运行契约，
不替代已有的外部LLM语义评审、证据复核或最终答案准确率。

## 停止与资源释放

```sh
.tools/rag-venv/bin/python scripts/rag/services.py stop
.tools/rag-venv/bin/python scripts/rag/services.py status
```

停止脚本仅操作清单中 PID 与创建时间一致的本次进程，结束 Go、ES、Phoenix、embedding 及子进程，并清理旧兼容位置`.cache/rag/deepseek.key`。用户持久配置`.local/rag/model.key`（0600）、数据库、模型缓存和导出报告保留；停止后 Phoenix 页面不可打开，重新`start`可读取已有数据库。

当前 OpenCode Go 凭据位于`.local/rag/model.key`，权限0600且被`.gitignore`排除，停止服务保留该文件。`services.py app`默认使用`https://opencode.ai/zen/go/v1/chat/completions`和`deepseek-v4.1-flash`，也可通过`HWOPS_MODEL_ENDPOINT`、`HWOPS_MODEL`或对应命令行参数显式覆盖。旧`.local/rag/deepseek.key`和`.cache/rag/deepseek.key`只用于显式选择`api.deepseek.com`时的兼容配置，避免不同提供方的密钥串用。
