# QA-06 REPLAY基线

查看 [报告](report.md) 与 [逐题证据](baseline.json)。12个样本覆盖两型号各两版本、
错误码、指示灯、配置前提、设备指代与切换、版本缺失、未召回、知识冲突及三种
运行数据结果。所有模型输出和监控数据均为明确的外部HTTP桩，存储为真实临时文件。

复现：

```sh
HWOPS_QA_BASELINE_OUTPUT="$PWD/reports/qa-baseline-20261003/baseline.json" \
  ./scripts/go.sh test ./test/acceptance -run '^TestQABaseline$' -count=1
./scripts/go.sh run ./cmd/hwops-eval \
  -input reports/qa-baseline-20261003/baseline.json \
  -output reports/qa-baseline-20261003/report.md
```

首轮构造“未召回”样本使用`zxqvnomatch`，与`atlas`标题共享`at`二元词，
本地词项检索正确返回了该标题，因此预期未召回失败。已将该合成测试问题改成
与语料无共享词项的`zzqqvvxx`并重跑；未调整检索排序或将失败记作通过。

真实模型语义质量与Phoenix实验沿用已有03f报告；本轮没有真实模型质量评分，
也未创建新的RAG质量实验。该综合基线属于离线程序协议验收。
PostgreSQL与生产监控接入仍未验收。
