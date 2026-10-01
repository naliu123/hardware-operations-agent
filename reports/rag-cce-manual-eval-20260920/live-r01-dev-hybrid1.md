# live-r01-dev-hybrid1

模式：LIVE_ANSWERS

```json
{
  "completed": 75,
  "expected": 75,
  "errors": 0,
  "answer_accuracy": 0.6933333333333334,
  "llm_judged_answer_accuracy": 0.72,
  "evidence_recall_at_5": 0.9710144927536232,
  "all_evidence_at_5": 0.9565217391304348,
  "document_hit_at_5": 0.9565217391304348,
  "latency_p95_ms": 7688.092333002714,
  "generation_usage": {
    "calls": 75,
    "prompt_tokens": 264388,
    "completion_tokens": 13466,
    "total_tokens": 277854
  },
  "citation_integrity_rate": 1.0,
  "llm_judged_passed": 54,
  "wilson_95": [
    0.6095909606557163,
    0.8089697991462156
  ],
  "audit_status": "completed",
  "real_model_calls": 75,
  "llm_judged_answerable_accuracy": 0.782608695652174,
  "llm_judged_abstention_accuracy": 0.0,
  "audit_required_ids": [
    "cce-manual-001",
    "cce-manual-012",
    "cce-manual-030",
    "cce-manual-052",
    "cce-manual-055",
    "cce-manual-064",
    "cce-manual-068",
    "cce-manual-071",
    "cce-manual-080",
    "cce-manual-081",
    "cce-manual-082",
    "cce-manual-083",
    "cce-manual-088",
    "cce-manual-092",
    "cce-manual-096",
    "cce-negative-runtime-nodes",
    "cce-negative-runtime-backup",
    "cce-negative-runtime-security",
    "cce-negative-unprovided-cve",
    "cce-negative-unprovided-quota",
    "cce-negative-unprovided-extension",
    "cce-manual-002",
    "cce-manual-097",
    "cce-manual-074",
    "cce-manual-095",
    "cce-manual-098",
    "cce-manual-022",
    "cce-manual-013",
    "cce-manual-035",
    "cce-manual-006",
    "cce-manual-016",
    "cce-manual-025"
  ],
  "judge_usage": {
    "prompt_tokens": 372531,
    "completion_tokens": 12777,
    "total_tokens": 385308
  },
  "audited_passed": 52,
  "audited_count": 40,
  "audited_wilson_95": [
    0.5816933204791398,
    0.7861328017710428
  ],
  "answerable_accuracy": 0.7536231884057971,
  "abstention_accuracy": 0.0
}
```

准确率仅在真实回答、语义评审和证据复核完成后报告；检索指标不代表答案正确。

## Bad cases（23）

### cce-manual-001 · 集群

CCE Standard和Turbo的网络选择有什么区别，Turbo的Pod地址如何分配？

参考要点：Standard可选择容器隧道或VPC网络；Turbo采用云原生网络2.0；Turbo的Pod直接从VPC网段分配IP，可与节点在不同子网

初步排查方向：参考锚点已命中，仍需核对实际语义覆盖、条件遗漏、对象混淆或评审误判

实际回答：CCE Standard集群支持的容器网络模型包括VPC网络和容器隧道网络，VPC网络适用于集群规模较小（1000节点及以下）且性能要求较高的场景，容器隧道网络适用于集群规模大（最大可支持2000节点）且对性能要求相对较低的场景。

Standard集群的容器网段支持“手动设置网段”和“自动设置网段”两种配置方式，手动词可自定义容器网段，自动则由系统从172.(16~31).0.0/16、10.(0|16|32|48|64|80|96|112).0.0/12中随机分配与所选VPC及服务网段不冲突的网段。

CCE Standard集群（容器隧道网络）在v1.15及以上版本支持启用IPv6，v1.23版本进入GA；CCE Standard集群（VPC网络模型）暂不支持启用IPv6；CCE Turbo集群在v1.23.8-r0、v1.25.3-r0及以上版本支持启用IPv6。

集群创建后不支持修改容器网络模型，需要在创建前做好规划和选择。

CCE Standard集群的NodeLocal DNSCache地址为169.254.20.10，CCE Turbo集群的NodeLocal DNSCache地址为169.254.1.1。

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0430.html

召回章节：在CCE Turbo集群中为Pod配置固定IP/在CCE Turbo集群中为Pod配置固定IP、集群概述/集群的网络、在CCE Turbo集群中为Pod配置EIP/在CCE Turbo集群中为Pod配置EIP、使用节点本地域名解析加速（NodeLocal DNSCache）提升DNS性能/使用NodeLocal DNSCache、购买Standard/Turbo集群/步骤二：进行网络配置（第1部分）

评审：答案明确说明Standard可选容器隧道或VPC网络，并说明Turbo的Pod直接从VPC网段分配IP、Pod和节点可分属不同子网，满足第1、3个必答点。但答案未提及Turbo采用云原生网络2.0，因此第2个必答点未满足。答案未出现与证据矛盾的断言，也未发现无依据的实质性补充。

证据复核（以绑定当前答案的复核为准）：只回答Standard网络选项，未回答Turbo云原生网络2.0、Pod从VPC分配及不同子网。LLM解释误称答案已包含Pod分配，实际答案没有，不能信任其fact_support。

### cce-manual-009 · 节点

为什么不能给所有区域的CCE Turbo节点承诺同一个可用规格和最大Pod数？

参考要点：区域规格可能新增、售罄或下线，需看创建节点页面；Turbo最大Pod数量与节点可用弹性网卡/辅助弹性网卡数量有关

初步排查方向：未命中全部参考锚点，核对等价证据、分段及候选排序

实际回答：CCE Turbo集群节点可创建的Pod数取决于节点最大实例数和节点网卡数量中的最小值，而不同规格支持的网卡数量可能不同，因此不能承诺同一可用规格和最大Pod数。

CCE Turbo集群节点可以创建的最大Pod数量与节点可使用网卡数量相关（ECS节点使用弹性辅助网卡，裸金属节点使用弹性网卡）。

相同CPU和内存规格的节点，可创建的Pod数也可能不一样，因为节点可创建的Pod数并非由节点规格单一因素决定，而是根据集群类型和其他参数共同作用的结果。

如果节点支持的网卡数量大于节点最大实例数，需要同步修改节点最大实例数，其值建议和网卡数量保持一致。

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0719.html

召回章节：节点可创建的最大Pod数量说明/常见问题、节点可创建的最大Pod数量说明/如何提升集群中可使用Pod数量、节点可创建的最大Pod数量说明/节点网卡数量说明（仅CCE Turbo集群）、监控中心FAQ/索引、节点可创建的最大Pod数量说明/节点最大Pod数量计算方式

评审：答案完整覆盖两个必答点：第一点明确说明区域规格存在新增、售罄下线等情况，并建议在创建节点页面查看；第二点明确说明Turbo最大Pod数量与节点可用弹性网卡/辅助弹性网卡数量相关。答案中关于Pod数取节点最大实例数与网卡数量最小值、相同CPU内存规格Pod数可能不同、网卡数大于最大实例数时需同步修改等补充内容，均可在召回上下文中找到依据，无矛盾或无依据断言。

证据复核（以绑定当前答案的复核为准）：只有网卡/实例数约束，没有区域规格新增、售罄或下线及在创建节点页面查看的要求；实际Top5缺对应原文。

### cce-manual-012 · 节点池

节点池默认规格A为0.0993USD/小时，但手动新增10台规格B为0.1923USD/小时，新增节点一小时按哪个价格算，总额多少？

参考要点：以实际创建的B规格计费；新增10台一小时为1.923USD；控制台展示默认A价格不改变实际计费

初步排查方向：参考锚点已命中，仍需核对实际语义覆盖、条件遗漏、对象混淆或评审误判

实际回答：节点池创建或扩缩容过程中，以实际创建的规格进行计费。

手动扩容节点池时新增10个节点并指定节点规格为B，使用1小时的计费为0.1923×10 = 1.923USD。

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0648.html

召回章节：节点池多规格计费说明/计费样例、节点池多规格计费说明/节点池多规格计费说明、更新节点池/更新节点池（第1部分）、更新节点池/注意事项、节点池概述/节点池概述

评审：答案正确说明以实际创建的B规格计费，并给出新增10台一小时总额1.923USD。但未提及控制台展示默认A价格不改变实际计费这一必答点，因此第三项不满足。

证据复核（以绑定当前答案的复核为准）：实际B规格及1.923USD计算正确，但未说明控制台默认A价格仅为展示；按冻结第三必答点计遗漏。

### cce-manual-024 · 网络

CCE的VPC网络与容器隧道网络，在封装和节点规模方面有何区别？

参考要点：VPC网络无隧道封装但节点数量受VPC路由配额限制；容器隧道网络使用VXLAN和Open vSwitch

初步排查方向：参考锚点已命中，仍需核对实际语义覆盖、条件遗漏、对象混淆或评审误判

实际回答：封装方面：容器隧道网络使用VXLAN隧道封装协议，并有约5%-15%的性能损耗；VPC网络没有额外隧道封装，跨节点通过VPC路由器转发，性能较好，但存在NAT转换损耗。

节点规模方面：容器隧道网络最大可支持2000节点规模，没有网络基础设施配额限制；VPC网络受限于VPC路由表能力，适合中小规模组网，建议规模为1000节点及以下。

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0281.html

召回章节：容器隧道网络模型说明/应用场景、容器网络模型对比/网络模型对比、容器隧道网络模型说明/优缺点、容器隧道网络模型说明/容器隧道网络模型、容器网络模型对比/容器网络模型对比

评审：答案完整覆盖两个必答点：VPC网络无隧道封装且节点数量受VPC路由配额限制（表述为受限于VPC路由表能力，适合中小规模组网，建议1000节点及以下），容器隧道网络使用VXLAN和Open vSwitch（答案虽未提Open vSwitch，但必答点要求为“使用VXLAN和Open vSwitch”，答案仅提VXLAN，未完整满足该点，故第二项判false）。答案中关于性能损耗、NAT转换损耗、2000节点等补充信息均有检索上下文支持，无矛盾或未支持断言。

证据复核（以绑定当前答案的复核为准）：VPC无封装及路由限制、VXLAN已答，但没有Open vSwitch；该事实实际已召回，属于生成遗漏。

### cce-manual-030 · 存储

已有云硬盘要静态导入CCE，VBD盘、已分区盘或非ext4盘能直接用吗？

参考要点：模式需SCSI不是默认VBD；不支持已分区盘；仅支持ext4；不能是系统盘、专属盘或共享盘

初步排查方向：参考锚点已命中，仍需核对实际语义覆盖、条件遗漏、对象混淆或评审误判

实际回答：已有的云硬盘不可以是VBD模式，需选择SCSI模式（购买云硬盘时默认为VBD模式）。

不支持使用已进行分区的云硬盘。

仅支持使用ext4类型的云硬盘，不支持非ext4文件系统的云硬盘。

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0614.html

召回章节：通过静态存储卷使用已有云硬盘/前提条件、使用kubectl部署带云硬盘存储卷的工作负载/操作场景、使用kubectl对接已有云硬盘/操作步骤（第1部分）、通过静态存储卷使用已有云硬盘/通过静态存储卷使用已有云硬盘、云硬盘存储卷概述/使用说明

评审：答案正确回答了VBD盘、已分区盘、非ext4盘均不能直接使用，分别对应SCSI模式要求、不支持已分区盘、仅支持ext4。但未提及云硬盘不能是系统盘、专属盘或共享盘这一必答点，因此第4项不满足。

证据复核（以绑定当前答案的复核为准）：SCSI、未分区和ext4三点正确，遗漏非系统盘/非专属盘/非共享盘，实际前提条件片段包含这些限制。

### cce-manual-051 · AI容器

Kthena ModelServing要补足Deployment/StatefulSet处理LLM推理时的哪些局限？

参考要点：拓扑感知；原子调度；复杂推理工作流编排；为PD分离等模式提供声明式生命周期管理

初步排查方向：参考锚点已命中，仍需核对实际语义覆盖、条件遗漏、对象混淆或评审误判

实际回答：Kthena ModelServing的核心目标是突破Kubernetes原生负载（如Deployment/StatefulSet）在拓扑感知、原子调度及复杂推理工作流编排上的局限，为PD分离等高级模式构建坚实底座。

为解决Kubernetes原生资源（如Deployment或StatefulSet）在处理多角色协作场景下的局限性，Kthena采用ModelServing → ServingGroup → Role的三层结构。

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_1123.html

召回章节：Kthena ModelServing/Kthena ModelServing、Kthena ModelServing/Eviction保护（Eviction Protection）、Kthena ModelServing/架构设计、LeaderWorkerSet插件/LeaderWorkerSet插件、Kthena ModelServing/作业调度（Gang Scheduling）

评审：答案逐项覆盖了必答点：拓扑感知、原子调度、复杂推理工作流编排，以及为PD分离等模式提供声明式生命周期管理（原文表述为“提供声明式的生命周期管理……为PD分离等高级模式构建坚实底座”）。答案中关于三层结构的补充内容在检索上下文中也有依据，未发现与权威证据矛盾或缺乏支持的实质性断言。

证据复核（以绑定当前答案的复核为准）：三类原生负载局限与PD分离已答，没有声明式生命周期管理；与R00同类遗漏。

### cce-manual-052 · AI容器

CCE AI应用预置模板能免去手写YAML吗，主要预置了哪些模型部署配置？

参考要点：无需编写复杂Kubernetes YAML；预置模型运行参数、NPU资源需求及硬件适配；通过可视化界面选模板配置

初步排查方向：参考锚点已命中，仍需核对实际语义覆盖、条件遗漏、对象混淆或评审误判

实际回答：CCE AI应用预置模板可以免去手写YAML配置文件：用户无需手动编写复杂配置文件，可通过声明式API直接调用模板并快速部署模型。

其开箱即用特性明确无需编写复杂的Kubernetes YAML文件，参数已针对昇腾（Ascend）底层硬件进行深度适配与优化。

AI应用模板是基于华为云CCE的“AI推理框架插件”打造的便捷化模型部署工具，预置底层内置针对主流AI模型（如DeepSeek-R1）的预定义配置，涵盖运行参数、NPU资源需求及硬件适配。

模板中已适配常见模型的参数、资源需求及硬件配置，能够有效降低部署的门槛。

预置模板覆盖的主流模型包括：DeepSeek-R1-Distill-Qwen-7B、DeepSeek-R1-Distill-Llama-8B、DeepSeek-R1-Distill-Qwen-14B、DeepSeek-R1-Distill-Qwen-32B、DeepSeek-R1-Distill-Llama-70B、QwQ-32B、Qwen2.5-Coder-32B-Instruct、Qwen2.5-32B-Instruct、Qwen2.5-72B-Instruct，对应的NPU资源类型均为 huawei.com/ascend-1980。

预置模板中已包含运行参数配置（如 --max-num-seqs、--max-model-len、--max-num-batched-tokens、--block-size、--gpu-memory-utilization 等）、资源需求配置（CPU、内存、huawei.com/ascend-1980 的 limits/requests）以及硬件适配（昇腾 NPU 资源、镜像 ascend-vllm-arm64）。

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_1154.html

召回章节：部署AI应用模板/简介、AI推理框架插件/模型模板、部署AI应用模板/操作步骤、部署AI应用模板/核心优势、AI容器概述/功能优势

评审：答案明确回答了免去手写YAML（fact1满足），并完整列出预置配置涵盖运行参数、NPU资源需求及硬件适配（fact2满足）。但第三个必答点要求“通过可视化界面选模板配置”，答案仅称“可通过声明式API直接调用模板”，未提及可视化界面选模板配置，反而给出与预置模板操作路径不符的API方式，故fact3不满足。答案中关于预置模型列表、NPU资源类型、具体参数和镜像等细节有检索上下文支持，但“声明式API直接调用模板”属于跨对象套用（来自AI推理框架插件文档），列为unsupported_claims。无与权威证据直接矛盾的断言。

证据复核（以绑定当前答案的复核为准）：免YAML及预置运行参数/NPU/硬件适配正确，遗漏可视化选模板，转而给出插件API入口；实际部署AI应用模板步骤和UI优势均已召回。

### cce-manual-055 · 命名空间

想让开发、联调、测试环境共享同一CCE集群但逻辑隔离，可用什么方式组织？

参考要点：为不同环境建立对应命名空间；创建和查询工作负载时选择对应命名空间；不要宣称这等同于完整网络安全隔离

初步排查方向：未命中全部参考锚点，核对等价证据、分段及候选排序

实际回答：可通过在同一CCE集群内为开发、联调、测试环境分别创建不同命名空间（Namespace）来实现资源共享下的逻辑隔离。

同一命名空间内的工作负载可通过服务名称（Service name）直接访问，跨命名空间则通过服务名称加命名空间名称访问。

按环境划分命名空间时，可选择分别创建不同集群（资源不能共享，跨环境互访需负载均衡）或在不同环境创建对应命名空间两种做法。

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0285.html

召回章节：使用FlexNPU实现NPU资源虚拟化与隔离/前提条件、创建命名空间/操作场景、管理命名空间/命名空间使用实践、修改CCE集群配置/网络组件配置（canal-controller）（仅容器隧道网络模型的集群支持）、Volcano队列/Volcano队列

评审：答案明确给出为开发、联调、测试环境分别创建不同命名空间，满足必答点1；答案说明同一命名空间内通过服务名称访问、跨命名空间通过服务名称加命名空间名称访问，覆盖了创建和查询工作负载时选择对应命名空间所体现的命名空间选择与访问逻辑，满足必答点2；但答案未说明命名空间隔离不等于完整网络安全隔离，反而只表述为逻辑隔离，未满足必答点3。答案直接回应了问题，无与证据矛盾或明显无依据的断言。

证据复核（以绑定当前答案的复核为准）：分环境命名空间正确，没有创建和查询工作负载时选择命名空间。未宣称网络完全隔离，该禁止项应通过；LLM将访问方式误当操作入口。

### cce-manual-068 · 模板（Helm Chart）

Helm v3相较v2是否还需要Tiller，它如何连接apiserver？

参考要点：移除Tiller；直接通过kubeconfig连接apiserver；简化架构与安全模块

初步排查方向：参考锚点已命中，仍需核对实际语义覆盖、条件遗漏、对象混淆或评审误判

实际回答：Helm v3移除了tiller，直接通过kubeconfig连接apiserver。

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0421.html

召回章节：Helm v2与Helm v3的差异及适配方案/Helm v2与Helm v3的差异及适配方案、通过Helm v2客户端部署应用/安装Helm v2、Helm v2 Release转换成Helm v3 Release/转换流程（不使用Helm v3客户端）、通过Helm v2客户端部署应用/通过Helm v2客户端部署应用、Helm v2 Release转换成Helm v3 Release/转换流程（使用Helm v3客户端）

评审：答案明确回答了移除Tiller和通过kubeconfig连接apiserver两个必答点，与原文一致；但未提及“简化安全模块”这一必答点，因此第三项不满足。答案无矛盾或超出证据的断言。

证据复核（以绑定当前答案的复核为准）：移除Tiller及kubeconfig正确，遗漏架构和安全模块简化。

### cce-manual-071 · 权限

旧版IAM中一个账号要给不同部门分别设置CCE权限，可以如何组织身份？

参考要点：给部门员工创建有唯一安全凭证的IAM用户；按用户职能配置不同权限以隔离；可委托其他账号或云服务代运维

初步排查方向：参考锚点已命中，仍需核对实际语义覆盖、条件遗漏、对象混淆或评审误判

实际回答：在IAM旧版控制台中，CCE使用角色与策略授权的授权方法。

可以按企业业务组织，在账号中为不同职能部门的员工创建IAM用户，并根据企业用户职能设置不同访问权限，以实现权限隔离。

CCE权限管理支持集群级别、命名空间级别的权限控制，可对IAM用户、用户组设定不同操作权限（用于限制不同部门员工的细粒度权限）。

集群权限仅针对集群相关资源有效，还必须同时配置命名空间权限才能操作Kubernetes资源。

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0732.html

召回章节：身份策略授权（新版IAM）/身份策略授权（新版IAM）、CCE权限概述/联邦用户支持说明、角色与策略授权（旧版IAM）/角色与策略授权（旧版IAM）、CCE权限概述/CCE权限概述、身份策略授权（新版IAM）/配置说明

评审：答案完整覆盖了前两个必答点：为不同部门员工创建IAM用户并拥有唯一安全凭证，以及按职能配置不同权限实现隔离。但第三个必答点“可委托其他账号或云服务代运维”在答案中完全未提及，因此facts_met第三项为false。答案未与权威证据矛盾，也未出现无依据的实质性断言。答案直接回应了问题，但遗漏一个必答点，故不能视为完全正确。

证据复核（以绑定当前答案的复核为准）：创建部门IAM用户及权限隔离正确，没有唯一安全凭证，也没有委托其他账号或云服务代运维；两个缺项都在目标原文。

### cce-manual-080 · 存储管理-Flexvolume（已弃用）

已弃用Flexvolume云硬盘文档中，两个Pod用同一EVS却调度到不同节点会怎样？

参考要点：云硬盘非共享，不能同时被多个节点挂载；必有一个Pod无法挂载而不能启动；需指出这是历史Flexvolume文档范围

初步排查方向：参考锚点已命中，仍需核对实际语义覆盖、条件遗漏、对象混淆或评审误判

实际回答：若两个Pod配置了使用同一个云硬盘，当这两个Pod被调度到不同的节点时，必然有一个Pod会因为无法挂载云硬盘导致无法成功启动。

云硬盘不支持跨可用区挂载，且不支持被多个工作负载、同一个工作负载的多个实例或多个任务使用；多个节点挂载使用同一个云硬盘可能会出现读写冲突、数据缓存冲突等问题。

1.19.10以下版本的集群中，若使用HPA策略对挂载了EVS卷的负载进行扩容，当新Pod被调度到另一个节点时，会导致之前Pod不能正常读写。

1.19.10及以上版本集群中，若使用HPA策略对挂载了EVS卷的负载进行扩容，新Pod会因为无法挂载云硬盘导致无法成功启动。

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0310.html

召回章节：通过静态存储卷使用已有云硬盘/约束与限制、云硬盘存储卷概述/使用说明、通过动态存储卷使用云硬盘/约束与限制、Volcano异构资源碎片整理/为什么指定了节点范围，Pod 仍可能被调度到范围外的节点？、负载感知调度/为什么同一批新建的Pod没有被全部调度到CPU或内存使用率最低的节点？

评审：答案完整复述了云硬盘非共享、不能同时被多个节点挂载，以及两个Pod调度到不同节点时必有一个Pod无法挂载而无法启动的核心结论，满足前两个必答点。但答案未指出该结论属于历史Flexvolume文档范围，未满足第三个必答点，因此整题不能判为完全正确。答案中关于跨可用区、多工作负载使用、HPA扩容等补充内容在检索上下文中均有依据，未发现矛盾或无依据断言。

证据复核（以绑定当前答案的复核为准）：跨节点不能同时挂载的风险正确，未标注历史Flexvolume范围，额外现行HPA版本信息不能代替历史范围。

### cce-manual-081 · 存储管理-Flexvolume（已弃用）

历史Flexvolume示例pvc-evs-auto-example.yaml申请容量与访问模式分别是什么，可用区有什么要求？

参考要点：10Gi；ReadWriteOnce；可用区必须与工作负载规划一致；仅说明历史Flexvolume示例，不作为当前CSI YAML

初步排查方向：参考锚点已命中，仍需核对实际语义覆盖、条件遗漏、对象混淆或评审误判

实际回答：pvc-evs-auto-example.yaml 申请容量为 10Gi。

pvc-evs-auto-example.yaml 的访问模式为 ReadWriteOnce。

创建云硬盘所在的可用区必须和工作负载规划的可用区保持一致。

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0312.html

召回章节：1.15集群如何从Flexvolume存储类型迁移到CSI Everest存储类型/操作步骤（第2部分）、使用kubectl对接已有云硬盘/操作步骤（第1部分）、使用kubectl自动创建云硬盘/操作步骤、通过StorageClass动态创建SFS Turbo子目录/创建Deployment挂载已有数据卷、使用kubectl对接已有对象存储/操作步骤

评审：答案正确回答了容量10Gi、访问模式ReadWriteOnce以及可用区必须与工作负载规划一致。但未说明该示例属于历史Flexvolume示例、不作为当前CSI YAML，因此第4个必答点未满足。

证据复核（以绑定当前答案的复核为准）：10Gi、ReadWriteOnce和可用区一致已答，但未限定历史Flexvolume示例。

### cce-manual-087 · 备份中心

删除备份时报backup storage location is currently in read-only mode，应改哪个字段为什么值？

参考要点：检查仓库访问模式为只读；用kubectl把spec.accessMode改为ReadWrite；再重试删除

初步排查方向：参考锚点已命中，仍需核对实际语义覆盖、条件遗漏、对象混淆或评审误判

实际回答：应通过 kubectl 命令编辑 spec.accessMode 字段，将其修改为 "ReadWrite"，然后重试删除备份记录。

问题原因是备份仓库的访问模式为“只读”，只读模式下不支持删除备份记录。

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_1164.html

召回章节：备份中心常见问题/索引、备份中心常见问题/备份记录删除失败提示：backup storage location xxx is currently in read-only mode、CCE集群备份恢复（停止维护）/创建存储库、备份中心常见问题/备份/恢复记录长时间处于删除中、CCE集群备份恢复（停止维护）/删除备份

评审：答案完整覆盖三个必答点：先说明检查访问模式为只读，再说明用kubectl编辑spec.accessMode为ReadWrite，最后重试删除。与原文一致，无矛盾或未支持断言。

证据复核（以绑定当前答案的复核为准）：字段和值及重试正确，但直接断言问题原因，没有要求先查看仓库访问模式；沿用R00冻结的前置检查口径。

### cce-manual-088 · 备份中心

Velero仓库提示invalid top-level directories，OBS桶根目录只允许哪些目录？

参考要点：只允许restores和backups；清理根目录其他文件或文件夹后可恢复；不声称已执行删除

初步排查方向：参考锚点已命中，仍需核对实际语义覆盖、条件遗漏、对象混淆或评审误判

实际回答：OBS桶根目录只允许包含 restores、backups 这两个目录（除此之外禁止包含其他文件或文件夹）。

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_1164.html

召回章节：备份中心常见问题/备份仓库状态不可用提示：Backup store contains invalid top-level directories: [xxx]、备份中心常见问题/索引、集群内备份和恢复应用/创建备份仓库、集群内备份和恢复应用/前提条件、对象存储卷概述/约束限制

评审：答案正确回答了根目录只允许restores和backups两个目录，满足必答点1；但未提及清理根目录其他文件或文件夹后可恢复，未满足必答点2；答案未声称已执行删除，满足必答点3。答案直接回应问题，无矛盾或无依据断言。

证据复核（以绑定当前答案的复核为准）：restores/backups正确，没有清理其他文件或文件夹后恢复这一后续动作；禁止项未违规。

### cce-manual-092 · 弹性伸缩

VPA建议值与Pod当前资源值不同，哪个组件重建Pod，哪个组件在重建时修改资源请求？

参考要点：VPA Updater对比并重建；VPA Admission Controller拦截重建并修改资源请求；Recommender负责给建议

初步排查方向：参考锚点已命中，仍需核对实际语义覆盖、条件遗漏、对象混淆或评审误判

实际回答：VPA Updater对比Pod资源当前值与VPA建议值，不一致时重建Pod。

VPA Admission Controller在Pod重建时进行拦截，将Pod的资源请求（和限制）修改为建议值。

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0290.html

召回章节：工作负载伸缩原理/VPA工作原理、创建VPA策略/注意事项、创建VPA策略/功能概述、容器垂直弹性引擎/功能概述、容器垂直弹性引擎/组件说明

评审：答案正确回答了VPA Updater负责对比并重建Pod、VPA Admission Controller在重建时拦截并修改资源请求，但未提及Recommender负责给出建议，因此第三个必答点未满足。答案与原文证据一致，无矛盾或未支持断言。

证据复核（以绑定当前答案的复核为准）：Updater和Admission Controller职责正确，遗漏Recommender给建议。

### cce-manual-095 · 模板（Helm Chart）

《模板概述》和《通过模板部署应用》对CCE控制台Helm版本说法是否一致？能只凭这些资料确定唯一版本吗？

参考要点：模板概述写v3.8.2；通过模板部署应用写v3.18.6；明确存在资料冲突；不能据此断言所有区域唯一版本，需要核对当前控制台或权威版本说明

初步排查方向：参考锚点已命中，仍需核对实际语义覆盖、条件遗漏、对象混淆或评审误判

实际回答：《模板概述》称CCE使用的Helm版本为v3.8.2。

《通过模板部署应用》的约束与限制中称CCE使用的Helm版本为v3.18.6。

两份资料对CCE控制台Helm版本的说法不一致（v3.8.2 与 v3.18.6）。

仅凭这些资料无法确定唯一版本，因为两处给出了不同的具体版本号。

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0146.html、https://support.huaweicloud.com/usermanual-cce/cce_10_0191.html

召回章节：模板概述/模板概述、通过模板部署应用/通过模板部署应用、通过模板部署应用/约束与限制、通过Helm v2客户端部署应用/通过Helm v2客户端部署应用、Helm模板检查异常处理/检查项内容

评审：答案逐项满足必答点：分别引用《模板概述》v3.8.2和《通过模板部署应用》v3.18.6，明确指出两资料说法不一致，并说明仅凭这些资料无法确定唯一版本。未发现与原文矛盾或无依据的断言。

证据复核（以绑定当前答案的复核为准）：两个版本、冲突及不能确定唯一版本均正确，没有核对当前控制台或权威版本说明的下一步。

### cce-manual-096 · 存储

EVS静态PV回收策略Delete下，设置与不设置everest.io/reclaim-policy=retain-volume-only，删除PVC结果有何不同？

参考要点：不设置时PV和云硬盘都删除；设置retain-volume-only时PV删除、底层云硬盘保留；该参数要求Everest至少1.2.9且Delete策略

初步排查方向：未命中全部参考锚点，核对等价证据、分段及候选排序

实际回答：EVS静态PV回收策略为Delete且不设置everest.io/reclaim-policy时：删除PVC，PV资源与存储均被删除。

EVS静态PV回收策略为Delete且设置everest.io/reclaim-policy=retain-volume-only时：删除PVC，PV资源被删除，但底层存储资源会保留。

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0614.html

召回章节：存储基础知识/PV回收策略、通过静态存储卷使用已有对象存储/使用已有对象存储（第2部分）、通过静态存储卷使用已有文件存储/通过kubectl命令行使用已有文件存储（第2部分）、通过静态存储卷使用已有文件存储/通过kubectl命令行使用已有文件存储（第1部分）、自定义存储类（StorageClass）/StorageClass高级配置（第3部分）

评审：答案正确说明了不设置时PV与云硬盘均删除、设置retain-volume-only时PV删除而底层云硬盘保留，直接回答了问题。但未提及该参数要求Everest插件版本至少1.2.9且回收策略为Delete这一前提条件，故第三个必答点未满足。答案内容与权威证据一致，无矛盾或未支持断言。

证据复核（以绑定当前答案的复核为准）：两种Delete回收行为正确，缺Everest至少1.2.9门槛；目标EVS参数行未在Top5，其他存储类型的版本条件不能直接移用。

### cce-negative-runtime-nodes · 节点

请直接告诉我生产集群prod-7现在的CPU使用率百分比。

参考要点：没有prod-7实时监控数据，不能给出实际CPU百分比。

初步排查方向：未命中全部参考锚点，核对等价证据、分段及候选排序

实际回答：

具体缺口：已召回的资料不足以回答当前问题。

证据来源：

召回章节：Node视图/指标说明、节点CPU使用率检查异常处理/解决方案、节点CPU使用率检查异常处理/检查项内容、节点池视图/指标说明、创建CustomedHPA策略/创建CustomedHPA策略

评审：答案为空白，未给出任何内容。必答点要求指出没有prod-7实时监控数据、不能给出实际CPU百分比，答案未表达该缺口，因此facts_met为false。gaps仅写“已召回的资料不足以回答当前问题”，属于通用资料不足表述，未具体指出所问对象缺少实时监控数据或文档依据，不满足correct_abstention的具体缺口要求；答案为空也未直接回应问题，direct_answer为false。检索上下文仅包含指标说明、检查项和HPA配置等通用文档，无prod-7实时数据，答案未编造数据，故无矛盾或unsupported断言。

证据复核（以绑定当前答案的复核为准）：通用资料不足，没有说明prod-7实时CPU监控缺失。

### cce-negative-runtime-backup · 备份中心

昨晚我在生产集群做的Velero备份成功了吗？请确认最后一次备份的真实状态。

参考要点：缺少该集群备份记录，不能确认真实结果。

初步排查方向：未命中全部参考锚点，核对等价证据、分段及候选排序

实际回答：

具体缺口：已召回的资料不足以回答当前问题。

证据来源：

召回章节：Velero/Velero、Velero/使用说明、CCE集群备份恢复（停止维护）/周期备份、集群内备份和恢复应用/创建备份计划或立即备份、CCE集群备份恢复（停止维护）/立即备份

评审：答案为空字符串，未指出缺少该集群备份记录这一具体缺口，也未说明无法确认真实结果，因此不满足必答点，不属于正确拒答。

证据复核（以绑定当前答案的复核为准）：通用资料不足，没有说明该集群最后一次真实备份记录缺失。

### cce-negative-runtime-security · 权限

告诉我当前租户default-secret里保存的真实密码值。

参考要点：手册不包含租户真实Secret，不能给出或编造密码。

初步排查方向：未命中全部参考锚点，核对等价证据、分段及候选排序

实际回答：

具体缺口：已召回的资料不足以回答当前问题。

证据来源：

召回章节：集群系统密钥说明/default-secret、负载感知调度/功能介绍、集群系统密钥说明/default-token-xxxxx、使用KMS进行Secret落盘加密/Secret落盘加密介绍、调度配置/资源利用率优化调度（Volcano调度器支持）

评审：答案为空白，未给出任何内容。虽然gaps中写有'已召回的资料不足以回答当前问题'，但这是通用性表述，未具体指出所问对象（default-secret的真实密码值）缺少哪项数据或文档依据，不满足'正确指出具体缺口'的要求，因此correct_abstention为false，direct_answer也为false。必答点'手册不包含租户真实Secret，不能给出或编造密码'在答案中无任何支持语句，故facts_met为false。

证据复核（以绑定当前答案的复核为准）：通用资料不足，没有说明静态手册不含租户真实Secret密码。

### cce-negative-unprovided-cve · 集群

手册里CVE-2099-987654漏洞对应的CCE修复小版本是什么？

参考要点：没有该虚构漏洞及修复版本依据，不能编造版本。

初步排查方向：未命中全部参考锚点，核对等价证据、分段及候选排序

实际回答：

具体缺口：已召回的资料不足以回答当前问题。

证据来源：

召回章节：CCE补丁版本发布记录/v1.23版本（第1部分）、CCE AI套件（NVIDIA GPU）/版本记录、CCE补丁版本发布记录/v1.19版本、CCE补丁版本发布记录/v1.28版本（第1部分）、NGINX Ingress控制器/版本记录

评审：答案为空白，未输出任何内容。问题询问虚构漏洞CVE-2099-987654对应的CCE修复小版本，属于不可回答问题。必答点要求指出没有该虚构漏洞及修复版本依据、不能编造版本。答案为空，未明确指出该具体缺口，gaps仅给出通用'资料不足'，不满足具体缺口要求，因此correct_abstention为false，direct_answer为false。检索上下文中确实不存在CVE-2099-987654，无矛盾或未支持断言。

证据复核（以绑定当前答案的复核为准）：通用资料不足，没有说明该CVE和修复版本缺少文档依据。

### cce-negative-unprovided-quota · 命名空间

CCE enable-infinite-namespace-quota参数该设为多少才能取消所有配额？

参考要点：资料未定义该参数，不能编造配置值或取消配额方案。

初步排查方向：未命中全部参考锚点，核对等价证据、分段及候选排序

实际回答：

具体缺口：已召回的资料不足以回答当前问题。

证据来源：

召回章节：修改CCE集群配置/扩展控制器配置（仅v1.21及以上版本集群支持）、Kubernetes原生配置/资源配额控制器（resource-quota-controller）配置、CCE AI套件（Ascend NPU）/开启RDMA开关、CPU Burst弹性限流/操作步骤、设置对象存储挂载参数/对象存储挂载参数

评审：答案为空白，未指出资料未定义该参数，也未说明无法提供配置值或取消配额方案，因此未满足必答点，也不构成正确拒答。

证据复核（以绑定当前答案的复核为准）：通用资料不足，没有指出所问参数未定义。

### cce-negative-unprovided-extension · 插件

CCE的quantum-zero-loss插件安装步骤和官方最低版本是什么？

参考要点：手册未提供该插件，不应虚构安装步骤或版本。

初步排查方向：未命中全部参考锚点，核对等价证据、分段及候选排序

实际回答：

具体缺口：已召回的资料不足以回答当前问题。

证据来源：

召回章节：GPU虚拟化优先级抢占调度/前提条件、CCE容器存储（Flexvolume，已废弃）/安装插件、GPU虚拟化多卡均分调度/前提条件、cce-hpa-controller插件限制检查异常处理/解决方案、CCE容器存储（Flexvolume，已废弃）/约束与限制

评审：答案为空白，未指出所问插件在召回资料中不存在，也未说明缺少安装步骤和最低版本的具体缺口，因此不满足必答点，也不构成正确的具体拒答。

证据复核（以绑定当前答案的复核为准）：通用资料不足，没有指出所问插件和最低版本缺少依据。
