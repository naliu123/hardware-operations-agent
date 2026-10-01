# live-r05-dev-reliability

模式：LIVE_ANSWERS

```json
{
  "completed": 75,
  "expected": 75,
  "errors": 0,
  "service_failures": 0,
  "answer_accuracy": 0.96,
  "llm_judged_answer_accuracy": 0.92,
  "evidence_recall_at_5": 0.9782608695652174,
  "all_evidence_at_5": 0.9710144927536232,
  "document_hit_at_5": 0.9855072463768116,
  "latency_p95_ms": 52418.69770800258,
  "generation_usage": {
    "calls": 152,
    "prompt_tokens": 1832047,
    "completion_tokens": 65489,
    "total_tokens": 1897536
  },
  "citation_integrity_rate": 1.0,
  "evidence_selection": {
    "responses": 75,
    "candidate_fragments_total": 3023,
    "expanded_fragments_total": 469,
    "input_bytes_total": 5410743,
    "final_context_bytes_max": 39438
  },
  "llm_judged_passed": 69,
  "wilson_95": [
    0.8362501593344202,
    0.9628203821056319
  ],
  "audit_status": "completed",
  "real_model_calls": 152,
  "llm_judged_answerable_accuracy": 0.927536231884058,
  "llm_judged_abstention_accuracy": 0.8333333333333334,
  "audit_required_ids": [
    "cce-manual-055",
    "cce-manual-064",
    "cce-manual-090",
    "cce-manual-091",
    "cce-manual-096",
    "cce-negative-runtime-nodes",
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
    "cce-manual-016"
  ],
  "judge_usage": {
    "prompt_tokens": 358742,
    "completion_tokens": 14415,
    "total_tokens": 373157
  },
  "audited_passed": 72,
  "audited_count": 20,
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

可以。CCE支持使用自定义指标驱动HPA，业务指标（如队列长度、在线用户数、每秒请求量、QPS、连接数等）可作为HPA的伸缩依据，比仅用CPU和内存更灵活。手册原文（《创建使用自定义指标的HPA策略》"创建使用自定义指标的HPA策略"章节）：Kubernetes默认的HPA策略只支持基于CPU和内存的自动伸缩，在复杂的业务场景中，仅使用CPU和内存使用率指标进行弹性伸缩往往无法满足日常运维需求。通过自定义指标配置工作负载HPA策略，可以根据业务自身特点，通过更多指标实现更灵活的弹性配置。手册原文（《工作负载伸缩原理》"HPA工作原理"章节）：HPA是基于指标阈值进行伸缩的，常见的指标主要是 CPU、内存，也可以通过自定义指标，例如QPS、连接数等进行伸缩。

CCE全面对接开源Prometheus生态，支持类型丰富的自定义指标监控，并支持用自定义指标实现弹性伸缩。手册原文（《创建使用自定义指标的HPA策略》"工作原理"章节）：CCE全面对接开源Prometheus生态，支持类型丰富的自定义指标监控，同时支持使用自定义指标实现弹性伸缩。

用自定义指标实现HPA弹性伸缩的核心流程（各参与组件及职责）：1.指标暴露——业务应用在代码里预置指标上报能力，将关键指标（如队列长度、在线用户数、每秒请求量）暴露出来；2.指标采集——如果应用无法直接暴露指标，可部署专用Exporter组件，从应用中抓取数据并转换成Prometheus能识别的格式；3.指标存储——Prometheus定期从Exporter或应用直接拉取指标并存储，形成历史数据；4.指标转换——通过配置文件（user-adapter-config）定义"PromQL查询语句"与"Kubernetes自定义指标API名称"之间的映射关系；5.API接入——在API Server中注册自定义指标API，使HPA可像查询CPU指标一样通过统一API查询自定义指标；6.执行伸缩——HPA定期查询自定义指标的值，计算工作负载的期望副本数量并执行扩缩容。手册原文（《创建使用自定义指标的HPA策略》"工作原理"章节）：利用自定义指标实现HPA弹性伸缩的核心流程如下：1. 指标暴露：业务应用需要在代码里预置指标上报能力，将关键指标（如队列长度、在线用户数、每秒请求量）暴露出来。2. 指标采集：如果应用无法直接暴露指标，可以部署一个专用的Exporter组件，它负责从应用中抓取数据，并转换成Prometheus能识别的格式。3. 指标存储：Prometheus定期从Exporter或应用直接拉取指标，并进行存储，形成历史数据，供后续查询和分析。4. 指标转换：通过配置文件（user-adapter-config）定义"PromQL查询语句"与"Kubernetes自定义指标API名称"之间的映射关系。5. API接入：在API Server中注册自定义指标API，使得HPA可以像查询CPU指标一样，通过统一的API接口查询到自定义指标。6. 执行伸缩：HPA定期查询自定义指标的值，计算工作负载的期望副本数量并执行扩缩容。

使用自定义指标HPA的前提条件：云原生监控插件需开启本地数据存储，才能使用自定义指标HPA功能；可在user-adapter-config配置项中配置HPA弹性策略需要的自定义指标。使用Prometheus监控自定义指标时，应用程序需要提供监控指标接口。手册原文（《云原生监控插件》"使用自定义指标创建HPA策略"章节）：云原生监控插件为开启本地数据存储时，才能使用自定义指标HPA功能，您可在user-adapter-config配置项中配置HPA弹性策略需要的自定义指标。使用Prometheus监控自定义指标时，应用程序需要提供监控指标接口。

使用HPA需要安装能够提供Metrics API的插件，可按集群版本和实际需求选择：Kubernetes Metrics Server提供基础资源使用指标（如容器CPU和内存使用率），默认采集周期60s，所有集群版本均可安装；云原生监控插件使用HPA需开启本地数据存储，支持v1.17及以后集群版本，默认采集周期15s。其中根据自定义指标进行弹性伸缩时，除了将Prometheus注册为Metrics API的服务，还需要将自定义指标聚合到Kubernetes API Server。手册原文（《创建HPA策略》"前提条件"章节）：使用HPA需要安装能够提供Metrics API的插件，您可根据集群版本和实际需求选择其中之一：Kubernetes Metrics Server：提供基础资源使用指标，例如容器CPU和内存使用率，默认采集周期为60s。所有集群版本均可安装。云原生监控插件：使用HPA需要开启本地数据存储。该插件支持v1.17及以后的集群版本，默认采集周期为15s。根据自定义指标进行弹性伸缩：除了将Prometheus注册为Metrics API的服务，还需要将自定义指标聚合到Kubernetes API Server，详情请参见使用自定义指标创建HPA策略。

HPA通过标准Metrics API对外提供指标，其中custom.metrics.k8s.io主要提供Kubernetes Object相关的自定义监控指标，external.metrics.k8s.io指标来源外部、与任何Kubernetes资源的指标无关。手册原文（《工作负载伸缩原理》"HPA工作原理"章节）：社区经过思考之后，定义了一套标准的Metrics API，通过聚合API对外提供服务。metrics.k8s.io： 主要提供Pod和Node的CPU和Memory相关的监控指标。custom.metrics.k8s.io： 主要提供Kubernetes Object相关的自定义监控指标。external.metrics.k8s.io：指标来源外部，与任何的Kubernetes资源的指标无关。

配置自定义指标HPA的操作步骤（《云原生监控插件》"使用自定义指标创建HPA策略"章节）：1.登录CCE控制台，单击集群名称进入集群；2.在左侧导航栏选择"配置与密钥"，切换至"monitoring"命名空间，在"配置项"页签找到user-adapter-config配置项（或adapter-config），单击"更新"；3.在"配置数据"中单击config.yaml对应的"编辑"按钮，在rules字段下添加自定义指标采集规则，修改完成后单击"确定"保存配置（如需增加多个采集规则，可在rules字段下添加多个配置）；4.重新部署monitoring命名空间下的custom-metrics-apiserver工作负载；5.在左侧导航栏选择"工作负载"，找到需要创建HPA策略的工作负载单击"更多>弹性伸缩"，可在"自定义策略"中选择上述参数创建弹性伸缩策略。手册原文：1. 登录CCE控制台，单击集群名称进入集群。2. 在左侧导航栏中选择"配置与密钥"，并切换至"monitoring"命名空间，在"配置项"页签找到user-adapter-config配置项（或adapter-config），并单击"更新"。3. 在"配置数据"中单击config.yaml对应的"编辑"按钮，在rules字段下添加自定义指标采集规则。修改完成后单击"确定"保存配置。如果您需要增加多个采集规则，可在rules字段下添加多个配置。4. 重新部署monitoring命名空间下的custom-metrics-apiserver工作负载。5. 在左侧导航栏中选择"工作负载"，找到需要创建HPA策略的工作负载单击"更多>弹性伸缩"。您可在"自定义策略"中选择上述参数创建弹性伸缩策略。

自定义采集规则示例（示例中的指标名称nginx_connections_accepted及派生名称nginx_connections_accepted_per_second为示例值，非固定属性）：匹配指标名称是nginx_connections_accepted的指标，必须确认指标名称，否则HPA控制器无法获取到指标；指定Pod和命名空间资源；使用nginx_connections_accepted_per_second来代表该指标，该名称即在HPA的自定义策略中的自定义指标名称；通过计算表达式rate(nginx_connections_accepted[2m])来代表每秒的请求接收量。手册原文（《云原生监控插件》"使用自定义指标创建HPA策略"章节）：自定义采集规则示例如下：rules: # 匹配指标名称是nginx_connections_accepted的指标，必须确认指标名称，否则HPA控制器无法获取到指标 - seriesQuery: '{__name__=~"nginx_connections_accepted",container!="POD",namespace!="",pod!=""}' resources: # 指定Pod和命名空间资源 overrides: namespace: resource: namespace pod: resource: pod name: #使用nginx_connections_accepted matches: "nginx_connections_accepted" #使用nginx_connections_accepted_per_second来代表该指标，该名称即在HPA的自定义策略中的自定义指标名称 as: "nginx_connections_accepted_per_second" #通过计算表达式rate(nginx_connections_accepted[2m])来代表是每秒的请求接收量 metricsQuery: 'rate(<<.Series>>{<<.LabelMatchers>>,container!="POD"}[2m])'

HPA的扩缩容决策算法：HPA controller根据当前指标和期望指标计算缩放比例，公式为"期望实例数 = 向上取整[当前实例数 * ( 当前的指标值 / 目标值 )]"（示例：当前指标值200m、目标值100m时，期望实例数翻倍，该数值为示例）。为保证稳定性，HPA controller从冷却时间和忍受度两方面优化：冷却时间——1.11及之前版本引入horizontal-pod-autoscaler-downscale-stabilization-window和horizontal-pod-autoScaler-upscale-stabilization-window两个启动参数代表缩容冷却时间和扩容冷却时间，保证冷却时间内跳过扩缩容；1.14版本之后引入延迟队列，保存一段时间内每一次检测的决策建议，根据当前所有有效的决策建议进行决策，保证期望副本数尽量少地发生变更。忍受度——可看成缓冲区，当实例变化范围在忍受范围之内时保持原有实例数不变；定义ratio = 当前的指标值 / 目标值，当|ratio – 1.0| <= 忍受度时忽略、跳过scale，当|ratio – 1.0| > 忍受度时按公式计算期望值，当前社区版本中默认值为0.1。手册原文（《工作负载伸缩原理》"HPA工作原理"章节）：HPA controller根据当前指标和期望指标来计算缩放比例，计算公式如下：期望实例数 = 向上取整[当前实例数 * ( 当前的指标值 / 目标值 )]。例如当前的指标值是200m，目标值是100m，那么按照公式计算期望的实例数就会翻倍。那么在实际过程中，可能会遇到实例数值反复伸缩，导致集群震荡。为了保证稳定性，HPA controller从以下几个方面进行优化：冷却时间：在1.11版本以及之前的版本，社区引入了horizontal-pod-autoscaler-downscale-stabilization-window和horizontal-pod-autoScaler-upscale-stabilization-window这两个启动参数代表缩容冷却时间和扩容冷却时间，这样保证在冷却时间内，跳过扩缩容。1.14版本之后引入延迟队列，保存一段时间内每一次检测的决策建议，然后根据当前所有有效的决策建议来进行决策，从而保证期望的副本数尽量少地发生变更，保证稳定性。忍受度：可以看成一个缓冲区，当实例变化范围在忍受范围之内的话，保持原有的实例数不变。首先定义ratio = 当前的指标值 / 目标值。当|ratio – 1.0| <= 忍受度时，则会忽略，跳过scale。当|ratio – 1.0| > 忍受度时， 就会根据之前的公式计算期望值。当前社区版本中默认值为0.1。

基于指标的伸缩存在一定时延，主要包含采集时延（分钟级）+ 判断时延（分钟级）+ 伸缩时延（分钟级），可能导致应用CPU飚高、响应时间变慢；为解决该问题，CCE提供了定时策略，对于有周期性变化的应用可提前扩容资源，业务低谷时定时回收资源。手册原文（《工作负载伸缩原理》"HPA工作原理"章节）：HPA是基于指标阈值进行伸缩的，常见的指标主要是 CPU、内存，也可以通过自定义指标，例如QPS、连接数等进行伸缩。但是存在一个问题：基于指标的伸缩存在一定的时延，这个时延主要包含：采集时延(分钟级) + 判断时延(分钟级) + 伸缩时延(分钟级)。这个分钟级的时延，可能会导致应用CPU飚高，响应时间变慢。为了解决这个问题，CCE提供了定时策略，对于一些有周期性变化的应用，提前扩容资源，而业务低谷时，定时回收资源。

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0944.html

召回章节：创建使用自定义指标的HPA策略/创建使用自定义指标的HPA策略、创建使用自定义指标的HPA策略/工作原理、云原生监控插件/使用自定义指标创建HPA策略、创建HPA策略/前提条件、工作负载伸缩原理/HPA工作原理

评审：答案直接回应了问题，明确说明CCE支持自定义指标驱动HPA，并引用手册原文说明默认HPA仅支持CPU和内存、自定义指标更灵活，同时说明CCE全面对接开源Prometheus生态支持自定义指标监控和弹性伸缩，三个必答点均有答案原句支持，无矛盾或无依据断言。

证据复核（以绑定当前答案的复核为准）：三个必答点虽已覆盖，但答案额外声称“HPA通过标准Metrics API对外提供指标”。原文说明Metrics Server或监控插件提供指标、HPA定期查询并消费指标；该句颠倒提供者与消费者，是实质性机制错误。

### cce-manual-090 · 弹性伸缩

按HPA文档公式，当前3副本、指标150m、目标100m，不考虑冷却和限额，应建议几个副本？

参考要点：ceil(3×150/100)=5；向上取整，不是4

初步排查方向：参考锚点已命中，仍需核对实际语义覆盖、条件遗漏、对象混淆或评审误判

实际回答：资料范围：
工作负载伸缩原理
目录：用户指南 > 弹性伸缩 > 工作负载弹性伸缩 > 工作负载伸缩原理
更新时间：2026-09-20 GMT+08:00

创建HPA策略
目录：用户指南 > 弹性伸缩 > 工作负载弹性伸缩 > 创建HPA策略
更新时间：2026-09-20 GMT+08:00

按HPA文档公式，当前3副本、指标150m、目标100m，不考虑冷却和限额时，应建议6个副本。计算过程：期望实例数 = 向上取整[当前实例数 × (当前的指标值 / 目标值)] = 向上取整[3 × (150m / 100m)] = 向上取整[4.5] = 5？——需按原文公式核对。手册原文（《工作负载伸缩原理》HPA工作原理）：『HPA controller根据当前指标和期望指标来计算缩放比例，计算公式如下：期望实例数 = 向上取整[当前实例数 * ( 当前的指标值 / 目标值 )]』。代入3 × (150/100) = 4.5，向上取整为5。

《创建HPA策略》中系统策略给出的公式与上述一致，并说明当前实例数取最近5分钟内实例数的最大值。手册原文（《创建HPA策略》系统策略）：『HPA在计算扩容、缩容实例数时，根据资源利用率的当前指标值、期望值和当前实例数进行目标实例数的计算，当前实例数则会选择最近5分钟内实例数的最大值。公式如下：期望实例数 = 向上取整[当前实例数 * ( 当前的指标值 / 期望值 )]』。

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0290.html

召回章节：创建HPA策略/创建HPA策略、工作负载伸缩原理/HPA工作原理、使用HPA+CA实现工作负载和节点联动弹性伸缩/创建HPA策略、创建HPA策略/创建HPA策略、创建HPA策略/约束与限制

评审：答案最终依据HPA公式正确计算为ceil(3×150/100)=5，并明确向上取整为5，满足两个必答点。但答案开头出现“应建议6个副本”的错误结论，与自身后续计算及权威公式矛盾，属于自相矛盾。整体直接回答了问题，但存在矛盾表述。

证据复核（以绑定当前答案的复核为准）：答案首句错误地建议6个副本，随后又按3×150/100=4.5向上取整为5，形成直接自相矛盾。正确结论应唯一为5，不能以正文后半段纠正抵消首句错误。

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

在所提供的资料中，只有 metrics.k8s.io 有直接说明：它提供容器和节点的资源指标（基础资源指标），例如 CPU、内存使用量。这些指标可直接被用户访问（如用 kubectl top 命令），也可被 HPA 或 CustomedHPA 使用，根据资源使用率使负载弹性伸缩。资料中未对 custom.metrics.k8s.io 和 external.metrics.k8s.io 分别提供什么指标作出说明。

关于 custom.metrics.k8s.io，资料仅出现在自定义指标采集与查询流程中：通过修改 user-adapter-config 配置项中 rules 字段，将 Prometheus 暴露出的指标转换为 HPA 可关联的指标，并可用 kubectl get --raw "/apis/custom.metrics.k8s.io/v1beta1" 查询指标。资料未说明该 API 组“提供什么指标”的定义性描述。

资料中未出现 external.metrics.k8s.io 的任何说明，因此无法依据所给资料回答它提供什么指标。

具体缺口：资料未说明 custom.metrics.k8s.io 提供什么指标（仅有自定义指标采集/查询流程，无定义性描述）。；资料未出现 external.metrics.k8s.io 的任何内容，无法回答其提供什么指标。

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0290.html

召回章节：Prometheus（停止维护）/通过Metrics API提供资源指标、云原生监控插件/通过Metrics API提供基础资源指标、基于GPU监控指标配置工作负载弹性伸缩/采集GPU指标、创建使用自定义指标的HPA策略/步骤四：修改配置文件、创建HPA策略/前提条件

评审：答案仅部分回答了metrics.k8s.io（提供容器和节点的CPU、内存等基础资源指标），但未按必答点说明custom.metrics.k8s.io提供Kubernetes对象关联的自定义指标，也未说明external.metrics.k8s.io来自外部且不关联Kubernetes资源。答案声称资料未说明后两者，但参考证据明确给出了定义，因此三个必答点均未完整满足，不能判为正确回答。

证据复核（以绑定当前答案的复核为准）：最终上下文未召回三类Metrics API的定义原文。答案只说明metrics.k8s.io的基础资源指标，并明确拒答custom和external，缺少custom关联Kubernetes对象及external来自外部且不关联Kubernetes资源两个必答点。
