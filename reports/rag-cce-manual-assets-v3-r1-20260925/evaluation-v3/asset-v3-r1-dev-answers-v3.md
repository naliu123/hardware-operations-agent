# asset-v3-r1-dev-answers-v3

模式：LIVE_ANSWERS

```json
{
  "completed": 40,
  "expected": 40,
  "errors": 0,
  "service_failures": 2,
  "answer_accuracy": null,
  "llm_judged_answer_accuracy": null,
  "evidence_recall_at_5": 0.925,
  "all_evidence_at_5": 0.925,
  "document_hit_at_5": 0.9,
  "latency_p95_ms": 32810.04616699647,
  "generation_usage": {
    "calls": 77,
    "prompt_tokens": 2039898,
    "completion_tokens": 12584,
    "total_tokens": 2052482
  },
  "citation_integrity_rate": 1.0,
  "evidence_selection": {
    "responses": 39,
    "candidate_fragments_total": 1660,
    "expanded_fragments_total": 164,
    "input_bytes_total": 6836011,
    "final_context_bytes_max": 43661
  }
}
```

准确率仅在真实回答、语义评审和证据复核完成后报告；检索指标不代表答案正确。

## Bad cases（3）

### cce-asset-image-054 · IMAGE

在《使用Prometheus监控控制节点组件指标》的“通过监控中心查看控制节点组件指标”截图中，右侧“采集配置”面板的ServiceMonitor表格里，命名空间为monitoring且标签包含app.kubernetes.io/name: dcgm-exporter的表格行，其“启用”列显示的状态是什么？

参考要点：关闭

初步排查方向：未命中全部参考锚点，核对等价证据、分段及候选排序

实际回答：资料范围：
实现DCGM指标的全面监控
目录：用户指南 > 调度 > GPU调度 > GPU监控 > 实现DCGM指标的全面监控
更新时间：2026-06-17 GMT+08:00

在《使用Prometheus监控控制节点组件指标》的“通过监控中心查看控制节点组件指标”截图中，右侧“采集配置”面板的ServiceMonitor表格里，命名空间为monitoring且标签包含app.kubernetes.io/name: dcgm-exporter的表格行，其“启用”列显示为蓝色开启状态的开关（即已启用）。手册原文：图片可见内容：页面标题为“采集配置”。顶部有“ServiceMonitor”和“PodMonitor”两个页签，当前选中“ServiceMonitor”。下方有“YAML创建”按钮。搜索栏显示筛选条件“名称: dcgm-exporter”，右侧有“添加筛选条件”输入框。表格列包括：名称、命名空间、标签、创建时间、启用、操作。表格中有一行数据：名称“dcgm-exporter”，命名空间“monitoring”，标签“app.kubernetes.io/name: dcgm-exporter”，创建时间“53天前”，启用列为蓝色开启状态的开关（被红框标出），操作列有“编辑YAML”和“删除”链接。

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0559.html

召回章节：实现DCGM指标的全面监控/步骤二：采集DCGM指标信息

### cce-asset-image-056 · IMAGE

在《变更集群规格》的“自动升配”截图中，自动升配触发流程图中“控制节点滚动更新”节点有一条向右的“失败”分支指向哪个节点？

参考要点：升配失败 自动退避重试

初步排查方向：未命中全部参考锚点，核对等价证据、分段及候选排序

实际回答：

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0403.html

召回章节：

### cce-asset-image-060 · IMAGE

在《集群关联子网可用IP余量监控及告警配置》的“方案原理”截图中，架构流程图里标注“3.指标聚合”的箭头是由哪两个实例指向“实例C”的？

参考要点：实例A和实例B

初步排查方向：未命中全部参考锚点，核对等价证据、分段及候选排序

实际回答：

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_1066.html

召回章节：
