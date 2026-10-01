# 03g: Phoenix 统一评测平台

**Task:** PHOENIX-EVALUATION-PLATFORM

**Blocked by:** 03f（已完成）

**Status:** done

## 交付行为

1. 盘点并迁移全部历史 RAG 评测数据集、实验、逐题 run 和 evaluation。
2. 使用全局发布清单绑定冻结数据集 SHA256、本地报告、Phoenix ID 和逐题结果，
   防止重复创建或把非实验 JSON 误导入。
3. 后续 `evaluate_manual.py run/judge/audit` 默认同步 Phoenix；Phoenix 不可用时
   明确失败，只有显式 `--no-phoenix` 才允许保留为本地离线结果。
4. 迁移后通过 Phoenix API 回读每个 dataset、experiment、run 和 evaluation 数量。
5. 将可在 Phoenix sandbox 中独立执行的确定性运行契约注册为 Code Evaluator，
   并绑定全部历史及后续 RAG dataset；外部 LLM 语义评审不伪装为 Phoenix 内执行。

## 验收条件

- [x] 历史 03a 原生实验和所有完整 `evaluate_manual.py` 实验均可在 Phoenix 查看。
- [x] 数据集按冻结 SHA256 与 split 去重，实验绑定准确的本地配置哈希。
- [x] 重复迁移不新增 dataset、experiment、run 或 evaluation。
- [x] 新评测默认 fail-closed 同步 Phoenix，离线绕过必须显式声明。
- [x] 单元测试、Go race、vet 和 build 全部通过。
- [x] Phoenix Evaluators 页面可查看并执行已绑定的 RAG 运行契约评分器。

## 完成证据

- 严格盘点得到6个03a原生实验和27个`evaluate_manual.py`完整实验；入库清单、
  bad case、探针、selection、cleanup和verification JSON均未导入。
- Phoenix API回读10个dataset、357个example、33个experiment、1633个run和
  9987个evaluation；SQLite持久库总数一致。
- 第二次运行全量迁移器及单独重发最终保留实验后，上述数量保持不变。
- `evaluate_manual.py run/judge/audit`默认在操作前检查Phoenix，并在本地阶段
  完成后同步；`--no-phoenix`是唯一离线绕过入口且会输出显式标记。
- Python 26项测试、`go test -race ./...`、`go vet ./...`、`go build ./...`
  及`git diff --check`通过。
- 证据见[迁移报告](../../../reports/rag-phoenix-evaluations/README.md)、
  [盘点](../../../reports/rag-phoenix-evaluations/inventory.json)、
  [全局清单](../../../reports/rag-phoenix-evaluations/manifest.json)和
  [API回读](../../../reports/rag-phoenix-evaluations/verification.json)。
- 2026-10-01注册`hwops-rag-runtime-contract-v1` Code Evaluator；Phoenix
  preview对正常回答、失败回答和纯检索三组输入共9项输出全部符合预期。
  Evaluators页面回读1个定义，展开可见全部10个dataset绑定；重复注册后仍为
  1个定义、10个绑定。定义检查`LIVE`模式、处理终态和回答引用存在性，不替代
  外部LLM语义评审。注册证据见
  [Evaluator清单](../../../reports/rag-phoenix-evaluations/evaluators.json)。
