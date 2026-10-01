# 03a: Elasticsearch + Phoenix 真实文档 RAG 评测

**Task:** RAG-ES-PHOENIX

**Blocked by:** 02 (QA-02，已完成)

**Status:** done

**Spec:** 用户本轮明确要求使用 Elasticsearch、本地轻量中文 embedding、DeepSeek `deepseek-flash` 和 Phoenix；该选择覆盖技术设计中 OpenSearch 的默认建议。

## 交付行为

将华为云 CCE《高危操作一览》导入 Elasticsearch，接入现有 Go + Eino 检索及答案流程；在本地部署 Phoenix，保存追踪、评测数据集与实验。基于 bad case 迭代优化，固定验收集 Recall@5 达到至少 60%，输出可复核统计报表并释放本次运行资源。

源文档：https://support.huaweicloud.com/usermanual-cce/cce_10_0054.html

## 验收约定

- [x] 实际运行 Elasticsearch 文档/向量库、本地中文 embedding、Phoenix，并接入 Go 项目。
- [x] 保留原始表格对象、操作、后果、恢复方案及来源；检索片段可映射到稳定的原文行标识。
- [x] 在检索实验前冻结问题、参考答案、相关证据标识、开发/验收划分及数据集哈希；优化只看开发集 bad case。
- [x] 固定主指标为验收集宏平均 Recall@5，至少 60%；同时报告 Recall@1/3、Precision@5、HitRate@5、MRR@5、nDCG@5 和时延。
- [x] 记录答案正确性、基于证据的忠实度、相关性、引用命中和无答案问题的拒答表现；LLM 评分单独标为模型评审结果。
- [x] Phoenix 中可读取数据集、实验结果和 Go 检索/模型/答案追踪；报告保留数据与必要导出。
- [x] 与现有型号版本、知识发布、引用检查和 HTTP 验收兼容。
- [x] 结束后停止本次启动的进程，释放 embedding、Elasticsearch 和 Phoenix 运行资源，保留可重现脚本和评测产物。

使用现有公开 HTTP 流程、外部适配器以及 Phoenix 公开接口验收，沿用技术设计 10.1 的授权测试接口。本任务只推进这次文档 RAG 实验及所需接入；不据此声明 QA-03 全部冲突纠偏、QA-04 实时监控或 QA-06 综合验收已完成。

## 运行记录

- 本机为 macOS arm64，48 GiB 内存；无 Docker 和 Java，采用工作区内隔离安装及本地进程，限制 ES 堆内存及 embedding CPU 线程。
- 源文档已通过正常浏览器读取，包含 8 张表、55 个数据行。命令行下载遇到安全验证页，不用该响应作为知识源，也不绕过验证。
- 密钥通过私有运行配置加载到进程环境，不提交代码、不写入报告或追踪；任务结束清理临时凭据。

## 完成证据（2026-09-20）

- 真实运行ES 9.5.4、BGE small zh v1.5固定revision和Phoenix 20.14.0；Go + Eino公开HTTP检索与问答均完成接入。55条原文证据及前言共56片段，原文也存入ES修订索引。
- 冻结66题，开发25题（23可回答）、验收41题（38可回答）；SHA256为`c08c4b67938d20506a1e58aa278778ab65e907871ac95cb10bab41981ee2cc10`。开发BM25基线Recall@5为97.83%，Dense优化为100%，RRF仍为97.83%；验收前按主指标冻结Dense，未使用验收题调参。
- 独立验收Recall@5为**93.42%≥60%**，Recall@1为86.84%、MRR@5为92.98%、nDCG@5为92.40%。检索p95为12.14ms，真实问答p95为1153.41ms。
- DeepSeek `deepseek-flash`真实生成与评审各66次。验收LLM正确性/忠实度/相关性均分为91.46%/98.65%/93.90%；不是人工认证，忠实度排除空答案。保留3道召回不足题、题目证据缺口与评审误判，不改写冻结分数。
- Phoenix接口回读通过：2个数据集、6个实验、182个run、607个span，248个实验/评审trace引用齐全。导出包含逐题证据、答案、真实usage、评分解释与trace_id。
- 全量race、实际ES/embedding公开流程、QA-01/QA-02回归、vet、构建通过；真实PostgreSQL因无连接配置跳过。确定性生成测试明确标REPLAY，与真实DeepSeek评测分别记录。
- 已停止Go、ES、Phoenix、embedding及全部子进程，核验19200、19300、16006、18765、18080端口全部释放；临时DeepSeek密钥文件已删除。保留数据库、缓存与报表，停止前进程树RSS合计1946.0MiB。

[主统计报表](../../../reports/rag-cce-20260920/report.md) · [优化记录](../../../reports/rag-cce-20260920/optimization.md) · [接口核验](../../../reports/rag-cce-20260920/verification.json) · [工程验证](../../../reports/rag-cce-20260920/validation/README.md) · [清理记录](../../../reports/rag-cce-20260920/cleanup.json) · [复现步骤](../../../scripts/rag/README.md)
