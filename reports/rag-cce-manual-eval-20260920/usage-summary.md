# 03c已保存模型用量

生成时间：2026-09-22T14:48:25.246266+00:00

下表仅汇总已保存usage。存在早期未记录的失败请求，不能当作精确费用总账。

| 产物 | 类型 | 已记录调用 | tokens |
| --- | --- | ---: | ---: |
| [live-r00-dev-hybrid60.json](live-r00-dev-hybrid60.json) | public_http_generation_including_selection | 75 | 256833 |
| [live-r00-dev-hybrid60.json](live-r00-dev-hybrid60.json) | judge | 75 | 341638 |
| [live-r01-dev-hybrid1.json](live-r01-dev-hybrid1.json) | public_http_generation_including_selection | 75 | 277854 |
| [live-r01-dev-hybrid1-judge-attempts.jsonl](live-r01-dev-hybrid1-judge-attempts.jsonl) | judge | 75 | 385308 |
| [live-r02-dev-completeness.json](live-r02-dev-completeness.json) | public_http_generation_including_selection | 75 | 317788 |
| [live-r02-dev-completeness-judge-attempts.jsonl](live-r02-dev-completeness-judge-attempts.jsonl) | judge | 79 | 424021 |
| [live-r03-dev-scope-title.json](live-r03-dev-scope-title.json) | public_http_generation_including_selection | 76 | 336610 |
| [live-r03-dev-scope-title-judge-attempts.jsonl](live-r03-dev-scope-title-judge-attempts.jsonl) | judge | 75 | 395266 |
| [live-r04-dev-evidence-selection.json](live-r04-dev-evidence-selection.json) | public_http_generation_including_selection | 26 | 350113 |
| [live-r04-dev-evidence-selection.json](live-r04-dev-evidence-selection.json) | judge | 0 | 0 |
| [evidence-blocks-probe.json](evidence-blocks-probe.json) | diagnostic_probe_not_accuracy | 5 | 21647 |
| [generation-extract-probe.json](generation-extract-probe.json) | diagnostic_probe_not_accuracy | 8 | 35314 |
| [generation-review-probe.json](generation-review-probe.json) | diagnostic_probe_not_accuracy | 8 | 35734 |
| [generation-scope-probe.json](generation-scope-probe.json) | diagnostic_probe_not_accuracy | 8 | 32025 |
| [generation-thinking-probe.json](generation-thinking-probe.json) | diagnostic_probe_not_accuracy | 5 | 31517 |
| [operation-scope-probe.json](operation-scope-probe.json) | diagnostic_probe_not_accuracy | 2 | 23808 |
| [r02-gap-probe.json](r02-gap-probe.json) | diagnostic_probe_not_accuracy | 3 | 9475 |
| [r03-gap-probe.json](r03-gap-probe.json) | diagnostic_probe_not_accuracy | 3 | 9331 |
| [r04-selection-probe.json](r04-selection-probe.json) | diagnostic_probe_not_accuracy | 16 | 221955 |
| [r04b-selection-probe.json](r04b-selection-probe.json) | diagnostic_probe_not_accuracy | 16 | 204486 |
| [r04c-selection-probe.json](r04c-selection-probe.json) | diagnostic_probe_not_accuracy | 6 | 86507 |
| [rerank-probe.json](rerank-probe.json) | diagnostic_probe_not_accuracy | 4 | 69061 |
| [selection-temperature-probe.json](selection-temperature-probe.json) | diagnostic_probe_not_accuracy | 3 | 76562 |

已记录合计：718次，3942853 tokens。

缺失：R01一次格式失败评审的tokens；R03题082失败生成的调用和tokens。
诊断探针不计为问答准确率；运行中的轮次仅统计已落盘结果。
