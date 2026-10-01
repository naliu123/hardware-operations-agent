# r00-dev-dense

模式：REAL_RETRIEVAL_ONLY

```json
{
  "completed": 75,
  "expected": 75,
  "errors": 0,
  "answer_accuracy": null,
  "llm_judged_answer_accuracy": null,
  "evidence_recall_at_5": 0.7536231884057971,
  "all_evidence_at_5": 0.7246376811594203,
  "document_hit_at_5": 0.7246376811594203,
  "latency_p95_ms": 1258.064875000855
}
```

准确率仅在真实回答、语义评审和证据复核完成后报告；检索指标不代表答案正确。

## Bad cases（19）

### cce-manual-001 · 集群

CCE Standard和Turbo的网络选择有什么区别，Turbo的Pod地址如何分配？

参考要点：Standard可选择容器隧道或VPC网络；Turbo采用云原生网络2.0；Turbo的Pod直接从VPC网段分配IP，可与节点在不同子网

初步排查方向：召回未覆盖全部参考证据，检查分段、标题上下文和候选排序

实际回答：未运行回答

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0430.html

召回章节：在CCE Turbo集群中为Pod配置固定IP/在CCE Turbo集群中为Pod配置固定IP、在CCE Turbo集群中为Pod配置EIP/在CCE Turbo集群中为Pod配置EIP、在CCE Turbo集群中为Pod配置固定EIP/在CCE Turbo集群中为Pod配置固定EIP、在CCE Turbo集群中为IPv6双栈网卡的Pod配置共享带宽/在CCE Turbo集群中为IPv6双栈网卡的Pod配置共享带宽、在CCE Turbo集群中为IPv6双栈网卡的Pod配置共享带宽/通过控制台设置

### cce-manual-002 · 集群

CCE Autopilot是否需要用户自行部署、管理节点，它按什么资源付费？

参考要点：无需维护节点的部署管理和安全性；按CPU和内存资源用量按需付费

初步排查方向：召回未覆盖全部参考证据，检查分段、标题上下文和候选排序

实际回答：未运行回答

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0430.html

召回章节：CCE容器网络扩展指标/使用约束、部署AI应用模板/前提条件、AI推理网关插件/前提条件、创建节点/相关文档、CCE节点故障检测/相关文档

### cce-manual-008 · 节点

为什么CCE 1.35及以上不再支持EulerOS 2.10和kata安全运行时？

参考要点：1.35默认nftables要求内核至少5.13；EulerOS 2.10内核不满足；kata仅适配EulerOS 2.10所以也不再支持

初步排查方向：召回未覆盖全部参考证据，检查分段、标题上下文和候选排序

实际回答：未运行回答

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0476.html

召回章节：CCE节点故障检测/约束与限制、CCE补丁版本发布记录/v1.28版本（第1部分）、CCE补丁版本发布记录/v1.25版本、CCE节点故障检测/权限说明、（停止维护）Kubernetes 1.25版本说明/CCE对Kubernetes 1.25版本的增强

### cce-manual-009 · 节点

为什么不能给所有区域的CCE Turbo节点承诺同一个可用规格和最大Pod数？

参考要点：区域规格可能新增、售罄或下线，需看创建节点页面；Turbo最大Pod数量与节点可用弹性网卡/辅助弹性网卡数量有关

初步排查方向：召回未覆盖全部参考证据，检查分段、标题上下文和候选排序

实际回答：未运行回答

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0719.html

召回章节：节点可创建的最大Pod数量说明/常见问题、节点可创建的最大Pod数量说明/节点网卡数量说明（仅CCE Turbo集群）、节点可创建的最大Pod数量说明/节点最大Pod数量计算方式、使用节点池配置为节点池上的Pod绑定默认的安全组/使用节点池配置为节点池上的Pod绑定默认的安全组、节点可创建的最大Pod数量说明/节点最大实例数说明

### cce-manual-018 · 工作负载

我想在集群每个节点部署日志采集器和节点监控，应选择哪类工作负载？

参考要点：DaemonSet守护进程集；日志采集进程和节点监控进程适合逐节点部署

初步排查方向：召回未覆盖全部参考证据，检查分段、标题上下文和候选排序

实际回答：未运行回答

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0216.html

召回章节：工作负载概述/守护进程集（DaemonSet）、集群配置概览/集群控制节点可用区、日志中心FAQ/节点负载过多，采集日志时缺少部分Pod信息、工作负载监控/功能入口、节点监控/节点监控

### cce-manual-024 · 网络

CCE的VPC网络与容器隧道网络，在封装和节点规模方面有何区别？

参考要点：VPC网络无隧道封装但节点数量受VPC路由配额限制；容器隧道网络使用VXLAN和Open vSwitch

初步排查方向：召回未覆盖全部参考证据，检查分段、标题上下文和候选排序

实际回答：未运行回答

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0281.html

召回章节：容器隧道网络模型说明/应用场景、容器网络模型对比/网络模型对比、容器隧道网络模型说明/容器隧道网络模型、容器隧道网络模型说明/优缺点、VPC网络模型说明/应用场景

### cce-manual-029 · 存储

使用EVS保存容器数据时，删掉容器会丢失卷中数据吗，迁移有何可用区前提？

参考要点：删除容器后卷数据仍在存储系统；随容器迁移要求同一可用区

初步排查方向：召回未覆盖全部参考证据，检查分段、标题上下文和候选排序

实际回答：未运行回答

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0613.html

召回章节：删除节点池/注意事项、删除节点池/约束与限制、临时存储卷概述/约束与限制、在存储池中导入临时卷/约束与限制、通过动态存储卷使用文件存储/约束与限制

### cce-manual-051 · AI容器

Kthena ModelServing要补足Deployment/StatefulSet处理LLM推理时的哪些局限？

参考要点：拓扑感知；原子调度；复杂推理工作流编排；为PD分离等模式提供声明式生命周期管理

初步排查方向：召回未覆盖全部参考证据，检查分段、标题上下文和候选排序

实际回答：未运行回答

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_1123.html

召回章节：Kthena ModelServing/Eviction保护（Eviction Protection）、Kthena ModelServing/架构设计、Kthena ModelServing/作业调度（Gang Scheduling）、Kthena ModelServing/约束与限制、Kthena ModelServing/使用示例：ModelServing负载完整用例（以Nginx负载为例）

### cce-manual-055 · 命名空间

想让开发、联调、测试环境共享同一CCE集群但逻辑隔离，可用什么方式组织？

参考要点：为不同环境建立对应命名空间；创建和查询工作负载时选择对应命名空间；不要宣称这等同于完整网络安全隔离

初步排查方向：召回未覆盖全部参考证据，检查分段、标题上下文和候选排序

实际回答：未运行回答

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0285.html

召回章节：使用FlexNPU实现NPU资源虚拟化与隔离/前提条件、修改CCE集群配置/网络组件配置（canal-controller）（仅容器隧道网络模型的集群支持）、修改CCE集群配置/集群配置参数说明、使用FlexNPU实现NPU资源虚拟化与隔离/步骤一：安装CCE AI套件（Ascend NPU）并开启FlexNPU特性、通过CloudShell连接集群/前提条件

### cce-manual-058 · 配置项与密钥

ConfigMap适合保存敏感密码吗，应用可以用哪两种方式读取？

参考要点：用于非敏感配置，密码应使用Secret等适当机制；文件挂载；环境变量注入

初步排查方向：召回未覆盖全部参考证据，检查分段、标题上下文和候选排序

实际回答：未运行回答

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0152.html

召回章节：创建密钥/操作场景、集群系统密钥说明/default-secret、插件概述/容器安全插件、创建密钥/Secret资源文件配置示例、加密对象存储卷/配置KMS权限

### cce-manual-080 · 存储管理-Flexvolume（已弃用）

已弃用Flexvolume云硬盘文档中，两个Pod用同一EVS却调度到不同节点会怎样？

参考要点：云硬盘非共享，不能同时被多个节点挂载；必有一个Pod无法挂载而不能启动；需指出这是历史Flexvolume文档范围

初步排查方向：召回未覆盖全部参考证据，检查分段、标题上下文和候选排序

实际回答：未运行回答

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0310.html

召回章节：通过静态存储卷使用已有云硬盘/约束与限制、Volcano异构资源碎片整理/为什么指定了节点范围，Pod 仍可能被调度到范围外的节点？、负载感知调度/为什么同一批新建的Pod没有被全部调度到CPU或内存使用率最低的节点？、通过动态存储卷使用云硬盘/约束与限制、节点CPU使用率检查异常处理/解决方案

### cce-manual-081 · 存储管理-Flexvolume（已弃用）

历史Flexvolume示例pvc-evs-auto-example.yaml申请容量与访问模式分别是什么，可用区有什么要求？

参考要点：10Gi；ReadWriteOnce；可用区必须与工作负载规划一致；仅说明历史Flexvolume示例，不作为当前CSI YAML

初步排查方向：召回未覆盖全部参考证据，检查分段、标题上下文和候选排序

实际回答：未运行回答

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0312.html

召回章节：1.15集群如何从Flexvolume存储类型迁移到CSI Everest存储类型/操作步骤（第2部分）、通过StorageClass动态创建SFS Turbo子目录/创建Deployment挂载已有数据卷、通过StorageClass动态创建SFS Turbo子目录/StatefulSet动态创建subpath模式的数据卷、使用kubectl自动创建对象存储/操作步骤、使用kubectl对接已有对象存储/操作步骤

### cce-manual-084 · 高危操作一览

重装CCE控制节点和Node节点操作系统后，手册给出的恢复结论相同吗？

参考要点：控制节点重装后组件删除，不可恢复；Node节点重装后可按文档重置节点

初步排查方向：召回未覆盖全部参考证据，检查分段、标题上下文和候选排序

实际回答：未运行回答

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0054.html

召回章节：节点自愈功能/节点自愈后残留node.cce.io/repair污点、删除/退订节点/操作场景、重置节点/重置DefaultPool中的节点、节点OS检查异常处理/检查项内容、节点自愈功能/节点重启后故障未恢复

### cce-manual-086 · 高危操作一览

删除/usr/local/bin/crictl后containerd持续重启，文档给出的原因与恢复方法是什么？

参考要点：crictl丢失使containerd健康检查异常；按文档重置节点恢复二进制和配置文件

初步排查方向：召回未覆盖全部参考证据，检查分段、标题上下文和候选排序

实际回答：未运行回答

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0054.html

召回章节：Containerd Pod重启风险检查异常处理/解决方案、节点自愈功能/重启失败、节点关键目录文件权限检查异常处理/解决方案、Containerd Pod重启风险检查异常处理/检查项内容、工作负载升级与回退/工作负载回退

### cce-manual-090 · 弹性伸缩

按HPA文档公式，当前3副本、指标150m、目标100m，不考虑冷却和限额，应建议几个副本？

参考要点：ceil(3×150/100)=5；向上取整，不是4

初步排查方向：召回未覆盖全部参考证据，检查分段、标题上下文和候选排序

实际回答：未运行回答

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0290.html

召回章节：创建HPA策略/相关文档、使用HPA+CA实现工作负载和节点联动弹性伸缩/创建HPA策略、使用HPA+CA实现工作负载和节点联动弹性伸缩/应用场景、创建CustomedHPA策略/相关文档、创建AHPA策略/相关文档

### cce-manual-091 · 弹性伸缩

metrics.k8s.io、custom.metrics.k8s.io和external.metrics.k8s.io分别提供什么指标？

参考要点：metrics提供Pod/Node的CPU内存；custom提供Kubernetes对象关联的自定义指标；external来自外部、不关联Kubernetes资源

初步排查方向：召回未覆盖全部参考证据，检查分段、标题上下文和候选排序

实际回答：未运行回答

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0290.html

召回章节：使用Prometheus监控控制节点组件指标/相关文档、K8s组件内存资源限制检查异常处理/检查项内容、Kubernetes Metrics Server/组件说明、Prometheus（停止维护）/通过Metrics API提供资源指标、K8s组件内存资源限制检查异常处理/K8s组件内存资源限制检查异常处理

### cce-manual-093 · 模板（Helm Chart）

Helm v3 chart的crds目录，升级release会更新CRD吗，删除release会删CRD吗？

参考要点：crds目录仅安装release时部署；升级不更新；删除不卸载；推荐分离CRD和使用CRD资源的chart管理

初步排查方向：召回未覆盖全部参考证据，检查分段、标题上下文和候选排序

实际回答：未运行回答

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0421.html

召回章节：Helm v2 Release转换成Helm v3 Release/注意事项：、Helm v2 Release转换成Helm v3 Release/背景介绍、Helm v2 Release转换成Helm v3 Release/Helm v2 Release转换成Helm v3 Release、Helm v2 Release转换成Helm v3 Release/转换流程（不使用Helm v3客户端）、Helm v2 Release转换成Helm v3 Release/转换流程（使用Helm v3客户端）

### cce-manual-094 · 模板（Helm Chart）

Helm v2与v3默认用什么存release信息，v3允许跨命名空间重用release名字吗？

参考要点：v2默认ConfigMap；v3默认Secret；v3可在不同命名空间重用release名称

初步排查方向：召回未覆盖全部参考证据，检查分段、标题上下文和候选排序

实际回答：未运行回答

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0421.html

召回章节：Helm v2 Release转换成Helm v3 Release/注意事项：、Helm v2 Release转换成Helm v3 Release/背景介绍、Helm v2 Release转换成Helm v3 Release/Helm v2 Release转换成Helm v3 Release、Helm v2 Release转换成Helm v3 Release/转换流程（不使用Helm v3客户端）、Helm v2 Release转换成Helm v3 Release/转换流程（使用Helm v3客户端）

### cce-manual-096 · 存储

EVS静态PV回收策略Delete下，设置与不设置everest.io/reclaim-policy=retain-volume-only，删除PVC结果有何不同？

参考要点：不设置时PV和云硬盘都删除；设置retain-volume-only时PV删除、底层云硬盘保留；该参数要求Everest至少1.2.9且Delete策略

初步排查方向：召回未覆盖全部参考证据，检查分段、标题上下文和候选排序

实际回答：未运行回答

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0614.html

召回章节：通过静态存储卷使用已有对象存储/使用已有对象存储（第2部分）、存储基础知识/PV回收策略、自定义存储类（StorageClass）/StorageClass高级配置（第3部分）、通过静态存储卷使用已有文件存储/约束与限制、通过静态存储卷使用已有文件存储/通过kubectl命令行使用已有文件存储（第2部分）
