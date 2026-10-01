# manual-v3-regression

模式：REAL_RETRIEVAL_ONLY

```json
{
  "completed": 75,
  "expected": 75,
  "errors": 0,
  "service_failures": 0,
  "answer_accuracy": null,
  "llm_judged_answer_accuracy": null,
  "evidence_recall_at_5": 0.9565217391304348,
  "all_evidence_at_5": 0.9420289855072463,
  "document_hit_at_5": 0.9130434782608695,
  "latency_p95_ms": 3846.066250000149,
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

## Bad cases（4）

### cce-manual-033 · 弹性伸缩

HPA怎样把监控到的负载变化转为副本数变化？

参考要点：周期检查Pod度量数据；计算达到目标数值所需副本数；调整Deployment等目标资源的replicas字段

初步排查方向：未命中全部参考锚点，核对等价证据、分段及候选排序

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0290.html

召回章节：工作负载伸缩原理/HPA工作原理、工作负载伸缩原理/CronHPA工作原理、通过CCE配置自定义告警/添加事件类告警、工作负载伸缩原理/HPA工作原理、Kubernetes原生配置/负载弹性伸缩控制器（horizontal-pod-autoscaler-controller）配置

### cce-manual-055 · 命名空间

想让开发、联调、测试环境共享同一CCE集群但逻辑隔离，可用什么方式组织？

参考要点：为不同环境建立对应命名空间；创建和查询工作负载时选择对应命名空间；不要宣称这等同于完整网络安全隔离

初步排查方向：未命中全部参考锚点，核对等价证据、分段及候选排序

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0285.html

召回章节：使用FlexNPU实现NPU资源虚拟化与隔离/前提条件、管理命名空间/命名空间使用实践、创建命名空间/操作场景、连接集群概述/连接集群方式对比、开启云原生混部/云原生混部配置

### cce-manual-064 · 插件

集群Pod因工作节点资源不足而调度失败，CCE集群弹性引擎会怎样处理，利用率低时呢？

参考要点：基于Autoscaler扩容新节点；扩容节点资源利用率很低时自动删除节点；不保证其他原因导致的Pending都能靠扩容解决

初步排查方向：未命中全部参考锚点，核对等价证据、分段及候选排序

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0154.html

召回章节：创建节点弹性策略/常见问题、CCE增强集群弹性引擎/插件说明 / 约束与限制、插件概述/容器调度与弹性插件、创建HPA策略/前提条件 / 约束与限制、自定义节点池的节点缩容条件/步骤二：为节点池设置自定义缩容条件

### cce-manual-094 · 模板（Helm Chart）

Helm v2与v3默认用什么存release信息，v3允许跨命名空间重用release名字吗？

参考要点：v2默认ConfigMap；v3默认Secret；v3可在不同命名空间重用release名称

初步排查方向：未命中全部参考锚点，核对等价证据、分段及候选排序

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0421.html

召回章节：Helm v2与Helm v3的差异及适配方案/Helm v2与Helm v3的差异及适配方案、Helm v2 Release转换成Helm v3 Release/背景介绍 / 注意事项：、Helm v2 Release转换成Helm v3 Release/转换流程（不使用Helm v3客户端）、Helm v2 Release转换成Helm v3 Release/转换流程（使用Helm v3客户端）、Helm v2与Helm v3的差异及适配方案/Helm v2与Helm v3的差异及适配方案
