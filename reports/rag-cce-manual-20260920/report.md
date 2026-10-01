# CCE 用户指南全目录入库报告

日期：2026-09-20

## 结论

指定页面左侧“用户指南”目录已完整采集并发布。目录、正文文件、入库 checkpoint、ES 计数、稳定别名及公共检索抽样均核验通过。

| 项目 | 结果 |
| --- | ---: |
| 导航节点 | 757 |
| 仅展开的分类节点 | 116 |
| 实际文档 | 641 |
| 成功采集 / 预期采集 | 641 / 641 |
| 采集失败 | 0 |
| 规范化正文 | 6,149,442 UTF-8 字节 |
| 已发布修订 | 641 |
| ES 检索片段 | 3,539 |
| BM25 跨目录抽样 | 5 / 5 |
| Dense 跨目录抽样 | 5 / 5 |

整库 SHA256：

```text
527101b5c146ec9b3961f4b4252b2ab009bc7b9209b16abc6f8e9d3ce0d41559
```

导航 SHA256：

```text
e265f860bcd4b74663f57feae0a79452b29b2feede3c32460aa229bc7edcb489
```

## 范围与数据

- 范围只包含左侧“用户指南”树中具有实际页面链接的文档。
- 116 个纯分类节点保留在导航清单中，不重复发布其子链接列表。
- 不递归正文交叉链接，不纳入 FAQ、最佳实践和 API 参考等其他手册。
- 每篇 Markdown 保留标题、目录路径、规范来源 URL、更新时间、表格、代码、图片和链接。
- 抓取遇到安全验证时只使用正常可见浏览器会话，不自动操作或绕过验证；验证页和空正文会被拒绝。

清单与正文位于 `testdata/rag/cce-manual/`：

- `navigation.json`：完整目录、节点类型和目录哈希。
- `manifest.json`：文档元数据、逐页内容哈希和整库哈希。
- `capture-checkpoint.json`：逐页采集状态。
- `documents/`：641 篇规范化 Markdown。

## 入库与检索

发布通过 Go 公共知识接口完成，业务状态使用 `.local/rag/hwops-cce-manual-state.json`。批量流程运行在 `REPLAY` 模式，不调用 DeepSeek；所有向量由本地 `BAAI/bge-small-zh-v1.5` 实际推理生成。

| 用途 | 名称 |
| --- | --- |
| 片段索引 | `hwops-cce-manual-v1` |
| 修订索引 | `hwops-cce-manual-v1-revisions` |
| 片段稳定别名 | `hwops-cce-manual` |
| 修订稳定别名 | `hwops-cce-manual-revisions` |

`ingestion.json` 记录 641 篇文档均为 `PUBLISHED`，ES 实际计数为 641 个修订和 3539 个片段。`verification.json` 保存 5 组跨目录问题的 BM25、Dense 返回来源和 trace ID，10 次检索全部命中预期文档。

## 工程验证

- `python -m unittest scripts/rag/test_cce_manual.py`：3 项通过。
- `python -m py_compile`：采集、入库、核验和服务脚本通过。
- `go test -race ./...`：全量通过。
- QA-01 公共 HTTP 引用与重启恢复：通过。
- 外部 OpenAI/DeepSeek 协议和 usage 契约：通过。
- 真实 ES/BGE `TestElasticsearchPublicFlow`：通过。
- `go vet ./...`：通过。
- `go build ./...`：通过。

PostgreSQL、资产系统和生产硬件未在本任务中联调。此次结果证明全目录采集、发布和检索链路，不是新的 DeepSeek 答案质量评测；真实模型质量仍以 03a 冻结评测报告为准。

## 资源释放

停止前本次进程树 RSS 合计 2336.8 MiB。Go、Elasticsearch、Phoenix 和 embedding 均已停止，`19200`、`19300`、`16006`、`18765`、`18080` 无监听；临时 DeepSeek 密钥不存在。语料、ES 数据、模型缓存和 Playwright 浏览器保留用于复核与续跑，详见 `cleanup.json`。

复现步骤见 [`scripts/rag/README.md`](../../scripts/rag/README.md)，详细机器可读结果见 [`ingestion.json`](ingestion.json)、[`verification.json`](verification.json) 和 [`cleanup.json`](cleanup.json)。
