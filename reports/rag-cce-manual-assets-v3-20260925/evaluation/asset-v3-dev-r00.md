# asset-v3-dev-r00

模式：REAL_RETRIEVAL_ONLY

```json
{
  "completed": 40,
  "expected": 40,
  "errors": 40,
  "service_failures": 0,
  "answer_accuracy": null,
  "llm_judged_answer_accuracy": null,
  "evidence_recall_at_5": 0.0,
  "all_evidence_at_5": 0.0,
  "document_hit_at_5": 0.0,
  "latency_p95_ms": 696.9868339947425,
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

## Bad cases（40）

### cce-asset-table-explanatory-002 · TABLE_EXPLANATORY

使用本地临时卷时，参数“挂载路径”应输入什么？对挂载目录有什么要求？

参考要点：需输入挂载路径，如：/tmp；数据存储挂载到容器上的路径。；请不要挂载在系统目录下，如“/”、“/var/run”等，会导致容器异常。；建议挂载在空目录下，若目录不为空，请确保目录下无影响容器启动的文件，否则文件会被替换，导致容器启动异常，工作负载创建失败。

初步排查方向：服务/依赖错误

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0726.html

召回章节：

### cce-asset-table-explanatory-004 · TABLE_EXPLANATORY

单集群视角的成本洞察中，命名空间估算成本&资源消耗汇总功能包含的“运行总核时”指的是什么？

参考要点：即所选时间周期内，命名空间总消耗的核时资源数。

初步排查方向：服务/依赖错误

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0879.html

召回章节：

### cce-asset-table-explanatory-005 · TABLE_EXPLANATORY

对象存储卷PV配置中，参数 storageClassName 用于指定什么？需配置为什么值？

参考要点：用于指定K8s storage class名称。；需配置为“csi-obs”。

初步排查方向：服务/依赖错误

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0343.html

召回章节：

### cce-asset-table-explanatory-006 · TABLE_EXPLANATORY

kube-apiserver组件监控指标中，apiserver_flowcontrol_current_executing_seats 是什么类型的指标？它表示什么含义？

参考要点：是一个 Gauge 类型的指标。；表示某个优先级队列中当前已占用的并发资源量，反映了当前队列中正在消耗的并发资源，可以了解队列的实际负载情况。

初步排查方向：服务/依赖错误

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0559.html

召回章节：

### cce-asset-table-explanatory-009 · TABLE_EXPLANATORY

在VPC集群（开启DataPlane V2替换kube-proxy）和CCE Turbo集群（开启DataPlane V2替换kube-proxy）中，对于独享型负载均衡类型Service且访问类型为私网，当客户端与服务Pod同节点时，访问情况如何？

参考要点：无法访问。

初步排查方向：服务/依赖错误

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0249.html

召回章节：

### cce-asset-table-explanatory-010 · TABLE_EXPLANATORY

AI数据加速引擎插件使用示例中，规格配置参数包含哪些信息？示例中的基础版内存规格是多少？

参考要点：规格配置参数包含产品类型-内存规格、版本号、实例类型等信息。；示例为基础版-16GB。

初步排查方向：服务/依赖错误

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0999.html

召回章节：

### cce-asset-table-explanatory-011 · TABLE_EXPLANATORY

对于Huawei Cloud EulerOS 2.0操作系统，集群版本v1.36支持CCE Standard集群的哪些网络模型以及CCE Turbo集群的哪种网络模型？

参考要点：支持CCE Standard集群的VPC网络模型、容器隧道网络模型；支持CCE Turbo集群的云原生网络2.0

初步排查方向：服务/依赖错误

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0476.html

召回章节：

### cce-asset-table-explanatory-013 · TABLE_EXPLANATORY

在Kubernetes社区v1.25版本中，资源Event的废弃API版本是什么？其替代API版本是什么？该替代API从社区哪个版本开始可用？

参考要点：废弃API版本为events.k8s.io/v1beta1；替代API版本为events.k8s.io/v1；该API从社区v1.19版本开始可用

初步排查方向：服务/依赖错误

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0302.html

召回章节：

### cce-asset-table-explanatory-014 · TABLE_EXPLANATORY

在云审计服务支持的CCE操作列表中，操作名称为“查询指定分区”时，其资源维度和对应的事件名称分别是什么？

参考要点：资源维度为“集群”；对应的事件名称为“GetOnePartition”

初步排查方向：服务/依赖错误

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0025.html

召回章节：

### cce-asset-table-explanatory-015 · TABLE_EXPLANATORY

在K8s节点污点检查异常处理中，检查节点上是否存在集群升级需要使用到的污点，该污点的名称和污点影响分别是什么？

参考要点：污点名称为node.kubernetes.io/upgrade；污点影响为NoSchedule

初步排查方向：服务/依赖错误

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0460.html

召回章节：

### cce-asset-table-explanatory-016 · TABLE_EXPLANATORY

在CCE密钥管理（对接DEW）的基础挂载配置中，objects参数下的objectName参数是什么类型？是否为必选参数？它表示什么？

参考要点：objectName参数为String类型；是必选参数；表示凭据名称，需填写ServiceAccount中引用的凭据

初步排查方向：服务/依赖错误

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0370.html

召回章节：

### cce-asset-table-explanatory-017 · TABLE_EXPLANATORY

在创建普通任务（Job）时，初始化容器（可选）参数用于选择容器是否作为初始化（Init）容器，初始化（Init）容器是否支持设置健康检查？

参考要点：初始化（Init）容器不支持设置健康检查

初步排查方向：服务/依赖错误

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0150.html

召回章节：

### cce-asset-table-explanatory-018 · TABLE_EXPLANATORY

在CCE有状态负载中动态挂载本地持久卷时，填写挂载路径的“权限”参数，只读和读写分别有什么含义？

参考要点：只读：只能读容器路径中的数据卷；读写：可修改容器路径中的数据卷，容器迁移时新写入的数据不会随之迁移，会造成数据丢失

初步排查方向：服务/依赖错误

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0635.html

召回章节：

### cce-asset-table-data-021 · TABLE_DATA

Envoy Gateway插件版本记录表中，插件版本1.0.35对应的社区版本是什么？

参考要点：插件版本1.0.35对应社区版本1.7.4

初步排查方向：服务/依赖错误

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_1155.html

召回章节：

### cce-asset-table-data-023 · TABLE_DATA

aC8型弹性云服务器规格表中，ac8.12xlarge.4规格的vCPU和内存分别是多少？

参考要点：ac8.12xlarge.4的vCPU为48核；内存为192 GiB

初步排查方向：服务/依赖错误

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0719.html

召回章节：

### cce-asset-table-data-024 · TABLE_DATA

Volcano队列资源规划表中，queue-team-b的初始任务请求在CPU、内存和GPU维度分别是多少？

参考要点：queue-team-b初始任务请求为2 CPU、4 GiB、1 GPU

初步排查方向：服务/依赖错误

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_11521.html

召回章节：

### cce-asset-table-data-025 · TABLE_DATA

Velero插件大规模场景内存建议表中，当集群资源数量为200000时，建议的内存申请值和限制值分别是多少？

参考要点：集群资源数量为200000时建议内存申请值为4000Mi；建议内存限制值为16000Mi

初步排查方向：服务/依赖错误

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_1162.html

召回章节：

### cce-asset-table-data-026 · TABLE_DATA

默认资源配额表中，对于1000节点集群，Pod和Deployment的配额分别是多少？

参考要点：1000节点集群Pod配额为5000；Deployment配额为2000

初步排查方向：服务/依赖错误

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0287.html

召回章节：

### cce-asset-table-data-028 · TABLE_DATA

对象存储常驻进程内存消耗表中，4并发写10M文件时的内存消耗约为多少？

参考要点：4并发写10M文件时内存消耗约为220m

初步排查方向：服务/依赖错误

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0628.html

召回章节：

### cce-asset-table-data-029 · TABLE_DATA

CPU Burst性能对比表中，开启CPU Burst时p99时延和nr_bursts突破limit值次数分别是多少？

参考要点：开启CPU Burst时p99时延为456us；nr_bursts突破limit值次数为469

初步排查方向：服务/依赖错误

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0700.html

召回章节：

### cce-asset-image-031 · IMAGE

在“立即恢复”页面中，哪个表单字段的下方说明文字提到支持通配符如 app-*？

参考要点：恢复命名空间字段的下方说明文字提到支持通配符如 app-*。

初步排查方向：服务/依赖错误

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_1160.html

召回章节：

### cce-asset-image-032 · IMAGE

在“负载监听器配置”表单的“更多配置”区域中，安全策略下拉框当前选中的具体策略名称是什么？

参考要点：安全策略下拉框选择“安全策略 tls-1-2”。

初步排查方向：服务/依赖错误

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0842.html

召回章节：

### cce-asset-image-033 · IMAGE

在子网可用IP余量监控原理图中，标注“3.指标聚合”的箭头连接了哪些AOM框内的实例？

参考要点：实例A和实例B通过向上箭头指向实例C；标注为“3.指标聚合”

初步排查方向：服务/依赖错误

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_1066.html

召回章节：

### cce-asset-image-036 · IMAGE

在标题为“Hubble”的Grafana看板截图中，“Top 10 Port Distribution”折线图图例里显示的5443/TCP端口速率是多少？

参考要点：5443/TCP 4.64 p/s

初步排查方向：服务/依赖错误

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_1063.html

召回章节：

### cce-asset-image-037 · IMAGE

在“从 DryRun 到 Execute：先评估计划，再执行受控迁移”流程图中，第③步“执行与验证”的流程箭头依次经过哪些环节？

参考要点：Eviction → 重建 Pod → nominatedNodeName → Scheduler 实际绑定；status.result 记录实际收益；status.relocations 记录逐 Pod 过程

初步排查方向：服务/依赖错误

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_11252.html

召回章节：

### cce-asset-image-038 · IMAGE

在“负载均衡配置”界面中，“负载均衡器”下哪个按钮处于蓝色选中状态？

参考要点：“选择已有”按钮处于蓝色选中状态。

初步排查方向：服务/依赖错误

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0831.html

召回章节：

### cce-asset-image-040 · IMAGE

日志查看界面中，当前选中的时间范围按钮是哪个，其右侧紧邻的按钮名称是什么？

参考要点：当前选中的时间范围按钮是“近1小时”（蓝色高亮）；其右侧紧邻的按钮是“高级搜索”下拉按钮

初步排查方向：服务/依赖错误

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0395.html

召回章节：

### cce-asset-image-041 · IMAGE

在“负载均衡配置”区域中，下拉框当前显示的负载均衡实例名称是什么？

参考要点：cie-test

初步排查方向：服务/依赖错误

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0843.html

召回章节：

### cce-asset-image-043 · IMAGE

环境变量配置表格中，第5行“资源引用”类型对应的变量名称是什么？

参考要点：key4

初步排查方向：服务/依赖错误

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0113.html

召回章节：

### cce-asset-image-044 · IMAGE

“指标观测”区域中，开关右侧显示的文字是什么？

参考要点：开启

初步排查方向：服务/依赖错误

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0971.html

召回章节：

### cce-asset-image-046 · IMAGE

“容器配置”页面中，镜像名称输入框右侧的按钮文字是什么？

参考要点：更换镜像

初步排查方向：服务/依赖错误

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0009.html

召回章节：

### cce-asset-image-047 · IMAGE

代码编辑器截图中，data字段下配置行的键名是什么？

参考要点：custom_535.216.03

初步排查方向：服务/依赖错误

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_1113.html

召回章节：

### cce-asset-image-053 · IMAGE

迁移流程示意图中，第二个蓝色步骤框的标题文字是什么？

参考要点：第二个蓝色步骤框的标题文字为“将流量逐渐转移至ELB Ingress”

初步排查方向：服务/依赖错误

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0858.html

召回章节：

### cce-asset-image-054 · IMAGE

在“编辑网段”对话框中，哪个按钮被红框标注？

参考要点：“添加IPv4扩展网段”按钮被红框标注

初步排查方向：服务/依赖错误

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0387.html

召回章节：

### cce-asset-image-055 · IMAGE

在“委托 / 创建委托”页面中，委托名称输入框填写的值是什么？

参考要点：委托名称输入框的值为agency_for_cce_service_account

初步排查方向：服务/依赖错误

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_1091.html

召回章节：

### cce-asset-image-056 · IMAGE

CCE集群概览页面的“网络信息”面板中，转发模式字段显示的值是什么？

参考要点：转发模式字段显示为iptables

初步排查方向：服务/依赖错误

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0437.html

召回章节：

### cce-asset-image-057 · IMAGE

AOM指标浏览页面的表格中，destination_workload为coredns的指标行，其当前值显示为多少？

参考要点：destination_workload为coredns的指标行当前值显示为40.47

初步排查方向：服务/依赖错误

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_1171.html

召回章节：

### cce-asset-image-058 · IMAGE

服务列表界面中，名为envoy-default-inference-pool-with-aigwroute-d416582c的服务，其访问端口->容器端口/协议列显示的内容是什么？

参考要点：访问端口->容器端口/协议列显示80->10080/TCP；下方还显示30961/TCP

初步排查方向：服务/依赖错误

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_1161.html

召回章节：

### cce-asset-image-059 · IMAGE

ELB后端服务器组页面中，表格里两条记录的权重值分别是多少？

参考要点：两条记录的权重均为10

初步排查方向：服务/依赖错误

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0682.html

召回章节：

### cce-asset-image-060 · IMAGE

在“节点伸缩策略”页签的策略列表中，点击“更多”后展开的下拉菜单里哪个选项被红色矩形框标出？

参考要点：“删除”选项被红色矩形框标出

初步排查方向：服务/依赖错误

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0063.html

召回章节：
