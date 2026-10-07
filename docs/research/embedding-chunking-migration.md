# Embedding 模型与文档拆分持续迭代时的 RAG 索引迁移

> 调研日期：2026-10-07
>
> 范围：本项目 Go / CloudWeGo Eino、Elasticsearch、BGE、Nested 多向量索引与 Phoenix。本文只给出研究结论，不修改实现。

## 1. 结论

把一次可上线的 RAG 物理索引定义为**不可变 index generation**。一个 generation
绑定拆分和派生表示配置 `chunk_profile_id`、embedding 完整推理契约
`embedding_profile_id`、维度/相似度/向量结构 `vector_schema_id`，以及索引文本
schema。文档内容通过不可变 source revision 持续演进；manifest 固定记录回填
snapshot，运行状态另记已消费的 source cursor。检索、改写、融合和证据选择参数
使用独立 `retrieval_profile_id`，允许同一物理 generation 上做无重建灰度。

推荐策略：

- 少量文档内容更新，且其余 profile 完全不变：把新 source revision 增量应用到当前
  generation，不改变 generation 身份，受 revision 发布门禁保护。
- embedding 模型、维度、相似度、chunker、输入清洗或多向量规则变化：新建物理索引，全量回填，追平增量，完成 Phoenix 离线评测和线上影子读，再原子切 alias。
- 仅查询时参数变化：创建新 `retrieval_profile_id` 并灰度，不重建物理索引。
- 切换后保留旧 generation、旧 embedding 服务并持续追平一段回滚窗口；回滚只切 alias，不拼接新旧索引。

硬约束：

> **不同 embedding profile 的向量不得写入同一向量字段、不得共用查询向量、不得直接融合原始相似度分数。即使输出维度相同，也不视为同一向量空间。**

Elasticsearch 要求 query 使用生成索引向量的同一模型，并明确警告跨索引混用不同 embedding 模型可能无报错却产生错误排序
([kNN query vector builder](https://www.elastic.co/docs/reference/query-languages/query-dsl/query-dsl-knn-query#build-query-vectors-for-knn-search),
[mixed embedding models](https://www.elastic.co/docs/reference/elasticsearch/mapping-reference/semantic-text-setup-configuration#default-endpoint-considerations))。
双写和影子读可以并行运行两个空间，但必须各用自己的 embedder；结果只在评测层比较，不能合并后返回用户。

## 2. 项目基线与差距

### 2.1 当前基线

技术设计规定 ES 只负责召回，业务存储持有发布状态和知识事实；检索前按已发布且适用的 revision 过滤，返回后再核对权威原文和适用性
([技术设计](../design/hardware-operations-agent-technical-design.md))。迁移控制面应保留该边界。

当前稳定索引 `hwops-cce-manual-v3-r1`：

- 641 个 revision、5585 个原文 fragment、42571 个 nested 向量；
- 512 维、`cosine`、`int8_hnsw`；
- 专项保留集 Evidence Recall@5 / 完整证据命中率均为 95%，引用完整性 100%，端到端 p95 15.31 秒；
- 原 75 题开发回归的 Evidence Recall@5 为 94.20%，完整证据命中率为 91.30%。

以上来自 [03f 工单](../../.scratch/hardware-operations-agent/issues/03f-rag-multimodal-assets.md#L42-L58)和
[v3-r1 报告](../../reports/rag-cce-manual-assets-v3-r1-20260925/report.md)，应作为后续迁移的回归基线，不是新语料的永久质量承诺。

已有可复用能力：

- v1/v2/v3-r1 使用独立物理索引和稳定 alias，旧版本未被覆盖
  ([03d](../../.scratch/hardware-operations-agent/issues/03d-rag-semantic-enrichment.md#L19-L26),
  [03f](../../.scratch/hardware-operations-agent/issues/03f-rag-multimodal-assets.md#L24-L30))。
- 入库已有语料 SHA256、enrichment SHA256、逐文档 checkpoint 和数量核对
  ([ingest_cce_manual.py](../../scripts/rag/ingest_cce_manual.py#L76-L105),
  [ingest_cce_manual.py](../../scripts/rag/ingest_cce_manual.py#L163-L194))。
- 语义片段校验连续原文、行范围和内容哈希；QUESTION/PASSAGE/TABLE/IMAGE 等表示作为 nested 向量绑定原文
  ([knowledge.go](../../internal/knowledge/knowledge.go#L178-L268))。
- 查询已记录命中的物理 `_index`，响应已有 `corpus_generation` 和 `index_generations`
  ([retriever.go](../../internal/adapters/elasticsearch/retriever.go#L303-L331),
  [search.go](../../internal/knowledge/search.go#L97-L104))。
- Phoenix 已按 dataset SHA256、split、experiment、run 和 evaluation 建立幂等发布清单
  ([03g 工单](../../.scratch/hardware-operations-agent/issues/03g-phoenix-evaluation-platform.md#L9-L27))。

### 2.2 必须补齐的差距

1. **向量契约硬编码，无启动握手。** `Index.Ensure/embed` 写死 512 维、cosine、`int8_hnsw` 和 BGE 模型名
   ([retriever.go](../../internal/adapters/elasticsearch/retriever.go#L126-L147),
   [retriever.go](../../internal/adapters/elasticsearch/retriever.go#L187-L210))。
   embedding `/healthz` 虽返回模型、revision、维度，但 Go 适配器不校验，响应也缺 tokenizer、pooling、归一化、query prefix、清洗器和模型资产摘要
   ([embedding_server.py](../../scripts/rag/embedding_server.py#L15-L25),
   [embedding_server.py](../../scripts/rag/embedding_server.py#L41-L66))。

2. **来源身份与派生产物身份耦合。** `Revision` 内嵌 fragments，`NewRevision` 随机生成 `DocumentID`、revision ID 和 fragment ID
   ([types.go](../../internal/domain/types.go#L97-L134),
   [knowledge.go](../../internal/knowledge/knowledge.go#L106-L135))。
   换 chunker 后无法仅靠 ID 做跨代对齐，也难区分“内容变了”和“拆分变了”。

3. **manifest 不能证明兼容。** 当前只绑定索引名、corpus/navigation/enrichment hash，未绑定 embedding revision、tokenizer、pooling、normalization、prefix、dims、similarity、mapping/settings
   ([ingest_cce_manual.py](../../scripts/rag/ingest_cce_manual.py#L92-L105))。

4. **切换顺序有风险。** 当前脚本分两次切 fragments/revisions alias，随后才执行公共检索
   ([verify_cce_manual.py](../../scripts/rag/verify_cce_manual.py#L67-L105))；两次请求间可能代次不一致，且验证失败时坏索引已经接流。

5. **只记录、不验证 generation。** 查询收集 `_index`，但未读取 manifest 或断言 query embedding 与索引兼容
   ([search.go](../../internal/knowledge/search.go#L142-L173))。alias 在一次 hybrid/multi-query 请求中切换时，也没有固定物理 generation。

6. **`corpus_generation` 太窄。** 当前只哈希已发布 revision IDs
   ([search.go](../../internal/knowledge/search.go#L93-L104))，不能表达 chunk、embedding、schema 或检索配置变化。

## 3. 官方资料给出的边界

### 3.1 Embedding 契约

Eino 的 `Embedder` 只返回具体实现决定维度的 `[][]float64`；`Retriever` 允许传入 embedder、index、TopK 和 DSL
([Eino Embedding](https://www.cloudwego.io/docs/eino/core_modules/components/embedding_guide/#interface-definition),
[Eino Retriever](https://www.cloudwego.io/docs/eino/core_modules/components/retriever_guide/#component-definition))。
框架不会验证 query vector 与索引来源是否一致，这必须由本项目适配器完成。

固定 revision 的 BGE 官方资料规定 `bge-small-zh-v1.5` 为 512 维、最长 512 token，直接 Transformer 用法取 CLS 并做 L2 normalization；短 query 检索长 passage 时建议只给 query 加中文 instruction，passage 不加
([模型卡](https://huggingface.co/BAAI/bge-small-zh-v1.5/blob/7999e1d3359715c523056ef9478215996d62a620/README.md),
[config](https://huggingface.co/BAAI/bge-small-zh-v1.5/blob/7999e1d3359715c523056ef9478215996d62a620/config.json))。

因此 `embedding_profile_id` 至少覆盖 model ID、不可变 revision、模型/tokenizer 资产摘要、运行库与后端、pooling、normalization、最大 token、截断、query/document prefix、输入清洗器和输出维度。任一项改变都生成新 profile。

### 3.2 Elasticsearch

- kNN query vector 必须与目标字段维度相同
  ([kNN 参数](https://www.elastic.co/docs/reference/query-languages/query-dsl/query-dsl-knn-query#knn-query-top-level-parameters))。
- similarity 决定排名及 `_score` 变换；float `dot_product` 要求 query/document 都是单位向量
  ([dense vector 参数](https://www.elastic.co/docs/reference/elasticsearch/mapping-reference/dense-vector#dense-vector-params))。
- Elasticsearch 9.5.4 源码限制已确定的 `dims` 再变更，并把 `similarity` 声明为不可更新参数
  ([`dims` 源码](https://github.com/elastic/elasticsearch/blob/v9.5.4/server/src/main/java/org/elasticsearch/index/mapper/vectors/DenseVectorFieldMapper.java#L340-L360),
  [`similarity` 源码](https://github.com/elastic/elasticsearch/blob/v9.5.4/server/src/main/java/org/elasticsearch/index/mapper/vectors/DenseVectorFieldMapper.java#L382-L400))。
- `_reindex` 不复制 settings、mapping、shards 或 replicas，目标必须预先创建
  ([Reindex API](https://www.elastic.co/docs/api/doc/elasticsearch/operation/operation-reindex))。
- Elastic 明确建议 embedding 模型变化时写新索引，不在承载查询的索引原地全量更新
  ([kNN 性能指南](https://www.elastic.co/docs/deploy-manage/production-guidance/optimize-performance/approximate-knn-search#_avoid_heavy_indexing_during_searches))。
- alias 多个 remove/add 可放入一次原子操作；`must_exist=true` 可避免 action 部分成功
  ([Aliases](https://www.elastic.co/docs/manage-data/data-store/aliases#multiple-actions))。

结论：维度、相似度、nested 结构、向量算法或 analyzer 变化均新建索引。`_reindex` 只适用于 embedding 文本和向量不变、仅重建兼容 mapping/index options；模型或 chunker 改变时从权威源重新派生。

## 4. 四条独立版本轴

| 版本轴 | 变化例子 | 失效范围 | 默认动作 |
| --- | --- | --- | --- |
| source revision / cursor | 页面新增、修订、删除、适用范围改变 | 受影响 revision/fragment/representation/vector | profile 不变时增量；大批更新可选蓝绿 |
| `chunk_profile_id` | 字节切分改语义边界、overlap、表格/图片策略变化 | 全部 fragment、representation、vector | 新 generation，全量拆分和 embedding |
| `embedding_profile_id` | 模型/revision、tokenizer、pooling、归一化、prefix、清洗规则变化 | 全部 vector 和通常所有阈值 | 新 generation，全量 embedding |
| `vector_schema_id` | 512→768、cosine→dot product、float/int8、HNSW 参数变化 | mapping 和向量索引结构 | 新索引；维度变更重算，相似度单变可复用已归档原向量 |

另记 `retrieval_profile_id`：analyzer、字段权重、候选数、`k/num_candidates`、RRF、query rewrite、evidence selection。应用层参数可在同一索引实验；analyzer/indexed field 变化仍需新索引。

BGE 官方说明绝对相似度阈值应按实际数据校准，Elastic 对各 similarity 的 `_score` 变换也不同，因此模型或 similarity 变化时旧阈值不能沿用
([BGE FAQ](https://huggingface.co/BAAI/bge-small-zh-v1.5/blob/7999e1d3359715c523056ef9478215996d62a620/README.md#frequently-asked-questions),
[Elastic similarity](https://www.elastic.co/docs/reference/elasticsearch/mapping-reference/dense-vector#dense-vector-params))。

内容版本轴不参与 `index_generation_id`：generation 的身份代表“如何派生和索引”，
`bootstrap_snapshot_id` 记录最初回填输入，`applied_source_cursor` 表达当前内容进度。
否则每次内容更新都会改变 generation 身份，与同代增量更新矛盾。

建议规范哈希：

```text
embedding_profile_id = SHA256(canonical_json(embedding_profile))
index_contract_id = SHA256(canonical_json(
  chunk_profile_id, embedding_profile_id, vector_schema, indexed_text_schema))
index_generation_id = SHA256(canonical_json(
  index_contract_id, builder_code_sha256, created_nonce))
serving_profile_id = SHA256(canonical_json(
  index_generation_id, retrieval_profile_id))
```

兼容规则：

- `query.embedding_profile_id == index.embedding_profile_id` 且 query dims == mapping dims；
- 同一线上响应只能有一个已知 `index_generation`；
- BM25 与 dense 只在同 generation 内融合；
- shadow 结果以 source anchor 比较，不比较跨空间原始 `_score`。

## 5. 策略比较

| 策略 | 适用 | 优点 | 风险 | 结论 |
| --- | --- | --- | --- | --- |
| 原地更新 | 少量内容变更，全部 profile 不变 | 容量小、快 | 半更新、回滚难；大量写影响搜索/merge；不支持不兼容 mapping | 仅用于受发布门禁保护的小增量 |
| 停机全量重建 | 小型离线环境 | 简单 | 不可用时间长、失败恢复慢 | 不作为生产默认 |
| 蓝绿 + alias | 模型、schema、chunker、mapping、大批内容变化 | 旧索引持续服务，验证/回滚清晰 | 峰值磁盘/RAM，需增量追平 | **默认方案** |
| 双写 | 回填期间持续更新 | 降低切换滞后 | 同步双写放大组合失败，不同 profile 处理不同 | 用单一变更日志 + 每代异步消费 |
| 影子读 | 新模型/chunker 线上分布验证 | 不影响用户，可成对比较 | 双倍成本，结果不可混合 | 离线门禁后采样开启 |

在同一索引原地新增第二个 vector field 会同时承载多个空间、多个完成度和复杂路由；对当前 nested `representations.embedding` 不推荐。

## 6. 推荐数据模型与 manifest

### 6.1 身份分层

| 标识 | 稳定范围 | 建议生成 |
| --- | --- | --- |
| `source_document_id` | 逻辑页面跨版本稳定 | 规范 URL 或来源系统 ID |
| `source_revision_id` | 一次不可变原文 | 来源版本；否则规范内容和关键元数据哈希 |
| `source_anchor_id` | 独立于 chunker 的最小可引用原文单元 | revision + 规范块/表格行/图片位置 + 内容 hash |
| `fragment_instance_id` | 某 chunk profile 的片段 | revision + chunk profile + 有序 anchor 集合 + 片段 hash |
| `representation_id` | 一段待 embedding 文本 | fragment + kind + ordinal + text hash |
| `vector_id` | 某空间的向量 | representation + embedding profile |

chunker 变化时 fragment ID 可变，但 source revision 和细粒度 source anchor 支持审计
及跨代对齐；片段同时保留 start/end，缺少稳定块标识时按同 revision 的来源范围重叠
比较。内容变化创建新 source revision，不改写历史引用。

### 6.2 Manifest

控制数据分三层：不可变 `generation_spec` 描述处理契约；PostgreSQL
`rag_index_generations` 保存状态、CAS、cursor、checkpoint 和错误；验证完成后生成
不可变 `generation_attestation`，绑定观测计数与 Phoenix 结果。两个 ES mapping 的
`_meta` 只保存 `generation_id`、`generation_spec_sha256` 和 profile IDs。官方允许
`_meta` 存应用元数据并由 Get Mapping 读取，但 ES 不解释其内容
([`_meta`](https://www.elastic.co/docs/reference/elasticsearch/mapping-reference/mapping-meta-field))。

最小 `generation_spec`：

```yaml
schema_version: 1
generation_id: sha256:...
parent_generation_id: sha256:...
physical_indices:
  fragments: hwops-cce-manual-g-...
  revisions: hwops-cce-manual-g-...-revisions
aliases:
  fragments_read: hwops-cce-manual
  revisions_read: hwops-cce-manual-revisions
content:
  corpus_id: cce-user-manual
  bootstrap_snapshot_id: sha256:...
  navigation_sha256: sha256:...
  snapshot_cursor: 1842
chunk_profile:
  id: sha256:...
  parser_version: ...
  normalizer_sha256: sha256:...
  chunker_code_sha256: sha256:...
  strategy: semantic-contiguous-lines
  max_fragment_bytes: 12288
  overlap_tokens: 64
  representation_kinds: [CONTENT, QUESTION, PASSAGE, TABLE_TEXT, TABLE_SUMMARY, IMAGE]
  prompt_sha256: {split: sha256:..., assets: sha256:...}
  generator_models: [{model: ..., revision: ...}]
embedding_profile:
  id: sha256:...
  model: BAAI/bge-small-zh-v1.5
  revision: 7999e1d3359715c523056ef9478215996d62a620
  artifacts_sha256: {model: sha256:..., tokenizer: sha256:...}
  runtime: {library: sentence-transformers, version: ..., backend: torch-cpu}
  pooling: cls
  normalize: l2
  max_tokens: 512
  truncation: enabled
  query_prefix_sha256: sha256:...
  document_prefix_sha256: sha256:...
  input_cleaner_sha256: sha256:...
  dimensions: 512
vector_schema:
  id: sha256:...
  field: representations.embedding
  nested_path: representations
  element_type: float
  dims: 512
  similarity: cosine
  index_options: {type: int8_hnsw, m: 16}
  mapping_sha256: sha256:...
  settings_sha256: sha256:...
build: {code_commit: ..., builder_image_digest: sha256:...}
```

可变状态另存 `state`、`caught_up_cursor`、`last_checkpoint`、重试/死信；最终
attestation 保存 `generation_spec_sha256`、验证 cursor、observed counts、
mapping/settings hash、Phoenix dataset/experiment IDs 和门禁结果。不得把可变 cursor
放入被 `_meta` 引用的不可变 hash。

`retrieval_profile` 另存为不可变 serving 配置，绑定 `index_generation_id`、analyzer、
字段权重、候选数、`k/num_candidates`、RRF、query rewrite 与 evidence selection；
切换查询参数只发布新的 `serving_profile_id`。

模型输出 dims 与 mapping dims 必须相等，但分别保存。mapping/settings 在建索引后通过官方 API 回读并规范化哈希
([Get Mapping](https://www.elastic.co/docs/api/doc/elasticsearch/operation/operation-indices-get-mapping),
[Get Settings](https://www.elastic.co/docs/api/doc/elasticsearch/operation/operation-indices-get-settings))。

### 6.3 Fingerprint 握手

embedding 服务 fingerprint 至少返回 profile ID、model/revision、模型和 tokenizer 摘要、pooling、normalization、max tokens、query/document prefix hash、cleaner hash 和 dims。Go 启动及 generation 装载时必须满足：

```text
generation spec fingerprint
  == embedding service fingerprint
  == ES mapping _meta.embedding_profile_id
```

不一致则 generation 不得 READY；运行中漂移则 dense 路由 fail closed，不能静默退到来源不明的向量服务。

## 7. 迁移状态机

```text
DRAFT -> PROVISIONING -> BACKFILLING -> CATCHING_UP -> VERIFYING
      -> SHADOWING -> READY -> CUTTING_OVER -> ACTIVE
      -> ROLLBACK_WINDOW -> RETIRED

PROVISIONING/BACKFILLING/CATCHING_UP/CUTTING_OVER -> BLOCKED
VERIFYING/SHADOWING -> REJECTED
ACTIVE/ROLLBACK_WINDOW -> ROLLING_BACK -> ACTIVE(old)
```

| 状态 | 工作与出口条件 |
| --- | --- |
| `DRAFT` | 固化 chunk/embedding/vector/indexed-text 契约、容量和评测计划；manifest hash 固定 |
| `PROVISIONING` | 创建两个物理索引；mapping/settings 回读 hash 匹配 |
| `BACKFILLING` | 从权威 snapshot 拆分、embedding、bulk；每 revision 有 checkpoint |
| `CATCHING_UP` | 重放 snapshot 后 upsert/delete/publish/withdraw；cursor 追平 |
| `VERIFYING` | refresh、完整性、映射、检索、Phoenix 和容量检查全过 |
| `SHADOWING` | 稳定采样达到时长/数量，质量与 SLO 过门禁 |
| `READY` | attestation 冻结，增量继续为零 lag |
| `CUTTING_OVER` | 获取锁、追到 barrier、一次请求切两个 alias，回读确认 |
| `ACTIVE/ROLLBACK_WINDOW` | 新索引主读，旧索引继续消费变更 |
| `RETIRED` | 回滚窗结束，有快照后停止旧消费并按保留策略删除 |
| `BLOCKED/REJECTED` | active 不变；保留 checkpoint 和失败实验，修复续跑或新建代次 |

状态转换使用 CAS；外部副作用使用 `generation_id + stage + source_revision_id` 幂等键。

## 8. 回填、追平与查询路由

### 8.1 变更日志

建立事务 outbox / `knowledge_change_log`：

```text
sequence, event_id, document_id, source_revision_id, operation,
content_sha256, publication_state, applicability_hash, occurred_at
```

知识 revision、发布状态和事件同事务提交；每个 generation 有独立 `caught_up_cursor`。这比请求内同步双写稳健：事实只提交一次，各 generation 按自己的 profile 幂等派生。

### 8.2 回填算法

1. 在一致 source snapshot 记录 `snapshot_cursor=W0`。
2. 按 `source_document_id` 扫描权威 revision，不从旧 ES 反向还原原文。
3. 用 candidate chunk profile 生成片段/表示，校验原文范围与 hash。
4. 只调用 candidate embedder，记录 profile ID，校验数量、dims、NaN/Inf、零向量和抽样 norm。
5. 用确定性 ID Bulk 写入；逐 item 检查，429 随机指数退避并逐文档 checkpoint。Bulk 支持批量 index/create/delete/update 和版本控制；官方建议实测 batch、监控 429
   ([Bulk API](https://www.elastic.co/docs/api/doc/elasticsearch/operation/operation-bulk),
   [indexing speed](https://www.elastic.co/docs/deploy-manage/production-guidance/optimize-performance/indexing-speed))。
6. 从 `W0+1` 重放事件；更新文档时删除该 generation 的旧 revision fragments，再 upsert 新 revision。
7. 追到 head 后 refresh；refresh 会等待变更对搜索可见
   ([Refresh API](https://www.elastic.co/docs/api/doc/elasticsearch/operation/operation-indices-refresh))。

每个 ES 文档携带 `source_event_sequence`，只允许更高序号覆盖更低序号。Bulk/Reindex 支持 external version；`external_gte` 使用不当可能丢数据，默认选严格 `external` 或业务 CAS
([Reindex API](https://www.elastic.co/docs/api/doc/elasticsearch/operation/operation-reindex))。
删除也必须有 tombstone/checkpoint。chunker 改变后按 `(generation, source_document, prior_revision)` 找旧片段，不能借用另一代 fragment IDs。

### 8.3 请求级 generation 绑定

在 Eino 自定义 Retriever 外增加 `GenerationRouter`：

```text
一次解析 ACTIVE generation
-> 校验 manifest/mapping/embedder fingerprint
-> 生成不可变 request-scoped GenerationHandle
-> BM25/dense/multi-query/evidence selection 全部使用其物理 index
-> 断言返回 _index 等于绑定 index
```

Eino `schema.Document` 支持下游 metadata
([Retriever Document](https://www.cloudwego.io/docs/eino/core_modules/components/retriever_guide/#document-struct))；
继续把 generation/profile IDs 和物理 index 传播到 `KnowledgeToolCall`、Phoenix span 和响应审计。

不要让并行 route 各自解析 alias。可用 Resolve Index API 在请求开始时固定物理目标
([Resolve Index](https://www.elastic.co/docs/api/doc/elasticsearch/operation/operation-indices-resolve-index))，更直接的是从 ACTIVE manifest 取得物理 index。一次响应若发现 0 或多个 generation，fail closed 并对单一 generation 重试一次。

### 8.4 影子读

- 按 query fingerprint 稳定采样；主路和影子路分别 embedding、检索、融合。
- 影子结果不进入答案；以 source revision/anchor 对齐，不用跨 chunker fragment ID。
- 比较 Evidence Recall@K、完整证据、文档命中、无答案分歧、错误/超时、各阶段延迟和资源。
- 跨 generation 不比较 raw score；只比较排名、标签指标和最终证据。

## 9. 切换、验证与回滚

### 9.1 切换 barrier

1. 获取 migration lock，读取 `source_head_cursor=Wc`。
2. candidate 和旧索引都处理到 `Wc`。
3. 短暂停 publication，或让新事件排在 `Wc` 后。
4. 最终 refresh 和校验。
5. 一次 `_aliases` 请求同时 remove/add fragments 与 revisions alias，并设置 `must_exist=true`。
6. 回读 alias，提交 manifest `ACTIVE`，恢复 publication。

Alias API 支持多 action 原子执行；`must_exist=true` 用于让任一 action 失败时整组失败
([Aliases](https://www.elastic.co/docs/manage-data/data-store/aliases#multiple-actions))。

### 9.2 硬门禁

- 两个物理索引 `_meta.generation_spec_sha256` 一致；mapping/settings、embedding 握手和四轴 profile 全匹配。
- document/revision/fragment/vector/tombstone 计数精确一致；source anchor 与内容 hash 100% 可核验。
- `caught_up_cursor == source_head_cursor`，稳定窗口内 lag=0，无死信或处理中任务。
- shard health 达部署要求。官方定义 green 为全部 primary/replica 已分配，yellow 为 primary 已分配但 replica 未全分配
  ([Cluster Health](https://www.elastic.co/docs/api/doc/elasticsearch/operation/operation-cluster-health-1))。
- 公开 HTTP + 真实临时 store 的确定性契约全过并标 REPLAY；真实 BGE/ES 与真实模型结果单报。
- Phoenix 新 experiment 绑定 dataset/manifest hash、generation/profile IDs 和 trace ID，不覆盖历史。Phoenix 支持持久 baseline 和逐样例对比
  ([Phoenix experiments](https://arize.com/docs/phoenix/datasets-and-experiments/how-to-experiments/run-experiments#set-a-baseline-experiment))。

建议把当前 v3 指标作为回归下限：专项 20 题 Recall@5/完整证据不低于 95%、引用完整性 100%、错误 0；原 75 题 Recall@5 不低于 94.20%、完整证据不低于 91.30%。模型/chunker 若已用这些集合调优，另建未使用 heldout；安全关键、版本适用性和拒答题逐题不得退化。

当前 20/20 的 Wilson 95% 区间仍为 83.89%～100%
([v3-r1 报告](../../reports/rag-cce-manual-assets-v3-r1-20260925/report.md))，所以切换还应看逐题配对、置信区间、影子样本和 SLO，不能只看平均值。

强制指标：`mixed_generation_total=0`、`embedding_fingerprint_mismatch_total=0`、`unmapped_source_anchor_total=0`。延迟分别报告纯检索和端到端，不把当前 15.31 秒的小样本值当生产 SLO。

### 9.3 回滚

回滚窗口保留旧索引、旧 embedder/model 资产、旧 generation processor、切换前 snapshot 和权威源 checkpoint。Elasticsearch snapshot 可不停机备份，但只是各 shard 在开始至结束区间内某时刻的视图，不是跨 shard 单一瞬时快照
([Snapshot and restore](https://www.elastic.co/docs/deploy-manage/tools/snapshot-and-restore#how-snapshots-work))。

以下触发停止扩量并回滚评估：fingerprint/mapping/manifest 不一致；请求混代；cursor 超阈值或死信；安全关键、适用性、引用或拒答回归；错误率/p95/资源超门槛；fragment 无法映射权威原文。

回滚前确认旧 processor 追到当前 barrier；否则暂停 publication 并先追平。随后一次 alias 请求把两个 read alias 一起切回并回读。不要用 snapshot restore 替代快速 alias 回滚。

## 10. 验证指标

**构建完整性**

- generation spec/attestation、mapping/settings/code/prompt/model/tokenizer hash；
- source revision 集合差异，fragment/representation 数量及 kind 分布；
- anchor 覆盖/重复/空片段/截断率；
- embedding 数量、dims、NaN/Inf、零向量、norm；
- ES `_count`、`_stats/docs,dense_vector,store,segments`；
- cursor、重复事件、tombstone、死信。

Index Stats 提供 documents、deleted docs、store、segments 和 dense vector 数量
([Index Stats](https://www.elastic.co/docs/api/doc/elasticsearch/operation/operation-indices-stats))；运行计数与 manifest 交叉核对，不能互相替代。

**离线与在线质量**

- Evidence Recall@1/3/5、all-evidence-at-5、document hit、MRR@5、nDCG@5；
- BM25/dense/hybrid 分路，表格/图片/命令/错误码/版本/无答案分层；
- 引用 hash/anchor、整题准确率、忠实度、拒答准确率；
- embedding/search/RRF/selection/端到端 p50/p95/p99、usage、错误；
- 影子 source-anchor overlap、新增相关证据率、丢失基线证据率、分层分歧。

Phoenix experiment 由冻结 Dataset、逐样例 task 和 evaluators 组成，能把公开 HTTP 输出、expected、input 和 metadata 一起评测
([Phoenix SDK workflow](https://arize.com/docs/phoenix/datasets-and-experiments/how-to-experiments/run-experiments#run-experiments-with-the-sdk))。metadata 至少记录 generation/profile IDs、物理 index、data mode 和 source cursor。

## 11. 容量估算

Elastic 对 `int8_hnsw` 给出：

```text
int8 vector RAM ~= N * (dims + 4)
HNSW graph RAM ~= N * 4 * m
raw float vector disk ~= N * dims * 4
int8 quantized disk overhead ~= raw float disk * 25%
```

公式及默认 `m=16` 来自 [Elastic kNN 内存指南](https://www.elastic.co/docs/deploy-manage/production-guidance/optimize-performance/approximate-knn-search#_ensure_data_nodes_have_enough_memory)；int8 仍保留原始 float，向量磁盘约增加 25%
([quantization](https://www.elastic.co/docs/reference/elasticsearch/mapping-reference/dense-vector#dense-vector-quantization))。

按当前 `N=42571`、`dims=512`、`m=16`、零副本：

| 项目 | 估算 |
| --- | ---: |
| 原始 float 向量 | 83.15 MiB |
| int8 搜索向量 RAM | 20.95 MiB |
| HNSW graph RAM | 2.60 MiB |
| 单代向量 working-set 下界 | 23.55 MiB |
| 单代原始 + int8 向量磁盘下界 | 103.93 MiB |
| 同规模蓝绿两代向量磁盘下界 | 207.86 MiB |
| 两代同时影子查询的向量 working-set 下界 | 47.10 MiB |

不含 nested Lucene docs、文本、`_source`、倒排、doc values、segments、translog、merge、revision index 和副本；副本磁盘乘 `(1+replicas)`。candidate 按 `N_new/42571 * dims_new/512` 缩放。

实际预算方法：

1. 先用 5%～10% 分层文档跑完整 candidate，外推 fragments/document、vectors/fragment、tokens 和字节的 p50/p95。
2. 对 active/sample 调 `_stats/store,dense_vector,docs,segments`；`_disk_usage` 资源密集且小索引可能不准，只在隔离时段单索引运行
   ([Disk Usage](https://www.elastic.co/docs/api/doc/elasticsearch/operation/operation-indices-disk-usage))。
3. 峰值磁盘至少为 `active + candidate + candidate 的 translog/merge 余量 + snapshot/recovery reserve`。建议 candidate 额外留 30%，同时服从更严格的集群水位。
4. 不默认在切换前 force merge；`max_num_segments=1` 时单 shard 临时空间最坏可达其三倍
   ([Force Merge](https://www.elastic.co/docs/api/doc/elasticsearch/operation/operation-indices-forcemerge))。
5. 当前完整重算是 42571 个文本向量；按真实 tokenizer、batch 和目标机器 `tokens/s` 实测，不按 HTTP 请求数估时。

## 12. 失败处理

| 失败 | 处理 | 禁止 |
| --- | --- | --- |
| fingerprint 不符 | `BLOCKED`，停止 backfill/查询 | 只验 dims 后继续 |
| 单文档处理失败 | 有界重试；死信阻止 READY | 跳过后宣称完整 |
| Bulk 部分失败/429 | 逐 item 重试，429 指数退避 | 只看 HTTP 200 |
| worker 崩溃 | 从文档 checkpoint + event cursor 恢复 | 把删索引重来当唯一方案 |
| 回填时内容更新 | 从 `W0+1` 事件追平 | 无边界重抓“最新全集” |
| mapping/settings 不符 | 未接流 candidate 重建或新 generation | 改 mapping 后沿用旧 hash |
| 验证/影子失败 | `REJECTED`，active 不变，保留 Phoenix 失败实验 | 覆盖报告/复用实验名 |
| alias 失败 | `must_exist=true` 整组失败并回读 | 分开切两个 alias |
| 切后回归 | 旧代追平后原子切回 | 混合两代结果 |
| 旧代未追平 | 暂停发布，补事件后再回滚 | 回滚到已知陈旧内容 |

异步 reindex task 仍可能因节点关闭消失或失败，失败后可能要清理 partial destination 再试
([Reindex API](https://www.elastic.co/docs/api/doc/elasticsearch/operation/operation-reindex))；ES task 不是迁移状态机/checkpoint 的权威来源。

## 13. ADR 与工单建议

ADR 先冻结：

1. generation 和四轴版本定义；
2. 不同 embedding profile 绝不混用、请求绑定单代；
3. 大变更采用蓝绿 + 事件追平 + alias；
4. PostgreSQL manifest/outbox 为控制事实，ES `_meta` 为核对副本；
5. 切换/回滚门禁和旧代保留窗口；
6. Phoenix 基线、回归、新 heldout 和 SLO；
7. 容量水位、并发、重试与人工审批边界。

后续工单按完整能力拆分：

1. Manifest schema、规范哈希、embedding fingerprint 与启动 fail-closed。
2. 稳定 source/fragment/representation/vector 身份，outbox、cursor、tombstone。
3. Generation builder：建索引、bulk 回填、checkpoint、追平、计数、容量报告。
4. Generation router：请求级绑定、两个 alias 单请求切换、混代拒绝、回滚。
5. Shadow/Phoenix：source-anchor 对齐、冻结 experiment、质量/SLO 自动门禁。
6. 运维闭环：回滚窗双消费、snapshot、retire/delete、死信恢复和演练。

每个工单继续按仓库约定通过 Go 公开 HTTP 流程和真实临时 store 验证；模拟外部依赖的确定性结果标 REPLAY，真实 BGE/ES 和真实模型结果分别报告。
