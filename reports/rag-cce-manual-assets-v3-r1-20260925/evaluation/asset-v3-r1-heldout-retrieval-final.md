# asset-v3-r1-heldout-retrieval-final

模式：REAL_RETRIEVAL_ONLY

```json
{
  "completed": 20,
  "expected": 20,
  "errors": 0,
  "service_failures": 0,
  "answer_accuracy": null,
  "llm_judged_answer_accuracy": null,
  "evidence_recall_at_5": 0.9,
  "all_evidence_at_5": 0.9,
  "document_hit_at_5": 1.0,
  "latency_p95_ms": 3675.2216670138296,
  "generation_usage": {
    "calls": 0,
    "prompt_tokens": 0,
    "completion_tokens": 0,
    "total_tokens": 0
  },
  "citation_integrity_rate": null
}
```

准确率仅在真实回答、语义评审和证据复核完成后报告；检索指标不代表答案正确。

## Bad cases（2）

### cce-asset-table-explanatory-015 · TABLE_EXPLANATORY

在CCE补丁版本发布记录的“v1.28版本”中，表9 v1.28补丁版本发布说明里，CCE集群补丁版本v1.28.15-r94对应哪个Kubernetes社区版本？

参考要点：v1.28.15

初步排查方向：未命中全部参考锚点，核对等价证据、分段及候选排序

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0405.html

召回章节：集群升级/集群升级路径、CCE补丁版本发布记录/v1.27版本、CCE补丁版本发布记录/更早版本 / v1.28版本、CCE补丁版本发布记录/v1.36版本、CCE补丁版本发布记录/v1.25版本

### cce-asset-image-048 · IMAGE

在《Prometheus插件平滑迁移实践》的“采集配置迁移”截图中，红框标注的配置项内容是什么？

参考要点：红框标注的配置项内容是- drop:--no-collector.arp。

初步排查方向：未命中全部参考锚点，核对等价证据、分段及候选排序

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0905.html

召回章节：Prometheus插件平滑迁移实践/采集配置迁移、Prometheus插件平滑迁移实践/Prometheus插件平滑迁移实践、Prometheus插件平滑迁移实践/采集配置迁移、Prometheus插件平滑迁移实践/采集配置迁移、Prometheus插件平滑迁移实践/采集数据迁移
