# live-r04-dev-evidence-selection

模式：LIVE_ANSWERS

```json
{
  "completed": 75,
  "expected": 75,
  "errors": 0,
  "service_failures": 0,
  "answer_accuracy": 0.96,
  "llm_judged_answer_accuracy": 0.9466666666666667,
  "evidence_recall_at_5": 0.9782608695652174,
  "all_evidence_at_5": 0.9710144927536232,
  "document_hit_at_5": 0.9855072463768116,
  "latency_p95_ms": 41768.441915999574,
  "generation_usage": {
    "calls": 151,
    "prompt_tokens": 1825954,
    "completion_tokens": 65525,
    "total_tokens": 1891479
  },
  "citation_integrity_rate": 1.0,
  "evidence_selection": {
    "responses": 75,
    "candidate_fragments_total": 3023,
    "expanded_fragments_total": 469,
    "input_bytes_total": 5410743,
    "final_context_bytes_max": 39438
  },
  "llm_judged_passed": 71,
  "wilson_95": [
    0.8707382679518444,
    0.9790669110399569
  ],
  "audit_status": "completed",
  "real_model_calls": 151,
  "llm_judged_answerable_accuracy": 0.9420289855072463,
  "llm_judged_abstention_accuracy": 1.0,
  "audit_required_ids": [
    "cce-manual-055",
    "cce-manual-064",
    "cce-manual-091",
    "cce-manual-096",
    "cce-manual-002",
    "cce-manual-097",
    "cce-manual-088",
    "cce-manual-074",
    "cce-manual-095",
    "cce-manual-098",
    "cce-manual-022",
    "cce-manual-071",
    "cce-manual-013",
    "cce-manual-012",
    "cce-manual-052",
    "cce-manual-035",
    "cce-manual-006",
    "cce-manual-016",
    "cce-negative-unprovided-extension"
  ],
  "judge_usage": {
    "prompt_tokens": 357738,
    "completion_tokens": 14107,
    "total_tokens": 371845
  },
  "audited_passed": 72,
  "audited_count": 54,
  "audited_wilson_95": [
    0.8888683137691623,
    0.9863041839985135
  ],
  "answerable_accuracy": 0.9565217391304348,
  "abstention_accuracy": 1.0
}
```

准确率仅在真实回答、语义评审和证据复核完成后报告；检索指标不代表答案正确。

## Bad cases（3）

### cce-manual-035 · 弹性伸缩

业务指标比CPU和内存更能反映流量，CCE能用自定义指标驱动HPA吗？

参考要点：支持自定义指标HPA；对接开源Prometheus生态暴露和监控自定义指标；不局限默认CPU内存指标

初步排查方向：参考锚点已命中，仍需核对实际语义覆盖、条件遗漏、对象混淆或评审误判

实际回答：资料范围：
创建使用自定义指标的HPA策略
目录：用户指南 > 弹性伸缩 > 工作负载弹性伸缩 > 创建使用自定义指标的HPA策略
更新时间：2026-09-20 GMT+08:00

工作负载伸缩原理
目录：用户指南 > 弹性伸缩 > 工作负载弹性伸缩 > 工作负载伸缩原理
更新时间：2026-09-20 GMT+08:00

云原生监控插件
目录：用户指南 > 插件 > 云原生可观测性插件 > 云原生监控插件
更新时间：2026-09-20 GMT+08:00

创建HPA策略
目录：用户指南 > 弹性伸缩 > 工作负载弹性伸缩 > 创建HPA策略
更新时间：2026-09-20 GMT+08:00

可以。CCE支持使用自定义指标驱动HPA，业务指标（如队列长度、在线用户数、每秒请求量、QPS、连接数等）可作为HPA的伸缩依据，从而比仅用CPU和内存更贴合业务特点。手册原文：来源《创建使用自定义指标的HPA策略》"创建使用自定义指标的HPA策略"章节："Kubernetes默认的HPA策略只支持基于CPU和内存的自动伸缩，在复杂的业务场景中，仅使用CPU和内存使用率指标进行弹性伸缩往往无法满足日常运维需求。通过自定义指标配置工作负载HPA策略，可以根据业务自身特点，通过更多指标实现更灵活的弹性配置。"；来源《工作负载伸缩原理》"HPA工作原理"章节："HPA是基于指标阈值进行伸缩的，常见的指标主要是 CPU、内存，也可以通过自定义指标，例如QPS、连接数等进行伸缩。"

CCE全面对接开源Prometheus生态，支持类型丰富的自定义指标监控，并支持用自定义指标实现弹性伸缩。手册原文：来源《创建使用自定义指标的HPA策略》"工作原理"章节："CCE全面对接开源Prometheus生态，支持类型丰富的自定义指标监控，同时支持使用自定义指标实现弹性伸缩"。

用自定义指标实现HPA弹性伸缩的核心流程为：1.指标暴露：业务应用在代码里预置指标上报能力，将关键指标（如队列长度、在线用户数、每秒请求量）暴露出来；2.指标采集：如果应用无法直接暴露指标，可部署专用Exporter组件，从应用中抓取数据并转换成Prometheus能识别的格式；3.指标存储：Prometheus定期从Exporter或应用直接拉取指标并存储，形成历史数据供后续查询和分析；4.指标转换：通过配置文件（user-adapter-config）定义"PromQL查询语句"与"Kubernetes自定义指标API名称"之间的映射关系；5.API接入：在API Server中注册自定义指标API，使HPA可以像查询CPU指标一样通过统一API接口查询自定义指标；6.执行伸缩：HPA定期查询自定义指标的值，计算工作负载的期望副本数量并执行扩缩容。手册原文：来源《创建使用自定义指标的HPA策略》"工作原理"章节："利用自定义指标实现HPA弹性伸缩的核心流程如下：1. 指标暴露：业务应用需要在代码里预置指标上报能力，将关键指标（如队列长度、在线用户数、每秒请求量）暴露出来。2. 指标采集：如果应用无法直接暴露指标，可以部署一个专用的Exporter组件，它负责从应用中抓取数据，并转换成Prometheus能识别的格式。3. 指标存储：Prometheus定期从Exporter或应用直接拉取指标，并进行存储，形成历史数据，供后续查询和分析。4. 指标转换：通过配置文件（user-adapter-config）定义"PromQL查询语句"与"Kubernetes自定义指标API名称"之间的映射关系。5. API接入：在API Server中注册自定义指标API，使得HPA可以像查询CPU指标一样，通过统一的API接口查询到自定义指标。6. 执行伸缩：HPA定期查询自定义指标的值，计算工作负载的期望副本数量并执行扩缩容。"

使用自定义指标HPA的前提条件：云原生监控插件需开启本地数据存储，才能使用自定义指标HPA功能；可在user-adapter-config配置项中配置HPA弹性策略需要的自定义指标。使用Prometheus监控自定义指标时，应用程序需要提供监控指标接口。手册原文：来源《云原生监控插件》"使用自定义指标创建HPA策略"章节："云原生监控插件为开启本地数据存储时，才能使用自定义指标HPA功能，您可在user-adapter-config配置项中配置HPA弹性策略需要的自定义指标。""使用Prometheus监控自定义指标时，应用程序需要提供监控指标接口"。

在CCE中配置自定义指标HPA的操作步骤为：1.登录CCE控制台，单击集群名称进入集群；2.在左侧导航栏选择"配置与密钥"，切换至"monitoring"命名空间，在"配置项"页签找到user-adapter-config配置项（或adapter-config），单击"更新"；3.在"配置数据"中单击config.yaml对应的"编辑"按钮，在rules字段下添加自定义指标采集规则，修改完成后单击"确定"保存配置（如需增加多个采集规则，可在rules字段下添加多个配置）；4.重新部署monitoring命名空间下的custom-metrics-apiserver工作负载；5.在左侧导航栏选择"工作负载"，找到需要创建HPA策略的工作负载单击"更多>弹性伸缩"，在"自定义策略"中选择上述参数创建弹性伸缩策略。手册原文：来源《云原生监控插件》"使用自定义指标创建HPA策略"章节："1. 登录CCE控制台，单击集群名称进入集群。2. 在左侧导航栏中选择"配置与密钥"，并切换至"monitoring"命名空间，在"配置项"页签找到user-adapter-config配置项（或adapter-config），并单击"更新"。3. 在"配置数据"中单击config.yaml对应的"编辑"按钮，在rules字段下添加自定义指标采集规则。修改完成后单击"确定"保存配置。如果您需要增加多个采集规则，可在rules字段下添加多个配置，关于采集规则配置详情请参见Metrics Discovery and Presentation Configuration。4. 重新部署monitoring命名空间下的custom-metrics-apiserver工作负载。5. 在左侧导航栏中选择"工作负载"，找到需要创建HPA策略的工作负载单击"更多>弹性伸缩"。您可在"自定义策略"中选择上述参数创建弹性伸缩策略。"

自定义指标采集规则示例（示例值，非实时值）：匹配指标名称是nginx_connections_accepted的指标，必须确认指标名称，否则HPA控制器无法获取到指标；指定Pod和命名空间资源；使用nginx_connections_accepted_per_second来代表该指标，该名称即在HPA的自定义策略中的自定义指标名称；通过计算表达式rate(nginx_connections_accepted[2m])来代表每秒的请求接收量。手册原文：来源《云原生监控插件》"使用自定义指标创建HPA策略"章节："自定义采集规则示例如下：rules: # 匹配指标名称是nginx_connections_accepted的指标，必须确认指标名称，否则HPA控制器无法获取到指标 - seriesQuery: '{__name__=~"nginx_connections_accepted",container!="POD",namespace!="",pod!=""}' resources: # 指定Pod和命名空间资源 overrides: namespace: resource: namespace pod: resource: pod name: #使用nginx_connections_accepted matches: "nginx_connections_accepted" #使用nginx_connections_accepted_per_second来代表该指标，该名称即在HPA的自定义策略中的自定义指标名称 as: "nginx_connections_accepted_per_second" #通过计算表达式rate(nginx_connections_accepted[2m])来代表是每秒的请求接收量 metricsQuery: 'rate(<<.Series>>{<<.LabelMatchers>>,container!="POD"}[2m])'"

使用HPA需要安装能够提供Metrics API的插件，可根据集群版本和实际需求选择：Kubernetes Metrics Server提供基础资源使用指标（如容器CPU和内存使用率，默认采集周期60s），所有集群版本均可安装；云原生监控插件使用HPA需要开启本地数据存储，支持v1.17及以后集群版本，默认采集周期15s。根据自定义指标进行弹性伸缩时，除了将Prometheus注册为Metrics API的服务，还需要将自定义指标聚合到Kubernetes API Server。手册原文：来源《创建HPA策略》"前提条件"章节："使用HPA需要安装能够提供Metrics API的插件，您可根据集群版本和实际需求选择其中之一：- Kubernetes Metrics Server：提供基础资源使用指标，例如容器CPU和内存使用率，默认采集周期为60s。所有集群版本均可安装。- 云原生监控插件：使用HPA需要开启本地数据存储。该插件支持v1.17及以后的集群版本，默认采集周期为15s。……- 根据自定义指标进行弹性伸缩：除了将Prometheus注册为Metrics API的服务，还需要将自定义指标聚合到Kubernetes API Server，详情请参见使用自定义指标创建HPA策略。"

HPA通过标准Metrics API对外提供指标，其中custom.metrics.k8s.io主要提供Kubernetes Object相关的自定义监控指标，external.metrics.k8s.io指标来源外部、与任何Kubernetes资源的指标无关。手册原文：来源《工作负载伸缩原理》"HPA工作原理"章节："社区经过思考之后，定义了一套标准的Metrics API，通过聚合API对外提供服务。- metrics.k8s.io： 主要提供Pod和Node的CPU和Memory相关的监控指标。- custom.metrics.k8s.io： 主要提供Kubernetes Object相关的自定义监控指标。- external.metrics.k8s.io：指标来源外部，与任何的Kubernetes资源的指标无关。"

HPA的扩缩容决策算法为：期望实例数 = 向上取整[当前实例数 * (当前的指标值 / 目标值)]。手册原文：来源《工作负载伸缩原理》"HPA工作原理"章节："HPA controller根据当前指标和期望指标来计算缩放比例，计算公式如下：期望实例数 = 向上取整[当前实例数 * ( 当前的指标值 / 目标值 )]"。

基于指标的伸缩存在一定时延，主要包含采集时延（分钟级）+判断时延（分钟级）+伸缩时延（分钟级），可能导致应用CPU飚高、响应时间变慢；为解决该问题，CCE提供了定时策略，对于有周期性变化的应用可提前扩容资源，业务低谷时定时回收资源。手册原文：来源《工作负载伸缩原理》"HPA工作原理"章节："HPA是基于指标阈值进行伸缩的……但是存在一个问题：基于指标的伸缩存在一定的时延，这个时延主要包含：采集时延(分钟级) + 判断时延(分钟级) + 伸缩时延(分钟级)。这个分钟级的时延，可能会导致应用CPU飚高，响应时间变慢。为了解决这个问题，CCE提供了定时策略，对于一些有周期性变化的应用，提前扩容资源，而业务低谷时，定时回收资源。"

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0944.html

召回章节：创建使用自定义指标的HPA策略/创建使用自定义指标的HPA策略、创建使用自定义指标的HPA策略/工作原理、云原生监控插件/使用自定义指标创建HPA策略、创建HPA策略/前提条件、工作负载伸缩原理/HPA工作原理

评审：答案明确回答“可以”，并逐项覆盖三个必答点：支持自定义指标HPA、CCE全面对接开源Prometheus生态并支持自定义指标监控与弹性伸缩、不局限默认CPU内存指标。所有关键断言均有原文或召回内容支持，无矛盾或未支持断言。

证据复核（以绑定当前答案的复核为准）：三项必答点已表达且无多余gaps，但补充断言“HPA通过标准Metrics API对外提供指标”把指标消费者HPA说成提供者。实际原文是社区定义Metrics API、经聚合API提供，HPA定期查询；引文正确不能消除其前面的错误机制断言。

### cce-manual-091 · 弹性伸缩

metrics.k8s.io、custom.metrics.k8s.io和external.metrics.k8s.io分别提供什么指标？

参考要点：metrics提供Pod/Node的CPU内存；custom提供Kubernetes对象关联的自定义指标；external来自外部、不关联Kubernetes资源

初步排查方向：未命中全部参考锚点，核对等价证据、分段及候选排序

实际回答：资料范围：
Prometheus（停止维护）
目录：用户指南 > 插件 > 云原生可观测性插件 > Prometheus（停止维护）
更新时间：2026-09-20 GMT+08:00

云原生监控插件
目录：用户指南 > 插件 > 云原生可观测性插件 > 云原生监控插件
更新时间：2026-09-20 GMT+08:00

基于GPU监控指标配置工作负载弹性伸缩
目录：用户指南 > 调度 > GPU调度 > GPU弹性伸缩 > 基于GPU监控指标配置工作负载弹性伸缩
更新时间：2026-09-20 GMT+08:00

创建使用自定义指标的HPA策略
目录：用户指南 > 弹性伸缩 > 工作负载弹性伸缩 > 创建使用自定义指标的HPA策略
更新时间：2026-09-20 GMT+08:00

创建HPA策略
目录：用户指南 > 弹性伸缩 > 工作负载弹性伸缩 > 创建HPA策略
更新时间：2026-09-20 GMT+08:00

在所提供的资料中，只有 metrics.k8s.io 有直接说明：它提供容器和节点的资源指标，如CPU、内存使用量，这些指标可通过Kubernetes的Metrics API获得，可直接被用户访问（例如用kubectl top命令），也可被HPA或CustomedHPA使用，根据资源使用率使负载弹性伸缩。资料中出现的APIService对象spec.group即为metrics.k8s.io，其name为v1beta1.metrics.k8s.io。

关于 custom.metrics.k8s.io：资料未直接说明该API组“提供什么指标”的定义性描述，只给出了使用方式——通过custom.metrics.k8s.io/v1beta1查询自定义指标，例如查询Pod的cce_gpu_memory_utilization指标，以及将Prometheus暴露的指标（如container_cpu_usage_seconds_total聚合为container_cpu_usage_core_per_second）转换为HPA可关联的自定义指标。因此该API组在资料中对应的是自定义指标（custom metrics）的查询与聚合，而非基础资源指标。

关于 external.metrics.k8s.io：所提供的全部资料中均未出现该API组，也没有任何关于它提供何种指标的说明。

具体缺口：资料未提供 external.metrics.k8s.io 提供何种指标的说明，也未给出 custom.metrics.k8s.io 的定义性描述（仅给出其查询自定义指标的使用示例）。

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0290.html

召回章节：Prometheus（停止维护）/通过Metrics API提供资源指标、云原生监控插件/通过Metrics API提供基础资源指标、基于GPU监控指标配置工作负载弹性伸缩/采集GPU指标、创建使用自定义指标的HPA策略/步骤四：修改配置文件、创建HPA策略/前提条件

评审：metrics.k8s.io部分正确，但custom.metrics.k8s.io未给出定义性描述（仅说明查询自定义指标的使用方式），external.metrics.k8s.io完全未回答，因此未完整满足三个必答点，direct_answer为false。答案对缺失内容作了如实说明，但可回答题证据不足时的谨慎拒答不能算答对，故correct_abstention为false。

证据复核（以绑定当前答案的复核为准）：三类Metrics API的定义原文未进入最终上下文，答案缺少external来自外部且不关联Kubernetes资源的说明，custom也只给使用示例，返回PARTIAL。谨慎承认缺口不等于可回答题答对。

### cce-manual-092 · 弹性伸缩

VPA建议值与Pod当前资源值不同，哪个组件重建Pod，哪个组件在重建时修改资源请求？

参考要点：VPA Updater对比并重建；VPA Admission Controller拦截重建并修改资源请求；Recommender负责给建议

初步排查方向：参考锚点已命中，仍需核对实际语义覆盖、条件遗漏、对象混淆或评审误判

实际回答：资料范围：
工作负载伸缩原理
目录：用户指南 > 弹性伸缩 > 工作负载弹性伸缩 > 工作负载伸缩原理
更新时间：2026-09-20 GMT+08:00

容器垂直弹性引擎
目录：用户指南 > 插件 > 容器调度与弹性插件 > 容器垂直弹性引擎
更新时间：2026-09-20 GMT+08:00

当VPA建议值与Pod当前资源值不一致时，由VPA Updater重建Pod。手册原文：VPA Updater：对比建议值和当前值，不一致时重建Pod。VPA生效的主要流程如下：……2. VPA Updater对比Pod资源当前值与VPA建议值是否一致。3. VPA Updater检测到Pod资源当前值与建议值不一致时，重建Pod以根据建议值调整规格。（来源：用户指南 > 弹性伸缩 > 工作负载弹性伸缩 > 工作负载伸缩原理，VPA工作原理）

在Pod重建时修改资源请求（资源申请量）的组件是VPA Admission Controller。手册原文：VPA Admission Controller：在Pod重建时将Pod的资源申请量修改为建议值。VPA生效的主要流程如下：……4. Pod发生重建时，VPA Admission Controller会进行拦截，确保资源请求和限制与建议值一致。5. 完成VPA调整，使用VPA建议值作为Pod的资源申请值。（来源：用户指南 > 弹性伸缩 > 工作负载弹性伸缩 > 工作负载伸缩原理，VPA工作原理）

从CCE容器垂直弹性引擎组件说明看，vpa-updater负责驱逐实际资源申请量与VPA建议值有偏差的Pod，触发Pod重建以使得资源建议值生效至新建的Pod；vpa-admission-controller负责在Pod创建时将容器的资源申请量调整为VPA生成的建议值。手册原文：vpa-admission-controller：Pod创建时，将容器的资源申请量调整为VPA生成的建议值。……vpa-updater：驱逐实际资源申请量与VPA建议值有偏差的Pod，触发Pod重建以使得资源建议值生效至新建的Pod。（来源：用户指南 > 插件 > 容器调度与弹性插件 > 容器垂直弹性引擎，组件说明）

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0290.html

召回章节：工作负载伸缩原理/VPA工作原理、容器垂直弹性引擎/组件说明、创建VPA策略/功能概述

评审：答案完整覆盖三个必答点：VPA Updater对比并重建Pod、VPA Admission Controller在重建时修改资源请求、Recommender负责给出建议。所有断言均有原文或检索上下文支持，无矛盾或未支持内容。

证据复核（以绑定当前答案的复核为准）：Updater比较并触发重建、Admission Controller拦截修改请求均正确，但答案完全遗漏Recommender负责产生建议。实际召回有明确组件和流程说明；引用中的省略号跳过步骤1，不能将上下文内容当作答案已经表达。
