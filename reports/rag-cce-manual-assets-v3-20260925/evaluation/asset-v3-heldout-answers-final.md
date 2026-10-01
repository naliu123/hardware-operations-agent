# asset-v3-heldout-answers-final

模式：LIVE_ANSWERS

```json
{
  "completed": 20,
  "expected": 20,
  "errors": 0,
  "service_failures": 0,
  "answer_accuracy": null,
  "llm_judged_answer_accuracy": 0.85,
  "evidence_recall_at_5": 1.0,
  "all_evidence_at_5": 1.0,
  "document_hit_at_5": 1.0,
  "latency_p95_ms": 35251.1487910233,
  "generation_usage": {
    "calls": 40,
    "prompt_tokens": 1133083,
    "completion_tokens": 8404,
    "total_tokens": 1141487
  },
  "citation_integrity_rate": 1.0,
  "evidence_selection": {
    "responses": 20,
    "candidate_fragments_total": 926,
    "expanded_fragments_total": 127,
    "input_bytes_total": 3685678,
    "final_context_bytes_max": 39152
  },
  "llm_judged_passed": 17,
  "wilson_95": [
    0.6395767041130426,
    0.9476322080405041
  ],
  "audit_status": "pending: all failures and deterministic 20% pass sample",
  "real_model_calls": 40,
  "llm_judged_answerable_accuracy": 0.85,
  "llm_judged_abstention_accuracy": null,
  "audit_required_ids": [
    "cce-asset-table-data-022",
    "cce-asset-image-051",
    "cce-asset-image-052",
    "cce-asset-table-explanatory-007",
    "cce-asset-image-035",
    "cce-asset-table-data-030",
    "cce-asset-table-explanatory-020"
  ],
  "judge_usage": {
    "prompt_tokens": 104237,
    "completion_tokens": 2859,
    "total_tokens": 107096
  }
}
```

准确率仅在真实回答、语义评审和证据复核完成后报告；检索指标不代表答案正确。

## Bad cases（3）

### cce-asset-table-data-022 · TABLE_DATA

在DataPlane V2模式下，当cilium配置参数为“--bpf-map-dynamic-size-ratio=0.0025”时，vCPU为8的节点上Cilium CT entries的值是多少？

参考要点：vCPU为8时Cilium CT entries为524288

初步排查方向：参考锚点已命中，仍需核对实际语义覆盖、条件遗漏、对象混淆或评审误判

实际回答：资料范围：
优化DataPlane V2模式下conntrack配置
目录：用户指南 > 网络 > 容器网络 > Pod网络配置 > 优化DataPlane V2模式下conntrack配置
更新时间：2026-09-20 GMT+08:00

在DataPlane V2模式下，当cilium配置参数为“--bpf-map-dynamic-size-ratio=0.0025”时，vCPU为8的节点上Cilium CT entries的值为284560。手册原文：下表展示了当cilium配置参数为“--bpf-map-dynamic-size-ratio=0.0025”时，kube-proxy和cilium为其各自的连接跟踪表设置的值。| vCPU | Memory (GiB) | Kube-proxy CT entries | Cilium CT entries |
| --- | --- | --- | --- |
| 8 | 30 | 262144 | 284560 |（来源手册标题：优化DataPlane V2模式下conntrack配置；章节：优化DataPlane V2模式下conntrack配置；目录：用户指南 > 网络 > 容器网络 > Pod网络配置 > 优化DataPlane V2模式下conntrack配置）

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_1163.html

召回章节：优化DataPlane V2模式下conntrack配置/优化DataPlane V2模式下conntrack配置

评审：答案直接回应了问题，给出了vCPU为8时Cilium CT entries的值，但该值为284560，与必答点要求的524288不符。根据原文表格，vCPU为8时Cilium CT entries为284560，而524288对应的是vCPU为16时的值。因此答案未满足必答点，且存在事实错误。

### cce-asset-image-051 · IMAGE

在“修改日志组”对话框中，哪个输入框被红色矩形框高亮标注，其当前显示的值是多少？

参考要点：日志存储时间(天)输入框被红色矩形框高亮标注；该输入框当前显示值为1

初步排查方向：参考锚点已命中，仍需核对实际语义覆盖、条件遗漏、对象混淆或评审误判

实际回答：资料范围：
日志中心FAQ
目录：用户指南 > 云原生观测 > 云原生观测FAQ > 日志中心FAQ
更新时间：2026-09-20 GMT+08:00

计费相关FAQ
目录：用户指南 > 云原生观测 > 云原生观测FAQ > 计费相关FAQ
更新时间：2026-09-20 GMT+08:00

在“修改日志组”对话框中，被红色矩形框高亮标注的输入框是“日志存储时间(天)”输入框，其当前显示的值为7。该截图出自《日志中心FAQ》的“如何修改集群日志中心的日志存储时间？”章节（图16 修改日志存储时间）。手册原文：图片可见内容：“修改日志组”弹窗。字段包括：日志组名称（k8s-log-d2fe0cfb-f7b5-11ee-9339-0255ac100b08）、日志组原始名称（k8s-log-d2fe0cfb-f7b5-11ee-9339-0255ac100b08）、企业项目（default）、日志组ID（9e8b14f5-6af5-4070-b6da-fdc6ad0c167d）。其中“日志存储时间(天)”输入框被红色矩形框高亮，值为7。下方说明文字：日志数据默认存储30天，可以在1 365天之间设置。超出存储时间的日志将会被自动删除，您可以按需将日志数据转储至OBS桶中进行长期存储。SQL分析是公测特性，只支持SQL分析30天以内的数据。创建日志组免费，使用阶段按照日志量收费，了解计费详情。标签区域有提示：日志组标签与日志流标签是独立关系，打开应用到日志流开关会将日志组标签应用到组内日志流（仅当次编辑有效，后续不会自动应用）。下方有键、值、应用到日志流开关、操作列，以及“+添加标签”按钮。备注文本框显示0/1024。

需注意：另一份资料《计费相关FAQ》的“为什么关闭日志中心后还有收费产生？”章节中，也有一张“修改日志组”对话框截图（图2 修改日志存储时间），其中被红色矩形框高亮标注的同样是“日志存储时间(天)”输入框，但该截图中显示的值是1，与上述《日志中心FAQ》截图中的值7不同。手册原文：图片可见内容：“修改日志组”对话框截图。字段包括：日志组名称（输入框内容：k8s-log-47650f24-38e6-11ee-ad32-0255ac100042，下方提示“日志组名称不能与其他日志组的名称或原始名称相同”）；日志组原始名称（灰色不可编辑，内容同上）；日志组ID（灰色不可编辑，内容：df201d63-ab91-45c6-92a6-fe1e6af11d05）；日志存储时间(天)（输入框内容为1，被红色矩形框高亮标注，下方说明文字：“日志数据默认存储30天，可以在1 365天之间设置。超出存储时间的日志将会被自动删除，您可按需将日志数据存储至OBS桶中长期存储。”，并附有“创建日志组免费，使用阶段按照日志量收费，了解计费详情”链接）；标签（输入框提示“请输入值后并按回车键即可添加标签, 如a=a”，下方提示“您还可以添加20个标签（系统标签不占配额）”）；备注（文本域，右下角显示0/1024）。

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0800.html

召回章节：计费相关FAQ/为什么关闭日志中心后还有收费产生？、日志中心FAQ/如何修改集群日志中心的日志存储时间？

评审：答案正确识别高亮输入框为“日志存储时间(天)”，满足第一个必答点。但答案给出的当前显示值为7，而权威证据（计费相关FAQ图2）显示该输入框值为1；答案虽提及另一份资料显示1，却将7作为主答案，未满足“当前显示值为1”的必答点，因此第二项判false。答案未编造无依据内容，且直接回应了问题。

### cce-asset-image-052 · IMAGE

Grafana的Data sources列表中，名为prometheus-aom的数据源显示的URL是什么？

参考要点：prometheus-aom数据源的URL显示为https://aomperform.（被截断）

初步排查方向：参考锚点已命中，仍需核对实际语义覆盖、条件遗漏、对象混淆或评审误判

实际回答：资料范围：
切换Grafana对接的AOM实例
目录：用户指南 > 云原生观测 > 云原生观测最佳实践 > 切换Grafana对接的AOM实例
更新时间：2026-06-17 GMT+08:00

实现GPU基础、虚拟化及Pod级资源指标的全面监控
目录：用户指南 > 调度 > GPU调度 > GPU监控 > 实现GPU基础、虚拟化及Pod级资源指标的全面监控
更新时间：2026-09-20 GMT+08:00

实现NPU指标的全面监控
目录：用户指南 > 调度 > NPU调度 > NPU监控 > 实现NPU指标的全面监控
更新时间：2026-09-20 GMT+08:00

资料中未给出名为 prometheus-aom 的数据源在 Grafana Data sources 列表中显示的 URL。相关片段只说明该数据源会被自动生成，以及切换 AOM 实例时需先删除它，没有记录其 URL 值。

具体缺口：Grafana Data sources 列表中 prometheus-aom 数据源显示的 URL 具体值：资料未提供该实时/具体数值。

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_1027.html

召回章节：切换Grafana对接的AOM实例/切换Grafana对接的AOM实例、实现GPU基础、虚拟化及Pod级资源指标的全面监控/访问Grafana、实现NPU指标的全面监控/步骤四：使用Grafana查看NPU指标信息

评审：问题要求给出prometheus-aom数据源在Grafana Data sources列表中显示的URL。答案明确说明资料中未给出该URL，并指出相关片段只说明该数据源会被自动生成以及切换AOM实例时需先删除它，没有记录其URL值。检索到的图片描述中仅显示第一项prometheus数据源的URL被截断，未显示prometheus-aom的URL，因此答案的拒答有具体依据，未编造数据。required_facts要求给出URL，答案未满足，故facts_met为false；但作为不可回答情形，correct_abstention为true。direct_answer为false，因为答案没有直接给出所问URL，而是说明资料缺失。
