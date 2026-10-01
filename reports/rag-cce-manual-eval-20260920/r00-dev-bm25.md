# r00-dev-bm25

模式：REAL_RETRIEVAL_ONLY

```json
{
  "completed": 75,
  "expected": 75,
  "errors": 0,
  "answer_accuracy": null,
  "llm_judged_answer_accuracy": null,
  "evidence_recall_at_5": 0.8840579710144928,
  "all_evidence_at_5": 0.8695652173913043,
  "document_hit_at_5": 0.8840579710144928,
  "latency_p95_ms": 1268.8952079915907
}
```

准确率仅在真实回答、语义评审和证据复核完成后报告；检索指标不代表答案正确。

## Bad cases（9）

### cce-manual-009 · 节点

为什么不能给所有区域的CCE Turbo节点承诺同一个可用规格和最大Pod数？

参考要点：区域规格可能新增、售罄或下线，需看创建节点页面；Turbo最大Pod数量与节点可用弹性网卡/辅助弹性网卡数量有关

初步排查方向：召回未覆盖全部参考证据，检查分段、标题上下文和候选排序

实际回答：未运行回答

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0719.html

召回章节：节点可创建的最大Pod数量说明/常见问题、监控中心FAQ/索引、节点可创建的最大Pod数量说明/如何提升集群中可使用Pod数量、Volcano异构资源碎片整理/以Gang语义评估工作负载中断影响、购买Standard/Turbo集群/步骤一：进行集群基本配置

### cce-manual-019 · 调度

Volcano主要弥补Kubernetes在哪类计算场景的调度能力？

参考要点：批处理平台；面向机器学习、深度学习、生物信息学/基因组学和大数据；提供高性能任务调度、异构芯片管理和任务运行管理

初步排查方向：召回未覆盖全部参考证据，检查分段、标题上下文和候选排序

实际回答：未运行回答

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0702.html

召回章节：Volcano调度器/插件简介、Volcano Job/Volcano Job、Volcano调度器/版本记录、Volcano调度概述/Volcano自定义资源、调度配置/设置集群默认调度器

### cce-manual-033 · 弹性伸缩

HPA怎样把监控到的负载变化转为副本数变化？

参考要点：周期检查Pod度量数据；计算达到目标数值所需副本数；调整Deployment等目标资源的replicas字段

初步排查方向：召回未覆盖全部参考证据，检查分段、标题上下文和候选排序

实际回答：未运行回答

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0290.html

召回章节：通过CCE配置自定义告警/添加事件类告警、通用检查项/模板检查、工作负载伸缩原理/CronHPA工作原理、工作负载伸缩原理/工作负载伸缩原理、创建AHPA策略/功能介绍

### cce-manual-049 · AI容器

通过CCE推理负载部署LLM服务，对集群和Volcano插件最低版本有什么要求？

参考要点：v1.29及以上CCE Standard或Turbo；Volcano 1.21.7及以上

初步排查方向：召回未覆盖全部参考证据，检查分段、标题上下文和候选排序

实际回答：未运行回答

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_11511.html

召回章节：部署推理负载/简介、AI容器概述/能力概览、LeaderWorkerSet插件/LeaderWorkerSet插件、多维多级网络拓扑调度/创建Kthena ModelServing示例、多维组调度（Gang）/创建Kthena ModelServing推理负载

### cce-manual-052 · AI容器

CCE AI应用预置模板能免去手写YAML吗，主要预置了哪些模型部署配置？

参考要点：无需编写复杂Kubernetes YAML；预置模型运行参数、NPU资源需求及硬件适配；通过可视化界面选模板配置

初步排查方向：召回未覆盖全部参考证据，检查分段、标题上下文和候选排序

实际回答：未运行回答

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_1154.html

召回章节：部署AI应用模板/简介、AI推理框架插件/模型模板、AI容器概述/功能优势、模板概述/Helm、AI容器概述/能力概览

### cce-manual-053 · 命名空间

CCE一个命名空间最多允许创建多少个Service？这里说的是Pod数吗？

参考要点：Service不超过6000个；指Kubernetes service资源，不是Pod数

初步排查方向：召回未覆盖全部参考证据，检查分段、标题上下文和候选排序

实际回答：未运行回答

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0278.html

召回章节：工作负载升级与回退/工作负载升级、使用容器网络配置为命名空间/工作负载绑定子网及安全组/操作场景、DNS概述/CoreDNS介绍、配置命名空间权限（Kubernetes RBAC授权）/自定义命名空间权限（kubectl）、节点可创建的最大Pod数量说明/节点最大实例数说明

### cce-manual-055 · 命名空间

想让开发、联调、测试环境共享同一CCE集群但逻辑隔离，可用什么方式组织？

参考要点：为不同环境建立对应命名空间；创建和查询工作负载时选择对应命名空间；不要宣称这等同于完整网络安全隔离

初步排查方向：召回未覆盖全部参考证据，检查分段、标题上下文和候选排序

实际回答：未运行回答

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0285.html

召回章节：创建命名空间/操作场景、管理命名空间/命名空间使用实践、Volcano队列/Volcano队列、创建命名空间/命名空间类别、在CCE Turbo分布式集群中使用边缘云资源/相关概念

### cce-manual-062 · 插件

修改CCE插件能直接在后台编辑资源吗，正确入口是什么？

参考要点：从插件中心或插件管理API操作；不要后台直接修改资源；可能导致插件异常或升级覆盖参数

初步排查方向：召回未覆盖全部参考证据，检查分段、标题上下文和候选排序

实际回答：未运行回答

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0277.html

召回章节：监控中心FAQ/索引、Kubernetes Dashboard/权限修改、云原生监控插件升级检查异常处理/解决方案、通用检查项/模板检查、CoreDNS域名解析/使用Corefile配置CoreDNS插件

### cce-manual-076 · 配置中心

CCE网络配置里扩展VPC网络容器网段，要求什么网络模型和最低集群版本？

参考要点：VPC网络模型；v1.19.16-r0及以上

初步排查方向：召回未覆盖全部参考证据，检查分段、标题上下文和候选排序

实际回答：未运行回答

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0784.html

召回章节：购买Standard/Turbo集群/步骤二：进行网络配置（第1部分）、VPC网络模型说明/VPC网络模型、DataPlane V2网络加速说明/DataPlane V2网络加速说明、容器网络模型对比/网络模型对比、删除VPC网络集群的容器网段/删除VPC网络集群的容器网段
