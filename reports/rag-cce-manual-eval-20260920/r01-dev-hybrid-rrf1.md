# r01-dev-hybrid-rrf1

模式：REAL_RETRIEVAL_ONLY

```json
{
  "completed": 75,
  "expected": 75,
  "errors": 0,
  "answer_accuracy": null,
  "llm_judged_answer_accuracy": null,
  "evidence_recall_at_5": 0.9710144927536232,
  "all_evidence_at_5": 0.9565217391304348,
  "document_hit_at_5": 0.9565217391304348,
  "latency_p95_ms": 443.5900410026079
}
```

准确率仅在真实回答、语义评审和证据复核完成后报告；检索指标不代表答案正确。

## Bad cases（3）

### cce-manual-009 · 节点

为什么不能给所有区域的CCE Turbo节点承诺同一个可用规格和最大Pod数？

参考要点：区域规格可能新增、售罄或下线，需看创建节点页面；Turbo最大Pod数量与节点可用弹性网卡/辅助弹性网卡数量有关

初步排查方向：召回未覆盖全部参考证据，检查分段、标题上下文和候选排序

实际回答：未运行回答

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0719.html

召回章节：节点可创建的最大Pod数量说明/常见问题、节点可创建的最大Pod数量说明/如何提升集群中可使用Pod数量、节点可创建的最大Pod数量说明/节点网卡数量说明（仅CCE Turbo集群）、监控中心FAQ/索引、节点可创建的最大Pod数量说明/节点最大Pod数量计算方式

### cce-manual-055 · 命名空间

想让开发、联调、测试环境共享同一CCE集群但逻辑隔离，可用什么方式组织？

参考要点：为不同环境建立对应命名空间；创建和查询工作负载时选择对应命名空间；不要宣称这等同于完整网络安全隔离

初步排查方向：召回未覆盖全部参考证据，检查分段、标题上下文和候选排序

实际回答：未运行回答

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0285.html

召回章节：使用FlexNPU实现NPU资源虚拟化与隔离/前提条件、创建命名空间/操作场景、管理命名空间/命名空间使用实践、修改CCE集群配置/网络组件配置（canal-controller）（仅容器隧道网络模型的集群支持）、Volcano队列/Volcano队列

### cce-manual-096 · 存储

EVS静态PV回收策略Delete下，设置与不设置everest.io/reclaim-policy=retain-volume-only，删除PVC结果有何不同？

参考要点：不设置时PV和云硬盘都删除；设置retain-volume-only时PV删除、底层云硬盘保留；该参数要求Everest至少1.2.9且Delete策略

初步排查方向：召回未覆盖全部参考证据，检查分段、标题上下文和候选排序

实际回答：未运行回答

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0614.html

召回章节：存储基础知识/PV回收策略、通过静态存储卷使用已有对象存储/使用已有对象存储（第2部分）、通过静态存储卷使用已有文件存储/通过kubectl命令行使用已有文件存储（第2部分）、通过静态存储卷使用已有文件存储/通过kubectl命令行使用已有文件存储（第1部分）、自定义存储类（StorageClass）/StorageClass高级配置（第3部分）
