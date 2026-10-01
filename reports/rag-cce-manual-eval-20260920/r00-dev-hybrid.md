# r00-dev-hybrid

模式：REAL_RETRIEVAL_ONLY

```json
{
  "completed": 75,
  "expected": 75,
  "errors": 0,
  "answer_accuracy": null,
  "llm_judged_answer_accuracy": null,
  "evidence_recall_at_5": 0.927536231884058,
  "all_evidence_at_5": 0.8985507246376812,
  "document_hit_at_5": 0.9130434782608695,
  "latency_p95_ms": 1271.6482919931877
}
```

准确率仅在真实回答、语义评审和证据复核完成后报告；检索指标不代表答案正确。

## Bad cases（7）

### cce-manual-019 · 调度

Volcano主要弥补Kubernetes在哪类计算场景的调度能力？

参考要点：批处理平台；面向机器学习、深度学习、生物信息学/基因组学和大数据；提供高性能任务调度、异构芯片管理和任务运行管理

初步排查方向：召回未覆盖全部参考证据，检查分段、标题上下文和候选排序

实际回答：未运行回答

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0702.html

召回章节：Volcano调度器/插件简介、Volcano Job/Volcano Job、AI负载调度/任务调度、AI负载调度/异构资源调度、多维组调度（Gang）/配置组调度策略

### cce-manual-024 · 网络

CCE的VPC网络与容器隧道网络，在封装和节点规模方面有何区别？

参考要点：VPC网络无隧道封装但节点数量受VPC路由配额限制；容器隧道网络使用VXLAN和Open vSwitch

初步排查方向：召回未覆盖全部参考证据，检查分段、标题上下文和候选排序

实际回答：未运行回答

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0281.html

召回章节：容器隧道网络模型说明/应用场景、容器网络模型对比/网络模型对比、容器隧道网络模型说明/优缺点、网络概述/集群网络构成、VPC网络模型说明/应用场景

### cce-manual-055 · 命名空间

想让开发、联调、测试环境共享同一CCE集群但逻辑隔离，可用什么方式组织？

参考要点：为不同环境建立对应命名空间；创建和查询工作负载时选择对应命名空间；不要宣称这等同于完整网络安全隔离

初步排查方向：召回未覆盖全部参考证据，检查分段、标题上下文和候选排序

实际回答：未运行回答

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0285.html

召回章节：使用FlexNPU实现NPU资源虚拟化与隔离/前提条件、创建命名空间/操作场景、管理命名空间/命名空间使用实践、修改CCE集群配置/网络组件配置（canal-controller）（仅容器隧道网络模型的集群支持）、Volcano队列/Volcano队列

### cce-manual-081 · 存储管理-Flexvolume（已弃用）

历史Flexvolume示例pvc-evs-auto-example.yaml申请容量与访问模式分别是什么，可用区有什么要求？

参考要点：10Gi；ReadWriteOnce；可用区必须与工作负载规划一致；仅说明历史Flexvolume示例，不作为当前CSI YAML

初步排查方向：召回未覆盖全部参考证据，检查分段、标题上下文和候选排序

实际回答：未运行回答

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0312.html

召回章节：使用kubectl对接已有对象存储/操作步骤、使用kubectl自动创建对象存储/操作步骤、使用kubectl自动创建文件存储/操作步骤、使用kubectl对接已有文件存储/操作步骤、1.15集群如何从Flexvolume存储类型迁移到CSI Everest存储类型/操作步骤（第1部分）

### cce-manual-084 · 高危操作一览

重装CCE控制节点和Node节点操作系统后，手册给出的恢复结论相同吗？

参考要点：控制节点重装后组件删除，不可恢复；Node节点重装后可按文档重置节点

初步排查方向：召回未覆盖全部参考证据，检查分段、标题上下文和候选排序

实际回答：未运行回答

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0054.html

召回章节：节点限制检查异常处理/解决方案、节点自愈功能/节点重启后故障未恢复、重置节点/操作场景、移除节点/约束限制、节点操作系统说明/操作系统内核更新信息

### cce-manual-093 · 模板（Helm Chart）

Helm v3 chart的crds目录，升级release会更新CRD吗，删除release会删CRD吗？

参考要点：crds目录仅安装release时部署；升级不更新；删除不卸载；推荐分离CRD和使用CRD资源的chart管理

初步排查方向：召回未覆盖全部参考证据，检查分段、标题上下文和候选排序

实际回答：未运行回答

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0421.html

召回章节：Helm v2 Release转换成Helm v3 Release/Helm v2 Release转换成Helm v3 Release、Helm v2 Release转换成Helm v3 Release/注意事项：、Helm v2 Release转换成Helm v3 Release/背景介绍、Helm v2 Release转换成Helm v3 Release/转换流程（使用Helm v3客户端）、Helm v2 Release转换成Helm v3 Release/转换流程（不使用Helm v3客户端）

### cce-manual-096 · 存储

EVS静态PV回收策略Delete下，设置与不设置everest.io/reclaim-policy=retain-volume-only，删除PVC结果有何不同？

参考要点：不设置时PV和云硬盘都删除；设置retain-volume-only时PV删除、底层云硬盘保留；该参数要求Everest至少1.2.9且Delete策略

初步排查方向：召回未覆盖全部参考证据，检查分段、标题上下文和候选排序

实际回答：未运行回答

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0614.html

召回章节：存储基础知识/PV回收策略、通过静态存储卷使用已有对象存储/使用已有对象存储（第2部分）、通过静态存储卷使用已有文件存储/通过kubectl命令行使用已有文件存储（第2部分）、自定义存储类（StorageClass）/StorageClass高级配置（第3部分）、自定义存储类（StorageClass）/StorageClass基础配置
