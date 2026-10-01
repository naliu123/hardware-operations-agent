# asset-v2-baseline

模式：REAL_RETRIEVAL_ONLY

```json
{
  "completed": 40,
  "expected": 40,
  "errors": 0,
  "service_failures": 0,
  "answer_accuracy": null,
  "llm_judged_answer_accuracy": null,
  "evidence_recall_at_5": 0.725,
  "all_evidence_at_5": 0.725,
  "document_hit_at_5": 0.825,
  "latency_p95_ms": 4160.83737500594,
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

## Bad cases（11）

### cce-asset-table-explanatory-005 · TABLE_EXPLANATORY

对象存储卷PV配置中，参数 storageClassName 用于指定什么？需配置为什么值？

参考要点：用于指定K8s storage class名称。；需配置为“csi-obs”。

初步排查方向：未命中全部参考锚点，核对等价证据、分段及候选排序

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0343.html

召回章节：通过静态存储卷使用已有对象存储/使用已有对象存储、通过静态存储卷使用专属存储/使用已有专属存储、自定义存储类（StorageClass）/StorageClass高级配置、对象存储卷挂载设置自定义访问密钥（AK/SK）/在对象存储卷中挂载Secret、设置对象存储挂载参数/配置挂载参数

### cce-asset-table-explanatory-011 · TABLE_EXPLANATORY

对于Huawei Cloud EulerOS 2.0操作系统，集群版本v1.36支持CCE Standard集群的哪些网络模型以及CCE Turbo集群的哪种网络模型？

参考要点：支持CCE Standard集群的VPC网络模型、容器隧道网络模型；支持CCE Turbo集群的云原生网络2.0

初步排查方向：未命中全部参考锚点，核对等价证据、分段及候选排序

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0476.html

召回章节：节点操作系统说明/裸金属服务器、配置网络策略（NetworkPolicy）限制Pod访问的对象/网络策略支持的集群对比、购买Standard/Turbo集群/步骤二：进行网络配置、集群类型对比/集群类型对比、CCE补丁版本发布记录/v1.25版本

### cce-asset-table-data-023 · TABLE_DATA

aC8型弹性云服务器规格表中，ac8.12xlarge.4规格的vCPU和内存分别是多少？

参考要点：ac8.12xlarge.4的vCPU为48核；内存为192 GiB

初步排查方向：未命中全部参考锚点，核对等价证据、分段及候选排序

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0719.html

召回章节：节点规格说明/AI加速型、节点规格说明/通用计算增强型、节点规格说明/AI加速型、节点规格说明/鲲鹏超高I/O型、节点规格说明/AI加速型

### cce-asset-image-032 · IMAGE

在“负载监听器配置”表单的“更多配置”区域中，安全策略下拉框当前选中的具体策略名称是什么？

参考要点：安全策略下拉框选择“安全策略 tls-1-2”。

初步排查方向：未命中全部参考锚点，核对等价证据、分段及候选排序

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0842.html

召回章节：使用安全组策略为工作负载绑定安全组/创建安全组策略、为负载均衡类型的Service配置黑名单/白名单访问策略/创建负载均衡并配置黑名单/白名单访问策略、为负载均衡类型的Service配置QUIC监听器/约束与限制、为负载均衡类型的Service配置TLS/创建负载均衡并配置TLS、为ELB Ingress配置QUIC监听器/配置QUIC监听器

### cce-asset-image-037 · IMAGE

在“从 DryRun 到 Execute：先评估计划，再执行受控迁移”流程图中，第③步“执行与验证”的流程箭头依次经过哪些环节？

参考要点：Eviction → 重建 Pod → nominatedNodeName → Scheduler 实际绑定；status.result 记录实际收益；status.relocations 记录逐 Pod 过程

初步排查方向：未命中全部参考锚点，核对等价证据、分段及候选排序

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_11252.html

召回章节：Volcano异构资源碎片整理/步骤二：（可选）准备工作负载与节点标签 / 步骤三：按需增加Scope / 步骤四：加入收益门槛和执行预算 / 步骤五：创建独立Execute Run / 步骤六：验证Execute是否生效 / 步骤七：在整理后的节点上提交大型训练任务 / 完整RepackRun CR示例、NGINX Ingress迁移至Envoy Gateway/使用ingress2eg进行迁移示例、Volcano异构资源碎片整理/以Gang语义评估工作负载中断影响 / 动态选择腾空目标，并抑制二次碎片 / 通过执行预算限制单次影响范围 / 通过nominatedNodeName提供候选节点建议 / 从方案评估到结果验证形成闭环 / 工作负载驱逐与重建机制 / 操作示例 / 步骤一：执行DryRun评估、Nginx Ingress迁移到ELB Ingress/迁移流程、NGINX Ingress迁移至Envoy Gateway/使用ingress2gateway进行迁移示例

### cce-asset-image-040 · IMAGE

日志查看界面中，当前选中的时间范围按钮是哪个，其右侧紧邻的按钮名称是什么？

参考要点：当前选中的时间范围按钮是“近1小时”（蓝色高亮）；其右侧紧邻的按钮是“高级搜索”下拉按钮

初步排查方向：未命中全部参考锚点，核对等价证据、分段及候选排序

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0395.html

召回章节：使用仪表盘/查看/切换视图、日志中心FAQ/如何修改集群日志中心的日志存储时间？、事件监控/事件、管理工作负载/事件、日志中心FAQ/如何关闭日志中心？

### cce-asset-image-041 · IMAGE

在“负载均衡配置”区域中，下拉框当前显示的负载均衡实例名称是什么？

参考要点：cie-test

初步排查方向：未命中全部参考锚点，核对等价证据、分段及候选排序

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0843.html

召回章节：为负载均衡类型的Service配置黑名单/白名单访问策略/创建负载均衡并配置黑名单/白名单访问策略、为负载均衡类型的Service配置HTTP/2/步骤一：部署示例应用、为负载均衡类型的Service配置区间端口监听/创建负载均衡并配置区间端口监听、为负载均衡类型的Service配置HTTP/HTTPS头字段/创建负载均衡并配置HTTP/HTTPS头字段、Envoy Gateway/安装插件

### cce-asset-image-054 · IMAGE

在“编辑网段”对话框中，哪个按钮被红框标注？

参考要点：“添加IPv4扩展网段”按钮被红框标注

初步排查方向：未命中全部参考锚点，核对等价证据、分段及候选排序

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0387.html

召回章节：管理节点污点/一键设置节点调度策略、事件监控/事件、管理节点标签/添加/更新或删除节点标签、通过节点标签配置GPU驱动版本/操作步骤、设置标签与注解/Pod标签

### cce-asset-image-055 · IMAGE

在“委托 / 创建委托”页面中，委托名称输入框填写的值是什么？

参考要点：委托名称输入框的值为agency_for_cce_service_account

初步排查方向：未命中全部参考锚点，核对等价证据、分段及候选排序

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_1091.html

召回章节：在CCE集群中使用容器组身份（Pod Identity）获取IAM凭证/在Pod中配置使用Pod Identity、系统委托说明/CCENodeAgency委托说明、更新节点池/更新节点池、集群自定义委托说明/前提条件、创建节点池/操作步骤

### cce-asset-image-056 · IMAGE

CCE集群概览页面的“网络信息”面板中，转发模式字段显示的值是什么？

参考要点：转发模式字段显示为iptables

初步排查方向：未命中全部参考锚点，核对等价证据、分段及候选排序

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0437.html

召回章节：修改CCE集群配置/网络组件配置（eni）（仅VPC网络模型的集群支持）、扩展VPC网络集群的容器网段/为CCE Standard集群添加容器网段、网络概述/网络概述、自定义存储类（StorageClass）/StorageClass高级配置、GPU虚拟化优先级抢占调度/约束与限制

### cce-asset-image-058 · IMAGE

服务列表界面中，名为envoy-default-inference-pool-with-aigwroute-d416582c的服务，其访问端口->容器端口/协议列显示的内容是什么？

参考要点：访问端口->容器端口/协议列显示80->10080/TCP；下方还显示30961/TCP

初步排查方向：未命中全部参考锚点，核对等价证据、分段及候选排序

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_1161.html

召回章节：集群内访问（ClusterIP）/创建ClusterIP类型Service、通过Envoy AI Gateway实现大模型推理流量路由/操作步骤、通过Envoy Gateway提供服务访问/自定义EnvoyProxy来为Gateway配置已有ELB、为负载均衡类型的Service配置TLS/创建负载均衡并配置TLS、通过Envoy Gateway提供服务访问/前提条件
