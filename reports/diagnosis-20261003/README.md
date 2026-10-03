# DX-01 持续诊断验收（2026-10-03）

完成单实例只读 Plan–Execute–Replan：模型提出下一步，程序校验并提交计划，执行实际
知识检索或监控查询，再根据结果重规划。计划版本、执行、证据、判断、预算与事件持久化；
支持等待、暂停、显式恢复、取消与进程重启。跨worker恢复和并行调度属于DX-02。

## REPLAY 与回归

业务操作全部经过公开HTTP，使用真实临时文件存储和外部HTTP模型/监控桩。
没有替换数据库或直接写入诊断运行以绕过业务校验。

| 验收场景 | 结果 |
| --- | --- |
| 500 rpm 后查电压，2000 rpm 后查温度；实际结果改变下一版计划 | PASS |
| 低转速+正常供电确认根因，正常转速+低温满足恢复规则；运行完成单独记录 | PASS |
| 环、缺依赖、错目标、CLI、伪造确认/恢复/引用、旧版本、可执行时错误WAIT | PASS，提交和执行前拒绝 |
| 缺观测、错误规则/结论、过期证据、单位错误 | PASS，不能确认 |
| 更新的反证、同一采样时间的矛盾观测被模型省略 | PASS，根因与恢复均拒绝确认 |
| 有效观测复用、依赖与前置条件、失败保持未知 | PASS |
| WAIT释放worker、活跃运行复用、恢复版本冲突、等待与活跃重启 | PASS |
| 规划中取消、观测中取消后归档原执行、设备快照变化丢弃提案 | PASS |
| 活跃时间/工具/模型预算、嵌套改写和选择、HTTP重试计数 | PASS，失败和恢复不清零 |

全量 `go test -race ./... -count=1`：88项顶层测试通过，其中14项为诊断测试，
另有6项外部集成测试默认SKIP。完整JSON事件见 [regression.jsonl](regression.jsonl)。
`go vet ./...`、`go build ./...`、`git diff --check` 通过。

验收中修正了文件存储的事务快照别名问题：发布的内存快照必须与调用方切片独立，
防止后续计划/执行追加绕过事务并产生竞态。修复后诊断与全量race均通过。

## LIVE 模型协议

实际调用 OpenCode Go `deepseek-v4.1-flash`，通过公开HTTP发布合成手册和设备，
使用隔离临时文件库。监控未配置，返回真实的能力缺口 `UNSUPPORTED`，
没有将模拟设备读数标为LIVE。

| 轮次 | 模型结果 | 业务行为 |
| --- | --- | --- |
| R0 | 将失败观测ID填入候选原因证据 | `INVALID_PLAN` 拒绝；未确认根因/恢复 |
| R1 | 提示词明确失败ID只用于决策依据/缺口后复测 | 3版计划、3次模型、2次工具；主动 `PAUSED`，根因/恢复均 `UNKNOWN` |

保留 [R0失败记录](live-model-r0.txt) 和 [R1通过记录](live-model.txt)。
该烟测证明外部模型能遵循诊断协议并处理观测缺口，不衡量生产诊断语义质量。

## 复现与未验收项

```sh
./scripts/go.sh test -race ./test/acceptance -run '^TestDiagnostic' -count=1 -v
./scripts/go.sh test -race ./... -count=1
./scripts/go.sh vet ./...
./scripts/go.sh build ./...
# 已配置LIVE模型环境变量时：
HWOPS_TEST_LIVE_MODEL=1 ./scripts/go.sh test ./test/acceptance -run '^TestLiveDiagnosticPlanningAndMissingObservation$' -count=1 -v
# 已配置专用 HWOPS_TEST_DATABASE_URL 时：
./scripts/go.sh test ./test/acceptance -run '^TestPostgreSQLDiagnostic' -count=1 -v
```

PostgreSQL实现及004迁移已提供，专用验收覆盖WAIT、恢复、CAS和历史重启读取；
本次未配置数据库，明确SKIP。ES专项集成也未在本轮运行。生产资产与监控、
专家判据质量、模型诊断准确率和容量均未验收。

当前采用单实例、单诊断worker，历史保存在运行JSONB聚合中；从业务快照重启Eino图，
尚未实现跨worker CheckPointStore、租约、outbox与并行分支。模型的自然语言理由
和摘要仍需语义评估；结构校验保证引用、判据、状态及读数约束，不能替代专家审核。
