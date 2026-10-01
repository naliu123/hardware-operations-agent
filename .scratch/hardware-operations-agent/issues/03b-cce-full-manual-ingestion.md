# 03b: CCE用户指南全目录入库

**Task:** CCE-FULL-MANUAL-INGESTION

**Blocked by:** 03a（已完成）

**Status:** done

## 交付行为

以用户指定页面左侧“用户指南”目录为边界，枚举全部目录节点，采集其中所有实际文档并发布到新的Elasticsearch索引版本。保留文档标题、目录层级、来源URL、更新时间、正文结构、表格、代码和链接；支持失败续跑及内容哈希核验。

目录入口：https://support.huaweicloud.com/usermanual-cce/cce_10_0054.html

## 范围约定

- “所有文档”指左侧“用户指南”树中具有实际页面链接的叶子文档。
- 目录树中仅用于展开、正文只列子链接的分类节点记录在导航清单中，但不作为重复知识发布。
- 不递归正文交叉链接，不纳入FAQ、最佳实践、公告、API参考等其他手册。
- 使用`intl/zh-cn`同语言页面完成稳定采集，入库来源统一为用户给出的非`intl`规范URL。
- 全量语料使用新索引`hwops-cce-manual-v1`，核验完成后切换稳定别名`hwops-cce-manual`；保留03a单页索引和评测产物。

## 验收条件

- [x] 保存完整导航清单、目录哈希、文档清单和每页内容哈希；导航URL无重复。
- [x] 所有叶子文档成功采集，无安全验证页或空正文混入。
- [x] 长章节被限制为可进入48 KiB上下文预算的片段，短文档分段行为不回归。
- [x] 所有采集文档通过Go公开知识接口创建并发布，ES文档数、修订数和片段数与清单一致。
- [x] BM25与Dense分别通过跨目录抽样检索，结果返回预期来源。
- [x] 原QA-01、QA-02和03a相关测试、race、vet及构建通过。
- [x] 更新复现说明和完成证据；停止本次进程并核对监听端口释放。

## 运行记录

- 2026-09-20通过当前目录页DOM识别757个唯一导航条目，其中116个分类节点、641个实际文档。
- 站点对命令行及无头浏览器返回EdgeOne安全验证页；使用本地隔离的有界浏览器会话取得当前目录和同语言正文，不绕过或自动操作验证码。采集后只保留规范化Markdown、哈希及来源元数据。

## 完成证据（2026-09-20）

- 757个导航节点的URL和文档ID均无重复；116个分类节点只进入导航清单，641篇实际文档全部采集，失败数为0。规范化正文共6149442 UTF-8字节，整库SHA256为`527101b5c146ec9b3961f4b4252b2ab009bc7b9209b16abc6f8e9d3ce0d41559`。
- 全部文档通过Go公开知识接口发布到`hwops-cce-manual-v1`，状态均为`PUBLISHED`。ES回读为641个不可变修订和3539个片段，片段使用真实本地BGE向量；批量模型模式明确标记为REPLAY。
- `hwops-cce-manual`和`hwops-cce-manual-revisions`稳定别名切换完成。5组跨目录问题分别使用BM25和Dense检索，10次均在Top 5命中预期规范来源。
- Python采集/规范化/UTF-8拆分测试3项通过；全量`go test -race ./...`、QA-01公共HTTP引用与重启恢复、外部OpenAI/DeepSeek协议契约、真实ES/BGE公开流程、`go vet ./...`及`go build ./...`均通过。
- 已停止Go、ES、Phoenix、embedding及子进程，核验19200、19300、16006、18765、18080端口全部释放；停止前进程树RSS合计2336.8 MiB。语料、ES数据库、模型缓存和浏览器保留用于复核。

[全目录入库报告](../../../reports/rag-cce-manual-20260920/report.md) · [入库checkpoint](../../../reports/rag-cce-manual-20260920/ingestion.json) · [核验结果](../../../reports/rag-cce-manual-20260920/verification.json) · [清理记录](../../../reports/rag-cce-manual-20260920/cleanup.json) · [复现步骤](../../../scripts/rag/README.md)
