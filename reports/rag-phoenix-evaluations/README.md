# Phoenix RAG 评测迁移

2026-09-26 将全部历史 RAG 评测统一纳入本地 Phoenix
`http://127.0.0.1:16006`。

## 结果

| 对象 | 回读数量 |
| --- | ---: |
| Dataset | 10 |
| Dataset example | 357 |
| Experiment | 33 |
| Run | 1633 |
| Evaluation | 9987 |
| Evaluator definition | 1 |
| Dataset evaluator binding | 10 |

其中 03a 的 2 个 dataset、6 个 experiment 和 182 个 run 原本已由旧评测脚本
直接写入 Phoenix；其余 `evaluate_manual.py` 历史结果已按冻结数据集 SHA256 和
split 去重迁移。迁移前有 24 个本地完整实验尚未发布。

迁移器只接收状态为 `complete`、行数完整、文件名与实验名一致、逐题 ID 与冻结
split 完全一致的实验。入库清单、bad case、探针、selection、cleanup 和
verification JSON 均未导入。

## 产物

- `inventory.json`：本地实验盘点和发布状态。
- `manifest.json`：本地报告与全部 Phoenix ID 的幂等绑定。
- `verification.json`：通过 Phoenix API 回读的逐实验 run/evaluation 数量。
- `evaluators.json`：Evaluator定义、源码/输出配置哈希和Dataset绑定ID。

重复运行迁移器后数量仍为 10/33/1633/9987，没有新增重复记录。

## Evaluator

2026-10-01 注册`hwops-rag-runtime-contract-v1` Code Evaluator，并绑定全部10个
dataset。它在Phoenix Monty Python sandbox中检查：

- 回答结果是否明确标记为`LIVE`。
- 处理是否到达非失败终态。
- `ANSWERED`或`PARTIAL`回答是否至少包含一条引用。

纯检索run不具备答案状态，三项输出均为`not_applicable`。该确定性Evaluator不
替代外部LLM语义评审或证据复核。重复注册后仍为1个定义、10个绑定。
