# QA-06 综合问答基线

日期：2026-10-03T08:16:57Z；数据模式：**REPLAY**。

模型配置：fixture-extractive-v1，经真实OpenAI HTTP适配器；本地词项检索；外部监控桩。真实模型效果：本轮未评估；既有DeepSeek协议烟测与文档质量见03h/03f报告。

公共HTTP入口、外部HTTP模型/监控桩与真实临时文件库；数值仅代表合成样本的确定性结构和规则验证。引用支持性检查为原文摘录与引用完整性，不是自然语言语义评审。生产设备、监控和PostgreSQL未联调。

| 样本 | 预期 / 实际状态 | 检查 | 耗时 ms |
| --- | --- | --- | ---: |
| atlas-r2-led | ANSWERED / ANSWERED | true | 30.71 |
| atlas-r10-error | ANSWERED / ANSWERED | true | 30.02 |
| boreal-b9-led | ANSWERED / ANSWERED | true | 35.30 |
| boreal-b10-led | ANSWERED / ANSWERED | true | 33.88 |
| follow-up-pronoun | ANSWERED / ANSWERED | true | 37.02 |
| switch-device-config | ANSWERED / ANSWERED | true | 36.53 |
| unknown-version | NEEDS_CLARIFICATION / NEEDS_CLARIFICATION | true | 36.22 |
| retrieval-miss | UNRESOLVED / UNRESOLVED | true | 39.06 |
| knowledge-conflict | UNRESOLVED / UNRESOLVED | true | 38.96 |
| partial-observation | PARTIAL / PARTIAL | true | 36.87 |
| no-record | ANSWERED / ANSWERED | true | 36.39 |
| monitor-failure | UNRESOLVED / UNRESOLVED | true | 36.12 |

| 指标 | 通过 / 适用样本数 |
| --- | ---: |
| 冲突出处与阻断 | 1 / 1 |
| 召回参考片段 | 6 / 6 |
| 引用原文支持性 | 6 / 6 |
| 版本适用性 | 6 / 6 |
| 确定性答案规则 | 12 / 12 |
| 设备上下文 | 12 / 12 |
| 调用失败不是设备故障 | 1 / 1 |

端到端 p50 36.22 ms；p95 39.06 ms。失败检查：[]。

逐题输入、可接受结论、禁止回答、设备快照、知识修订及内容哈希、实际回答和指标保存在同目录 baseline.json。数据均为合成；该文件可供运维复核并作为诊断回归基准。
