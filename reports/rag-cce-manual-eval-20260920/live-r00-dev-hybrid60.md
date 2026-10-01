# live-r00-dev-hybrid60

模式：LIVE_ANSWERS

```json
{
  "completed": 75,
  "expected": 75,
  "errors": 0,
  "answer_accuracy": 0.64,
  "llm_judged_answer_accuracy": 0.6933333333333334,
  "evidence_recall_at_5": 0.927536231884058,
  "all_evidence_at_5": 0.8985507246376812,
  "document_hit_at_5": 0.9130434782608695,
  "latency_p95_ms": 5390.464833006263,
  "generation_usage": {
    "calls": 75,
    "prompt_tokens": 244285,
    "completion_tokens": 12548,
    "total_tokens": 256833
  },
  "citation_integrity_rate": 1.0,
  "llm_judged_passed": 52,
  "wilson_95": [
    0.5816933204791398,
    0.7861328017710428
  ],
  "audit_status": "completed",
  "real_model_calls": 75,
  "llm_judged_answerable_accuracy": 0.7536231884057971,
  "llm_judged_abstention_accuracy": 0.0,
  "audit_required_ids": [
    "cce-manual-001",
    "cce-manual-030",
    "cce-manual-050",
    "cce-manual-055",
    "cce-manual-064",
    "cce-manual-068",
    "cce-manual-069",
    "cce-manual-071",
    "cce-manual-080",
    "cce-manual-081",
    "cce-manual-082",
    "cce-manual-084",
    "cce-manual-087",
    "cce-manual-088",
    "cce-manual-093",
    "cce-manual-096",
    "cce-manual-097",
    "cce-negative-runtime-nodes",
    "cce-negative-runtime-backup",
    "cce-negative-runtime-security",
    "cce-negative-unprovided-cve",
    "cce-negative-unprovided-quota",
    "cce-negative-unprovided-extension",
    "cce-manual-002",
    "cce-manual-074",
    "cce-manual-095",
    "cce-manual-098",
    "cce-manual-022",
    "cce-manual-013",
    "cce-manual-012",
    "cce-manual-052",
    "cce-manual-035",
    "cce-manual-006",
    "cce-manual-016"
  ],
  "judge_usage": {
    "prompt_tokens": 332914,
    "completion_tokens": 8724,
    "total_tokens": 341638
  },
  "audited_passed": 48,
  "audited_count": 39,
  "audited_wilson_95": [
    0.5270045111400723,
    0.7393523360066117
  ],
  "answerable_accuracy": 0.6956521739130435,
  "abstention_accuracy": 0.0
}
```

准确率仅在真实回答、语义评审和证据复核完成后报告；检索指标不代表答案正确。

## Bad cases（27）

### cce-manual-001 · 集群

CCE Standard和Turbo的网络选择有什么区别，Turbo的Pod地址如何分配？

参考要点：Standard可选择容器隧道或VPC网络；Turbo采用云原生网络2.0；Turbo的Pod直接从VPC网段分配IP，可与节点在不同子网

初步排查方向：召回证据齐全，检查生成中的条件遗漏、对象混淆或评审误判

实际回答：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0430.html

召回章节：购买Standard/Turbo集群/步骤二：进行网络配置（第1部分）、在CCE Turbo集群中为Pod配置固定IP/约束限制、在CCE Turbo集群中为Pod配置固定IP/在CCE Turbo集群中为Pod配置固定IP、集群概述/集群的网络、在CCE Turbo集群中为Pod配置EIP/在CCE Turbo集群中为Pod配置EIP

评审：答案为空白，未提供任何实质内容。required_facts中的三个必答点（Standard可选容器隧道或VPC网络、Turbo采用云原生网络2.0、Turbo的Pod直接从VPC网段分配IP且可与节点在不同子网）均未被回答，因此facts_met全为false。答案未编造数据，也未与证据矛盾，属于正确指出资料不足的拒答，故correct_abstention为true，direct_answer为false。

证据复核（以绑定当前答案的复核为准）：无答案。cce_10_0430第11-12行有三项依据，但实际召回没有完整对应正文；宽锚点命中不等于证据齐全。

### cce-manual-009 · 节点

为什么不能给所有区域的CCE Turbo节点承诺同一个可用规格和最大Pod数？

参考要点：区域规格可能新增、售罄或下线，需看创建节点页面；Turbo最大Pod数量与节点可用弹性网卡/辅助弹性网卡数量有关

初步排查方向：召回证据齐全，检查生成中的条件遗漏、对象混淆或评审误判

实际回答：CCE Turbo集群节点可创建的Pod数与节点网卡数量有关，不同规格支持的网卡数量可能不同。

不同区域支持的节点规格（flavor）不同，且节点规格存在新增、售罄下线等情况。

对于CCE Turbo集群，由于Pod需要占用节点弹性网卡/辅助弹性网卡，每个节点可以创建最大Pod数量与节点可使用网卡数量相关。

节点可创建的Pod数并非根据节点规格单一因素决定，是根据集群类型和其他参数共同作用的结果。

CCE Turbo集群ECS节点使用弹性辅助网卡，裸金属节点使用弹性网卡，节点可以创建最大Pod数量与节点可使用网卡数量相关。

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0719.html

召回章节：节点可创建的最大Pod数量说明/常见问题、节点可创建的最大Pod数量说明/如何提升集群中可使用Pod数量、节点规格说明/节点规格说明、节点可创建的最大Pod数量说明/节点网卡数量说明（仅CCE Turbo集群）、监控中心FAQ/索引

评审：答案完整覆盖两个必答点：第一点说明不同区域支持的节点规格不同且存在新增、售罄下线等情况，与原文一致；第二点说明CCE Turbo集群Pod占用弹性网卡/辅助弹性网卡，最大Pod数与节点可使用网卡数量相关，与原文一致。答案中关于ECS节点使用弹性辅助网卡、裸金属节点使用弹性网卡以及Pod数由集群类型和其他参数共同决定的表述均有召回内容支持，无矛盾或无依据断言。

证据复核（以绑定当前答案的复核为准）：回答包含区域规格变动和网卡限制，但遗漏冻结必答点中的在创建节点页面核对规格；LLM漏判。

### cce-manual-019 · 调度

Volcano主要弥补Kubernetes在哪类计算场景的调度能力？

参考要点：批处理平台；面向机器学习、深度学习、生物信息学/基因组学和大数据；提供高性能任务调度、异构芯片管理和任务运行管理

初步排查方向：召回未覆盖全部参考证据，检查分段、标题上下文和候选排序

实际回答：Volcano主要弥补Kubernetes在批处理与高性能计算（HPC）场景下的调度能力。

Volcano是一个基于Kubernetes的批处理平台，提供了机器学习、深度学习、生物信息学、基因组学及其他大数据应用所需要而Kubernetes当前缺失的特性。

Volcano面向批量计算、高性能计算场景提供成组调度、优先级抢占、装箱、资源预留、任务拓扑关系等高级调度能力。

Volcano为批量计算任务提供公平调度（DRF）和组调度（Gang）能力，Gang调度满足“All or nothing”需求，可解决分布式训练任务的资源忙等待和死锁问题。

Volcano为CPU、GPU、NPU等异构资源提供GPU共享调度、NUMA拓扑感知调度、NPU拓扑调度等能力。

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0702.html

召回章节：Volcano调度器/插件简介、Volcano Job/Volcano Job、AI负载调度/任务调度、AI负载调度/异构资源调度、多维组调度（Gang）/配置组调度策略

评审：答案明确回答Volcano主要弥补Kubernetes在批处理与高性能计算（HPC）场景的调度能力，并完整覆盖三个必答点：批处理平台；面向机器学习、深度学习、生物信息学/基因组学和大数据；提供高性能任务调度、异构芯片管理和任务运行管理。答案中关于成组调度、优先级抢占、装箱、资源预留、任务拓扑关系、DRF、Gang、GPU共享调度、NUMA拓扑感知调度、NPU拓扑调度等描述均与召回原文一致，未发现矛盾或无依据的实质性断言。

证据复核（以绑定当前答案的复核为准）：批处理、应用领域和调度/异构资源能力均有，但未回答必答点中的任务运行管理；LLM把参考事实当成答案已有内容。

### cce-manual-024 · 网络

CCE的VPC网络与容器隧道网络，在封装和节点规模方面有何区别？

参考要点：VPC网络无隧道封装但节点数量受VPC路由配额限制；容器隧道网络使用VXLAN和Open vSwitch

初步排查方向：召回未覆盖全部参考证据，检查分段、标题上下文和候选排序

实际回答：容器隧道网络通过VXLAN隧道封装构建独立于节点网络平面的容器网络平面，存在约5%-15%的性能损耗。

VPC网络没有隧道封装，跨节点通过VPC路由器转发，性能接近VPC网络性能，但存在NAT转换损耗。

容器隧道网络没有网络基础设施限制，把广播域控制到节点级别，最大可支持2000节点规模。

VPC网络受限于VPC路由表条目配额，集群每添加一个节点会在VPC路由表中添加一条路由，建议集群规模为1000节点及以下。

VPC网络采用VPC路由方式与底层网络深度整合，节点数量受限于VPC的路由配额。

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0281.html

召回章节：容器隧道网络模型说明/应用场景、容器网络模型对比/网络模型对比、容器隧道网络模型说明/优缺点、网络概述/集群网络构成、VPC网络模型说明/应用场景

评审：答案完整覆盖两个必答点：VPC网络无隧道封装且节点数量受VPC路由配额限制（并补充1000节点及以下建议），容器隧道网络使用VXLAN和Open vSwitch（并补充2000节点规模、5%-15%性能损耗等，均有召回证据支持）。未发现与权威证据或自身矛盾的断言，也未发现无依据的实质性断言。

证据复核（以绑定当前答案的复核为准）：VXLAN、VPC无封装及路由配额已答，但缺少必答点中的Open vSwitch。

### cce-manual-030 · 存储

已有云硬盘要静态导入CCE，VBD盘、已分区盘或非ext4盘能直接用吗？

参考要点：模式需SCSI不是默认VBD；不支持已分区盘；仅支持ext4；不能是系统盘、专属盘或共享盘

初步排查方向：召回证据齐全，检查生成中的条件遗漏、对象混淆或评审误判

实际回答：已有云硬盘模式需选择SCSI，购买云硬盘时默认为VBD模式，即VBD盘不满足条件。

不支持使用已进行分区的云硬盘。

仅支持使用ext4类型的云硬盘，不支持非ext4文件系统的云硬盘。

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0614.html

召回章节：通过静态存储卷使用已有云硬盘/前提条件、使用kubectl部署带云硬盘存储卷的工作负载/操作场景、云硬盘存储卷概述/使用说明、云硬盘概述/使用场景、通过静态存储卷使用已有云硬盘/通过静态存储卷使用已有云硬盘

评审：答案正确指出VBD盘不满足（需SCSI）、不支持已分区盘、仅支持ext4，但未提及'不能是系统盘、专属盘或共享盘'这一必答点，因此第4项为false。其余三项均完整满足。答案直接回答了问题，无矛盾或无依据断言。

证据复核（以绑定当前答案的复核为准）：遗漏系统盘、专属盘、共享盘不支持的限制，SCSI/分区/ext4三项正确仍不能整题通过。

### cce-manual-051 · AI容器

Kthena ModelServing要补足Deployment/StatefulSet处理LLM推理时的哪些局限？

参考要点：拓扑感知；原子调度；复杂推理工作流编排；为PD分离等模式提供声明式生命周期管理

初步排查方向：召回证据齐全，检查生成中的条件遗漏、对象混淆或评审误判

实际回答：Kthena ModelServing的核心目标是突破Kubernetes原生负载（如Deployment/StatefulSet）在拓扑感知、原子调度及复杂推理工作流编排上的局限，为PD分离等高级模式构建坚实底座。

Kubernetes原生资源（如Deployment或StatefulSet）在处理多角色协作场景下存在局限性，Kthena采用ModelServing → ServingGroup → Role的三层结构来应对。

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_1123.html

召回章节：Kthena ModelServing/架构设计、Kthena ModelServing/Kthena ModelServing、Kthena ModelServing/Eviction保护（Eviction Protection）、Kthena ModelServing/作业调度（Gang Scheduling）、部署推理负载/简介

评审：答案完整覆盖四个必答点：拓扑感知、原子调度、复杂推理工作流编排、为PD分离等模式提供声明式生命周期管理，均与原文一致。额外提到的三层结构由召回上下文支持，无矛盾或无依据断言。

证据复核（以绑定当前答案的复核为准）：三类原生负载局限及PD分离已给出，但遗漏声明式生命周期管理；LLM漏判。

### cce-manual-052 · AI容器

CCE AI应用预置模板能免去手写YAML吗，主要预置了哪些模型部署配置？

参考要点：无需编写复杂Kubernetes YAML；预置模型运行参数、NPU资源需求及硬件适配；通过可视化界面选模板配置

初步排查方向：召回证据齐全，检查生成中的条件遗漏、对象混淆或评审误判

实际回答：CCE AI应用预置模板可以免去手写YAML：该工具开箱即用，无需编写复杂的Kubernetes YAML文件，参数已针对昇腾（Ascend）底层硬件进行深度适配与优化。

底层已内置针对主流AI模型（如DeepSeek-R1）的预定义配置，涵盖运行参数、NPU资源需求及硬件适配。

模板中已适配常见模型的参数、资源需求及硬件配置；用户无需手动编写复杂配置文件，可通过声明式API直接调用模板并快速部署模型。

预置模板覆盖的模型包括 DeepSeek-R1-Distill-Qwen-7B、DeepSeek-R1-Distill-Llama-8B、DeepSeek-R1-Distill-Qwen-14B、DeepSeek-R1-Distill-Qwen-32B、DeepSeek-R1-Distill-Llama-70B、QwQ-32B、Qwen2.5-Coder-32B-Instruct、Qwen2.5-32B-Instruct、Qwen2.5-72B-Instruct，NPU资源类型均为 huawei.com/ascend-1980。

预置模板的部署配置涵盖容器镜像、环境变量（如 HF_HUB_OFFLINE、INFER_MODE 等）、推理启动参数（如 --model、--max-num-seqs、--max-model-len、--gpu-memory-utilization 等）、CPU/内存/NPU 资源 limits 与 requests、容器端口、模型存储卷挂载（PVC），以及 readinessProbe/livenessProbe 健康检查。

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_1154.html

召回章节：部署AI应用模板/简介、AI推理框架插件/模型模板、部署AI应用模板/操作步骤、部署AI应用模板/核心优势、部署AI应用模板/前提条件

评审：答案完整覆盖三个必答点：明确说明无需编写复杂Kubernetes YAML；列出预置配置涵盖运行参数、NPU资源需求及硬件适配；并指出可通过可视化界面选模板配置。答案中关于模型列表、NPU资源类型及部署配置细节（镜像、环境变量、启动参数、资源limits/requests、端口、PVC挂载、健康检查）均与召回原文一致，无矛盾或无依据的实质性断言。

证据复核（以绑定当前答案的复核为准）：免手写YAML及预置运行参数/NPU/硬件配套已答，未说明通过可视化界面选择模板配置；LLM漏判。

### cce-manual-055 · 命名空间

想让开发、联调、测试环境共享同一CCE集群但逻辑隔离，可用什么方式组织？

参考要点：为不同环境建立对应命名空间；创建和查询工作负载时选择对应命名空间；不要宣称这等同于完整网络安全隔离

初步排查方向：召回未覆盖全部参考证据，检查分段、标题上下文和候选排序

实际回答：可以使用命名空间（Namespace）实现同一CCE集群内的逻辑隔离：在同一个集群内可创建不同的命名空间，不同命名空间中的数据彼此隔离，使它们既共享同一集群的服务，又互不干扰。

可按不同环境划分命名空间，将开发环境、联调环境、测试环境分别创建对应的命名空间。

同一命名空间下，通过服务名称（Service name）可直接访问；跨命名空间的可以通过服务名称、命名空间名称访问。

作为对比，分别创建不同集群的做法下，不同集群之间资源不能共享，且不同环境中的服务互访需要通过负载均衡才能实现。

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0285.html

召回章节：使用FlexNPU实现NPU资源虚拟化与隔离/前提条件、创建命名空间/操作场景、管理命名空间/命名空间使用实践、修改CCE集群配置/网络组件配置（canal-controller）（仅容器隧道网络模型的集群支持）、Volcano队列/Volcano队列

评审：答案正确给出核心方式：为开发、联调、测试环境分别创建对应命名空间，满足必答点1。但必答点2要求说明创建和查询工作负载时选择对应命名空间，答案未提及创建/查询工作负载时选择命名空间，故不满足。必答点3要求不要宣称这等同于完整网络安全隔离，答案未作此宣称，但也未明确说明命名空间隔离不等于完整网络安全隔离，因此该限定点未完整满足。答案中关于同命名空间/跨命名空间服务访问方式以及不同集群互访需负载均衡的表述，在本次召回证据中无对应支持，属于无依据断言。

证据复核（以绑定当前答案的复核为准）：环境分命名空间正确，但遗漏创建和查询工作负载时选择对应命名空间。答案未声称完整网络安全隔离，该禁止项本应通过；不影响因缺事实而整题失败。

### cce-manual-068 · 模板（Helm Chart）

Helm v3相较v2是否还需要Tiller，它如何连接apiserver？

参考要点：移除Tiller；直接通过kubeconfig连接apiserver；简化架构与安全模块

初步排查方向：召回证据齐全，检查生成中的条件遗漏、对象混淆或评审误判

实际回答：Helm v3移除了tiller，直接通过kubeconfig连接apiserver。

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0421.html

召回章节：Helm v2与Helm v3的差异及适配方案/Helm v2与Helm v3的差异及适配方案、通过Helm v2客户端部署应用/安装Helm v2、通过Helm v2客户端部署应用/通过Helm v2客户端部署应用、Helm v2 Release转换成Helm v3 Release/转换流程（使用Helm v3客户端）、Helm v2 Release转换成Helm v3 Release/背景介绍

评审：答案正确指出Helm v3移除了tiller并直接通过kubeconfig连接apiserver，满足前两个必答点。但未提及'简化安全模块'这一必答点，因此第三项为false。答案无矛盾或无依据的断言，且直接回答了问题。

证据复核（以绑定当前答案的复核为准）：已说明移除Tiller与kubeconfig，遗漏架构/安全模块简化的必答点。

### cce-manual-069 · 模板（Helm Chart）

后台用Helm v2删除实例后，CCE模板管理仍显示实例信息，文档建议怎么办？

参考要点：CCE会尝试把v2实例转换为v3；在CCE模板管理页面再单击删除

初步排查方向：召回证据齐全，检查生成中的条件遗漏、对象混淆或评审误判

实际回答：若在后台操作Helm v2模板实例，删除实例后发现CCE模板管理页面仍有实例信息，单击删除即可。

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0420.html

召回章节：通过Helm v2客户端部署应用/注意事项、通过模板部署应用/通过模板部署应用、Helm v2 Release转换成Helm v3 Release/注意事项：、通过Helm v2客户端部署应用/通过Helm v2客户端部署应用、通过模板部署应用/卸载通过模板部署的应用

评审：必答点1要求说明CCE会尝试把v2实例转换为v3，答案未提及该转换机制，仅复述了删除操作，故不满足。必答点2要求在CCE模板管理页面再单击删除，答案明确给出“单击删除即可”，满足。答案直接回答了问题，无矛盾或无依据断言。

证据复核（以绑定当前答案的复核为准）：删除入口正确，但未解释CCE会尝试将v2实例转换为v3这一必答机制。

### cce-manual-071 · 权限

旧版IAM中一个账号要给不同部门分别设置CCE权限，可以如何组织身份？

参考要点：给部门员工创建有唯一安全凭证的IAM用户；按用户职能配置不同权限以隔离；可委托其他账号或云服务代运维

初步排查方向：召回证据齐全，检查生成中的条件遗漏、对象混淆或评审误判

实际回答：旧版IAM（角色与策略授权）下，可根据企业的业务组织，在账号中给不同职能部门的员工创建IAM用户，让员工拥有唯一安全凭证并使用CCE资源。

旧版IAM下，可根据企业用户的职能设置不同的访问权限，以达到用户之间的权限隔离。

CCE权限管理支持基于IAM的细粒度权限控制和IAM Token认证，支持集群级别、命名空间级别的权限控制，可对租户下的IAM用户、用户组设定不同的操作权限。

集群权限仅针对与集群相关的资源（如集群、节点等）有效，须同时配置命名空间权限，才能有操作Kubernetes资源（如工作负载、Service等）的权限。

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0732.html

召回章节：身份策略授权（新版IAM）/身份策略授权（新版IAM）、CCE权限概述/联邦用户支持说明、角色与策略授权（旧版IAM）/角色与策略授权（旧版IAM）、CCE权限概述/CCE权限概述、身份策略授权（新版IAM）/前提条件

评审：答案覆盖了必答点1（创建IAM用户、唯一安全凭证）和必答点2（按职能配置不同权限以隔离），但未提及必答点3（可委托其他账号或云服务代运维），因此facts_met第三项为false。答案中关于细粒度权限控制、IAM Token认证、集群/命名空间级别权限控制以及集群权限须同时配置命名空间权限的表述，在提供的召回证据中无对应支持，属于unsupported_claims。答案直接回应了问题，无与证据矛盾的断言。

证据复核（以绑定当前答案的复核为准）：用户/安全凭据及职能权限隔离正确，遗漏委托其他账号或云服务代运维。权限补充其实可在实际召回中核对，不属于无依据断言。

### cce-manual-080 · 存储管理-Flexvolume（已弃用）

已弃用Flexvolume云硬盘文档中，两个Pod用同一EVS却调度到不同节点会怎样？

参考要点：云硬盘非共享，不能同时被多个节点挂载；必有一个Pod无法挂载而不能启动；需指出这是历史Flexvolume文档范围

初步排查方向：召回证据齐全，检查生成中的条件遗漏、对象混淆或评审误判

实际回答：若两个Pod配置了使用同一个云硬盘，当这两个Pod被调度到不同的节点时，必然有一个Pod会因为无法挂载云硬盘导致无法成功启动。

多个节点挂载使用同一个云硬盘可能会出现读写冲突、数据缓存冲突等问题，建议工作负载只选择一个实例。

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0310.html

召回章节：通过静态存储卷使用已有云硬盘/约束与限制、通过动态存储卷使用云硬盘/约束与限制、高危操作一览/云硬盘、通过静态存储卷使用已有云硬盘/验证数据持久化、云硬盘存储卷概述/使用说明

评审：答案完整复述了云硬盘非共享、不能同时被多个节点挂载，以及两个Pod调度到不同节点时必有一个Pod无法挂载而无法启动这两个必答点。但答案未指出该结论属于历史Flexvolume文档范围，因此第三个必答点未满足。答案中关于读写冲突、数据缓存冲突及建议单实例的内容有召回证据支持，无矛盾或无依据断言。

证据复核（以绑定当前答案的复核为准）：两个Pod跨节点共用EVS的风险正确，但没有标明历史Flexvolume资料范围。

### cce-manual-081 · 存储管理-Flexvolume（已弃用）

历史Flexvolume示例pvc-evs-auto-example.yaml申请容量与访问模式分别是什么，可用区有什么要求？

参考要点：10Gi；ReadWriteOnce；可用区必须与工作负载规划一致；仅说明历史Flexvolume示例，不作为当前CSI YAML

初步排查方向：召回未覆盖全部参考证据，检查分段、标题上下文和候选排序

实际回答：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0312.html

召回章节：使用kubectl对接已有对象存储/操作步骤、使用kubectl自动创建对象存储/操作步骤、使用kubectl自动创建文件存储/操作步骤、使用kubectl对接已有文件存储/操作步骤、1.15集群如何从Flexvolume存储类型迁移到CSI Everest存储类型/操作步骤（第1部分）

评审：答案为空字符串，未提供任何信息。required_facts中的四个必答点（10Gi、ReadWriteOnce、可用区必须与工作负载规划一致、仅说明历史Flexvolume示例不作为当前CSI YAML）均未满足。虽然gaps中声明资料不足，但reference_evidence明确包含pvc-evs-auto-example.yaml的容量10Gi、访问模式ReadWriteOnce以及可用区要求，因此该问题实际可回答，不属于正确弃答。

证据复核（以绑定当前答案的复核为准）：无答案，容量10Gi、ReadWriteOnce、可用区一致及历史范围均缺失；目标原文未进入召回。

### cce-manual-084 · 高危操作一览

重装CCE控制节点和Node节点操作系统后，手册给出的恢复结论相同吗？

参考要点：控制节点重装后组件删除，不可恢复；Node节点重装后可按文档重置节点

初步排查方向：召回未覆盖全部参考证据，检查分段、标题上下文和候选排序

实际回答：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0054.html

召回章节：节点限制检查异常处理/解决方案、节点自愈功能/节点重启后故障未恢复、重置节点/操作场景、移除节点/约束限制、节点操作系统说明/操作系统内核更新信息

评审：答案为空，未给出任何结论，因此两个必答点均未满足。但已召回资料确实不包含控制节点重装后不可恢复或Node节点重装后可按文档重置节点的权威内容，答案正确指出资料不足，未编造数据，属于正确弃答。

证据复核（以绑定当前答案的复核为准）：无答案，控制节点不可恢复与Node重置节点两条均未回答，实际召回缺关键表格。

### cce-manual-087 · 备份中心

删除备份时报backup storage location is currently in read-only mode，应改哪个字段为什么值？

参考要点：检查仓库访问模式为只读；用kubectl把spec.accessMode改为ReadWrite；再重试删除

初步排查方向：召回证据齐全，检查生成中的条件遗漏、对象混淆或评审误判

实际回答：可通过kubectl命令编辑备份仓库的spec.accessMode字段，将其修改为“ReadWrite”后重试删除。

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_1164.html

召回章节：备份中心常见问题/索引、备份中心常见问题/备份记录删除失败提示：backup storage location xxx is currently in read-only mode、CCE集群备份恢复（停止维护）/创建存储库、CCE集群备份恢复（停止维护）/删除备份、备份中心常见问题/备份/恢复记录长时间处于删除中

评审：必答点1要求检查仓库访问模式为只读，答案未提及检查步骤，故不满足；必答点2要求用kubectl把spec.accessMode改为ReadWrite，答案完整满足；必答点3要求再重试删除，答案完整满足。答案无与证据矛盾或未支持的实质性断言，且直接回答了应改字段及目标值。

证据复核（以绑定当前答案的复核为准）：字段spec.accessMode=ReadWrite和重试删除正确，但未说明先检查只读访问模式。

### cce-manual-088 · 备份中心

Velero仓库提示invalid top-level directories，OBS桶根目录只允许哪些目录？

参考要点：只允许restores和backups；清理根目录其他文件或文件夹后可恢复；不声称已执行删除

初步排查方向：召回证据齐全，检查生成中的条件遗漏、对象混淆或评审误判

实际回答：Velero备份仓库对应OBS桶的根目录中禁止包含除restores、backups之外的文件或文件夹。

备份数据在桶中的子目录禁止包含除restores、backups之外的文件或文件夹，避免备份恢复任务失败或造成已有数据覆盖。

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_1164.html

召回章节：备份中心常见问题/备份仓库状态不可用提示：Backup store contains invalid top-level directories: [xxx]、集群内备份和恢复应用/前提条件、集群内备份和恢复应用/创建备份仓库、节点关键目录文件权限检查异常处理/解决方案、备份中心常见问题/索引

评审：答案正确指出OBS桶根目录只允许restores和backups，满足必答点1；但未提及清理根目录其他文件或文件夹后可恢复，必答点2不满足；答案未声称已执行删除，满足必答点3。答案中关于子目录的表述虽在召回文档中有依据，但并非针对问题所问的根目录，属于超出问题范围的额外断言，列为unsupported_claims。答案直接回答了问题，无矛盾。

证据复核（以绑定当前答案的复核为准）：允许目录正确，但遗漏清理其他根目录文件后恢复这一必答点。子目录补充在实际召回中有依据，LLM的unsupported理由不成立。

### cce-manual-092 · 弹性伸缩

VPA建议值与Pod当前资源值不同，哪个组件重建Pod，哪个组件在重建时修改资源请求？

参考要点：VPA Updater对比并重建；VPA Admission Controller拦截重建并修改资源请求；Recommender负责给建议

初步排查方向：召回证据齐全，检查生成中的条件遗漏、对象混淆或评审误判

实际回答：VPA Updater 对比建议值和当前值，不一致时重建 Pod。

VPA Admission Controller 在 Pod 重建时进行拦截，将 Pod 的资源申请量（资源请求）修改为建议值。

vpa-updater 驱逐实际资源申请量与 VPA 建议值有偏差的 Pod，触发 Pod 重建以使得资源建议值生效至新建的 Pod。

vpa-admission-controller 在 Pod 创建时，将容器的资源申请量调整为 VPA 生成的建议值。

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0290.html

召回章节：工作负载伸缩原理/VPA工作原理、创建VPA策略/功能概述、创建VPA策略/注意事项、容器垂直弹性引擎/功能概述、容器垂直弹性引擎/组件说明

评审：答案完整覆盖三个必答点：VPA Updater对比建议值与当前值不一致时重建Pod；VPA Admission Controller在Pod重建时拦截并将资源申请量修改为建议值；Recommender负责根据历史数据给出建议。与原文一致，无矛盾或无依据断言。

证据复核（以绑定当前答案的复核为准）：Updater及Admission Controller正确，未提及Recommender；LLM解释错误声称答案已有Recommender。

### cce-manual-093 · 模板（Helm Chart）

Helm v3 chart的crds目录，升级release会更新CRD吗，删除release会删CRD吗？

参考要点：crds目录仅安装release时部署；升级不更新；删除不卸载；推荐分离CRD和使用CRD资源的chart管理

初步排查方向：召回未覆盖全部参考证据，检查分段、标题上下文和候选排序

实际回答：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0421.html

召回章节：Helm v2 Release转换成Helm v3 Release/Helm v2 Release转换成Helm v3 Release、Helm v2 Release转换成Helm v3 Release/注意事项：、Helm v2 Release转换成Helm v3 Release/背景介绍、Helm v2 Release转换成Helm v3 Release/转换流程（使用Helm v3客户端）、Helm v2 Release转换成Helm v3 Release/转换流程（不使用Helm v3客户端）

评审：答案为空白，未提供任何事实性内容，因此四个必答点均未满足。答案未直接回答问题，但状态为UNRESOLVED且gaps指出已召回资料不足以回答，属于正确指出具体缺口、未编造数据，故correct_abstention为true。检索到的上下文均为Helm v2转v3操作指南，未包含crds目录升级/删除行为及chart管理建议，无法支持回答。

证据复核（以绑定当前答案的复核为准）：无答案，实际召回未包含CRD的安装、升级、删除规则与拆分chart建议。

### cce-manual-095 · 模板（Helm Chart）

《模板概述》和《通过模板部署应用》对CCE控制台Helm版本说法是否一致？能只凭这些资料确定唯一版本吗？

参考要点：模板概述写v3.8.2；通过模板部署应用写v3.18.6；明确存在资料冲突；不能据此断言所有区域唯一版本，需要核对当前控制台或权威版本说明

初步排查方向：召回证据齐全，检查生成中的条件遗漏、对象混淆或评审误判

实际回答：《模板概述》称CCE使用的Helm版本为v3.8.2，支持上传Helm v3语法的模板包。

《通过模板部署应用》的约束与限制称CCE使用的Helm版本为v3.18.6，支持上传Helm v3版本语法的模板包。

《通过模板部署应用》正文仅说明各region将逐步切换至Helm v3，模板管理不再支持Helm v2版本的模板，未给出具体Helm版本号。

两份资料对CCE控制台Helm版本的说法不一致：《模板概述》为v3.8.2，而《通过模板部署应用》的约束与限制为v3.18.6。

仅凭这些资料无法确定唯一的Helm版本号，因为所给片段对具体版本号存在相互矛盾的描述。

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0146.html、https://support.huaweicloud.com/usermanual-cce/cce_10_0191.html

召回章节：模板概述/模板概述、通过模板部署应用/通过模板部署应用、通过模板部署应用/约束与限制、通过Helm v2客户端部署应用/通过Helm v2客户端部署应用、通过模板部署应用/创建模板实例

评审：答案逐项满足必答点：指出《模板概述》写v3.8.2、《通过模板部署应用》约束与限制写v3.18.6，明确两份资料说法不一致/存在冲突，并说明仅凭这些资料无法确定唯一版本号。未发现与证据矛盾或超出召回内容的实质性断言。

证据复核（以绑定当前答案的复核为准）：两个版本、冲突及无法唯一确定均答对，但未给出核对当前控制台或权威版本说明的下一步；按冻结必答点计未完整。

### cce-manual-096 · 存储

EVS静态PV回收策略Delete下，设置与不设置everest.io/reclaim-policy=retain-volume-only，删除PVC结果有何不同？

参考要点：不设置时PV和云硬盘都删除；设置retain-volume-only时PV删除、底层云硬盘保留；该参数要求Everest至少1.2.9且Delete策略

初步排查方向：召回未覆盖全部参考证据，检查分段、标题上下文和候选排序

实际回答：Delete且不设置everest.io/reclaim-policy：删除PVC，PV资源与存储均被删除。

Delete且设置everest.io/reclaim-policy=retain-volume-only：删除PVC，PV资源被删除，存储资源会保留。

PV回收策略设置为Delete，并添加annotations“everest.io/reclaim-policy: retain-volume-only”，这样在删除PVC时，PV会被删除，但底层存储资源会保留。

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0614.html

召回章节：存储基础知识/PV回收策略、通过静态存储卷使用已有对象存储/使用已有对象存储（第2部分）、通过静态存储卷使用已有文件存储/通过kubectl命令行使用已有文件存储（第2部分）、自定义存储类（StorageClass）/StorageClass高级配置（第3部分）、自定义存储类（StorageClass）/StorageClass基础配置

评审：答案正确说明不设置时PV与云硬盘均删除、设置retain-volume-only时PV删除而底层云硬盘保留，直接回答了差异；但未提及该参数要求Everest插件版本>=1.2.9且回收策略为Delete时生效，因此第三个必答点未满足。答案未与证据矛盾，也无无依据的实质性断言。

证据复核（以绑定当前答案的复核为准）：Delete及retain-volume-only的差异正确，缺Everest至少1.2.9的版本门槛。

### cce-manual-097 · 存储

EVS的Retain回收策略删除PVC后，原PV能直接绑定另一个PVC吗？

参考要点：PV和底层存储均保留；PV变Released不能直接再绑定；若复用底层存储，需删除PV后新建PV/PVC与原存储关联

初步排查方向：召回证据齐全，检查生成中的条件遗漏、对象混淆或评审误判

实际回答：Retain策略下，PVC删除后PV资源状态为“已释放（Released）”，且不能直接再次被PVC绑定使用。

若希望继续使用该底层存储资源，需先删除PV，再新建PV、PVC和底层存储关联。

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0614.html

召回章节：存储基础知识/PV回收策略、通过静态存储卷使用已有对象存储/使用已有对象存储（第2部分）、通过静态存储卷使用已有文件存储/约束与限制、自定义存储类（StorageClass）/StorageClass高级配置（第3部分）、通过静态存储卷使用已有文件存储/通过kubectl命令行使用已有文件存储（第2部分）

评审：必答点1要求说明PV和底层存储均保留，答案未提及底层存储资源保留，仅说PV状态为Released，故不完整；必答点2和3均被答案准确覆盖。答案直接回答了问题，无矛盾或无依据断言。

证据复核（以绑定当前答案的复核为准）：Released、不可直接绑定及重建PV/PVC正确，但未明确PV与底层存储均保留；按冻结完整性口径计未完整。

### cce-negative-runtime-nodes · 节点

请直接告诉我生产集群prod-7现在的CPU使用率百分比。

参考要点：没有prod-7实时监控数据，不能给出实际CPU百分比。

初步排查方向：召回未覆盖全部参考证据，检查分段、标题上下文和候选排序

实际回答：

证据来源：

召回章节：Node视图/指标说明、节点CPU使用率检查异常处理/解决方案、节点CPU使用率检查异常处理/检查项内容、节点池视图/指标说明、创建CustomedHPA策略/创建CustomedHPA策略

评审：答案为空，未给出prod-7的CPU百分比。已召回资料仅包含CCE指标说明和检查项，无prod-7实时监控数据，因此无法回答；答案未编造数据，属于正确弃答。

证据复核（以绑定当前答案的复核为准）：没有编造CPU，但仅返回统一资料不足提示，没有说明缺少prod-7实时监控及无法确认实际百分比。

### cce-negative-runtime-backup · 备份中心

昨晚我在生产集群做的Velero备份成功了吗？请确认最后一次备份的真实状态。

参考要点：缺少该集群备份记录，不能确认真实结果。

初步排查方向：召回未覆盖全部参考证据，检查分段、标题上下文和候选排序

实际回答：

证据来源：

召回章节：Velero/Velero、CCE集群备份恢复（停止维护）/周期备份、CCE集群备份恢复（停止维护）/立即备份、集群内备份和恢复应用/创建备份计划或立即备份、Velero/使用说明

评审：答案为空，未直接回答备份是否成功，但问题本身不可回答：召回资料仅包含Velero插件介绍、备份配置模板和查看备份状态的通用命令，没有任何该用户生产集群的具体备份记录或状态数据，因此无法确认真实结果。答案正确指出资料不足以回答，未编造数据，符合正确拒答。

证据复核（以绑定当前答案的复核为准）：没有编造备份结果，但未指出缺少该集群备份记录，未给出具体缺口。

### cce-negative-runtime-security · 权限

告诉我当前租户default-secret里保存的真实密码值。

参考要点：手册不包含租户真实Secret，不能给出或编造密码。

初步排查方向：召回未覆盖全部参考证据，检查分段、标题上下文和候选排序

实际回答：

证据来源：

召回章节：集群系统密钥说明/default-secret、使用KMS进行Secret落盘加密/Secret落盘加密介绍、创建密钥/Secret资源文件配置示例、使用KMS进行Secret落盘加密/使用KMS进行Secret落盘加密、CCE密钥管理（对接 DEW）/约束与限制

评审：答案为空，未给出或编造密码值，符合手册不包含租户真实Secret、不能给出或编造密码的必答点；正确指出资料不足以回答，属于正确弃答。

证据复核（以绑定当前答案的复核为准）：未泄露或编造密码，但未说明静态手册不含该租户default-secret真实值。

### cce-negative-unprovided-cve · 集群

手册里CVE-2099-987654漏洞对应的CCE修复小版本是什么？

参考要点：没有该虚构漏洞及修复版本依据，不能编造版本。

初步排查方向：召回未覆盖全部参考证据，检查分段、标题上下文和候选排序

实际回答：

证据来源：

召回章节：CCE补丁版本发布记录/v1.19版本、CCE补丁版本发布记录/v1.23版本（第1部分）、CCE补丁版本发布记录/v1.34版本、CCE补丁版本发布记录/v1.33版本、CCE补丁版本发布记录/v1.28版本（第1部分）

评审：问题询问CVE-2099-987654对应的CCE修复小版本，该CVE编号为虚构（2099年），召回的所有CCE补丁版本发布记录中均无此漏洞记录。答案为空，未编造任何版本号，正确指出资料不足以回答，符合不可回答问题的正确弃答要求。

证据复核（以绑定当前答案的复核为准）：未编造版本，但统一提示未明确缺少所问CVE及修复版本依据。

### cce-negative-unprovided-quota · 命名空间

CCE enable-infinite-namespace-quota参数该设为多少才能取消所有配额？

参考要点：资料未定义该参数，不能编造配置值或取消配额方案。

初步排查方向：召回未覆盖全部参考证据，检查分段、标题上下文和候选排序

实际回答：

证据来源：

召回章节：修改CCE集群配置/扩展控制器配置（仅v1.21及以上版本集群支持）、Kubernetes原生配置/资源配额控制器（resource-quota-controller）配置、设置对象存储挂载参数/对象存储挂载参数、设置资源配额及限制/设置资源配额及限制、CCE AI套件（Ascend NPU）/开启RDMA开关

评审：答案为空，未编造任何配置值或取消配额方案。召回资料中仅定义了 enable-resource-quota 参数，未出现 enable-infinite-namespace-quota 参数，无法回答该参数应设为多少才能取消所有配额，符合 required_facts 中“资料未定义该参数，不能编造配置值或取消配额方案”的要求，属于正确拒答。

证据复核（以绑定当前答案的复核为准）：未编造配置值，但未指出文档没有定义所问参数。

### cce-negative-unprovided-extension · 插件

CCE的quantum-zero-loss插件安装步骤和官方最低版本是什么？

参考要点：手册未提供该插件，不应虚构安装步骤或版本。

初步排查方向：召回未覆盖全部参考证据，检查分段、标题上下文和候选排序

实际回答：

证据来源：

召回章节：CCE容器存储（Flexvolume，已废弃）/约束与限制、CCE AI套件（NVIDIA GPU）/验证插件、Gatekeeper/安装步骤、GPU虚拟化优先级抢占调度/前提条件、CCE容器存储（Flexvolume，已废弃）/安装插件

评审：召回资料中未出现 quantum-zero-loss 插件，答案为空且指出资料不足以回答，未虚构安装步骤或版本，符合正确拒答。

证据复核（以绑定当前答案的复核为准）：未虚构安装步骤，但未指出手册没有所问插件和版本资料。
