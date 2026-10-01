# Kubernetes 1.35版本说明

来源：https://support.huaweicloud.com/usermanual-cce/cce_bulletin_0117.html
目录：用户指南 > 集群 > 集群版本发布说明 > Kubernetes版本发布记录 > Kubernetes 1.35版本说明
更新时间：2026-06-05 GMT+08:00

云容器引擎（CCE）严格遵循社区一致性认证，现已支持Kubernetes 1.35集群版本特性。本文介绍Kubernetes 1.35版本的变更说明。

#### 索引

- [新增特性及特性增强](https://support.huaweicloud.com/usermanual-cce/cce_bulletin_0117.html)
- [API变更与弃用](https://support.huaweicloud.com/usermanual-cce/cce_bulletin_0117.html)
- [CCE对Kubernetes 1.35版本的增强](https://support.huaweicloud.com/usermanual-cce/cce_bulletin_0117.html)
- [参考链接](https://support.huaweicloud.com/usermanual-cce/cce_bulletin_0117.html)

#### 新增特性及特性增强

- PreferSameNode traffic distribution （GA）

  在Kubernetes 1.35中，PreferSameNode traffic distribution进阶至GA。该特性新增 PreferSameNode 策略，使服务流量优先转发至与客户端同一节点的端点；同时废弃 PreferClose，改用 PreferSameNode 替代原有行为。详细用法请参考[同节点优先流量分发](https://github.com/kubernetes/enhancements/blob/master/keps/sig-network/3015-prefer-same-node/README.md)。
- Support managedBy field for Jobs（GA）

  在Kubernetes 1.35中，Support managedBy field for Jobs进阶至GA。此功能在Jobs中引入了.spec.managedBy。您可以通过该字段指定外部控制器（如Kueue、MultiKueue等），将Job的同步逻辑与状态更新委托给指定的外部控制器。详细使用方式请参考[Job的managedBy字段支持](https://kubernetes.io/zh-cn/blog/2025/12/18/kubernetes-v1-35-job-managedby-for-jobs-goes-ga/)。
- Pod Generation（GA）

  在 Kubernetes 1.35 中，Pod Generation 进阶至 GA。此功能为 Pod 启用metadata.generation并在 PodSpec 更新时自动递增，同时新增status.observedGeneration字段，由 kubelet 上报以标识已观测到的 Pod 配置版本。详细介绍请参考 [Pod Generation 特性支持](https://github.com/kubernetes/enhancements/tree/master/keps/sig-node/5067-pod-generation)。
- TopologyManager max-allowable-numa-nodes（GA）

  在 Kubernetes 1.35 中，TopologyManager max-allowable-numa-nodes 进阶至 GA。此功能为拓扑管理器新增max-allowable-numa-nodes策略选项，支持自定义配置最大允许 NUMA 节点数，解除原有 8 个的硬编码限制，兼容搭载超过 8 个 NUMA 节点的高端 CPU 与 AI 加速芯片。详细使用方式请参考[TopologyManager NUMA节点上限配置](https://github.com/kubernetes/enhancements/blob/master/keps/sig-node/4622-topologymanager-max-allowable-numa-nodes/README.md)。
- Node Topology Downward API（Beta）

  在 Kubernetes 1.35 中，Node Topology Downward API 进阶至 Beta。此功能通过内置PodTopologyLabels准入插件，将节点标准拓扑标签同步至 Pod，支持通过 Downward API 供容器直接获取区域、可用区、主机名等拓扑信息，无需特权初始化容器。详细介绍请参考[节点拓扑信息 Downward API 暴露](https://github.com/kubernetes/enhancements/blob/master/keps/sig-node/4742-node-topology-downward-api/README.md)。
- Storage Version Migrator in-tree（Beta）

  在 Kubernetes 1.35 中，Storage Version Migrator in-tree 进阶至 Beta。此功能将存储版本迁移器（SVM）从外部组件移入内核并集成到 kube-controller-manager，提供标准化StorageVersionMigration API。详细使用方式请参考[存储版本迁移器内核集成](https://github.com/kubernetes/enhancements/blob/master/keps/sig-api-machinery/4192-svm-in-tree/README.md)。
- Mutable CSINode Allocatable（Beta）

  在 Kubernetes 1.35 中，Mutable CSINode Allocatable 进阶至 Beta。此功能支持动态修改CSINode.Spec.Drivers[\*].Allocatable.Count字段，在卷挂载因容量不足失败时自动修正容量信息，避免调度器基于过时数据导致 Pod 卡住。详细介绍请参考[可变更 CSINode 可分配容量特性](https://github.com/kubernetes/enhancements/blob/master/keps/sig-storage/4876-mutable-csinode-allocatable/README.md)。
- Opportunistic batching（Beta）

  在Kubernetes 1.35中，此功能为调度器新增机会式批处理机制，通过Pod 调度签名复用调度结果，提升同类批量 Pod 的调度效率。详细介绍请参考调[度器机会式批处理特性](https://github.com/kubernetes/enhancements/blob/master/keps/sig-scheduling/5598-opportunistic-batching/README.md)。
- StatefulSet maxUnavailable（Beta）

  在 Kubernetes 1.35 中，StatefulSet maxUnavailable 进阶至 Beta。此功能为 StatefulSet 滚动更新策略新增maxUnavailable字段，支持配置更新时最大不可用 Pod 数（支持数值或百分比）。详细使用方式请参考 [StatefulSet 滚动更新 maxUnavailable 特性](https://github.com/kubernetes/enhancements/blob/master/keps/sig-apps/961-maxunavailable-for-statefulset/README.md)。
- KYAML（Beta）

  在 Kubernetes 1.35 中，KYAML 进阶至 Beta。此功能为 kubectl 新增kyaml输出格式（-o kyaml），兼容标准 YAML 解析器。详细使用方式请参考 [KYAML 安全输出格式](https://github.com/kubernetes/enhancements/blob/master/keps/sig-cli/5295-kyaml/README.md)。
- Configurable HPA tolerance（Beta）

  在 Kubernetes 1.35 中，Configurable HPA tolerance 进阶至 Beta。此功能为原生HPA 扩缩容策略新增tolerance配置项。详细使用方式请参考 [HPA 可配置扩缩容容忍度特性](https://github.com/kubernetes/enhancements/blob/master/keps/sig-autoscaling/4951-configurable-hpa-tolerance/README.md)。
- Support User Namespaces（Beta）

  在 Kubernetes 1.35 中，User Namespaces Support 进阶至 Beta。此功能通过pod.spec.hostUsers字段启用用户命名空间隔离，提升 Pod 安全隔离性。详细使用方式请参考[用户命名空间支持特性](https://github.com/kubernetes/enhancements/blob/master/keps/sig-node/127-user-namespaces/README.md)。
- OCI Volume Source（Beta）

  在 Kubernetes 1.35 中，此功能为 Pod 新增image类型卷源，支持直接将 OCI 镜像 / 制品挂载为只读卷。使用此功能时，containerd 需为 v2.1 或更高版本，详细使用方式请参考 [OCI 卷源支持](https://github.com/kubernetes/enhancements/tree/master/keps/sig-node/4639-oci-volume-source)。
- Ensure Secret Pulled Images（Beta）

  在 Kubernetes 1.35 中，Ensure Secret Pulled Images 进阶至 Beta。此功能由 kubelet 缓存镜像拉取凭证并强制验证访问权限，针对 IfNotPresent/Never 拉取策略校验 Pod 是否有权使用节点已有镜像。详细介绍请参考[镜像拉取凭证强制验证特性](https://github.com/kubernetes/enhancements/blob/master/keps/sig-node/2535-ensure-secret-pulled-images/README.md)。
- Container Restart Rules（Beta）

  在 Kubernetes 1.35 中，Container Restart Rules 进阶至 Beta。此功能为容器新增restartPolicy与restartPolicyRules配置，可覆盖 Pod 级重启策略。详细使用方式请参考[容器重启规则特性](https://github.com/kubernetes/enhancements/blob/master/keps/sig-node/5307-container-restart-policy/README.md)。
- CSI driver service account tokens via secrets field（Beta）

  在 Kubernetes 1.35 中，此功能支持将 CSIDriver 对象中的spec.serviceAccountTokenInSecrets字段设为true，以支持将服务账号令牌通过 CSI 请求的专用 secrets 字段传递，替代volume\_context 字段，避免敏感令牌泄露。详细说明请参考 [CSI 驱动服务账号令牌安全传递特性](https://github.com/kubernetes/enhancements/blob/master/keps/sig-storage/5538-csi-sa-tokens-secrets-field/README.md)。
- Consider Terminating Pods in Deployments（Beta）

  在 Kubernetes 1.35 中，Consider Terminating Pods in Deployments 进阶至 Beta。此功能为 Deployment 与 ReplicaSet 新增status.terminatingReplicas字段，统计处于终止中的 Pod 数量。详细介绍请参考 [Deployment 终止副本统计特性](https://github.com/kubernetes/enhancements/blob/master/keps/sig-apps/3973-consider-terminating-pods-deployment/README.md)。

#### API变更与弃用

- 自 Kubernetes 1.35 起，cgroupv1被标记为弃用并默认不支持。CCE集群在1.35集群支持[cgroupv2](https://kubernetes.io/docs/concepts/architecture/cgroups/)并兼容cgroupv1节点操作系统。
- 自 Kubernetes 1.35 起，kube-proxy 的 ipvs 模式已被标记为弃用，并计划在未来版本中移除。CCE集群自1.35版本起，新增支持并默认使用nftables转发模式。详见[nftables转发模式介绍](https://support.huaweicloud.com/usermanual-cce/cce_10_0349.html)。
- Kubernetes 1.35是最后一个支持containerd 1.x的集群版本。CCE集群自1.35版本起，默认使用containerd 2.x。
- 在 Kubernetes 1.35 中，为 CustomResourceDefinition 的 conditions 字段添加了 ObservedGeneration
- 在 Kubernetes 1.35 中，新增 StorageVersionMigration v1beta1 API 并移除 v1alpha1 API，升级前必须删除所有 v1alpha1 资源。
- 在 Kubernetes 1.35 中，更新 kuberc 配置架构，新增 credPluginPolicy 和 credPluginAllowlist 两个可选配置字段。
- 在 Kubernetes 1.35 中，DRA 核心功能特性门控已锁定为默认启用且无法再被关闭。
- 在 Kubernetes 1.35 中，默认启用 kubectl get -o kyaml，可通过设置 KUBECTL\_KYAML=false 关闭该行为。
- 在 Kubernetes 1.35 中，启用 Pod 级资源原地调整，PodStatus 中新增 Resources 与 AllocatedResources 字段分别记录 Pod 级 cgroup 资源与申请资源。
- 在 Kubernetes 1.35 中，ReplicaSet 与 Deployment 的 status.terminatingReplicas 统计特性升级至 Beta 并默认启用。

  在 Kubernetes 1.35 中，JobManagedBy 特性升级至 GA，特性门控锁定为 true 并将在后续版本移除。

#### CCE对Kubernetes 1.35版本的增强

在版本维护周期中，CCE会对Kubernetes 1.35版本进行定期的更新，并提供功能增强。

关于CCE集群版本的更新说明，请参见[补丁版本发布说明](https://support.huaweicloud.com/usermanual-cce/cce_10_0405.html)。

#### 参考链接

关于Kubernetes 1.35与其他版本的性能对比和功能演进的更多信息，请参考：[Kubernetes v1.35 Release Notes](https://github.com/kubernetes/kubernetes/blob/master/CHANGELOG/CHANGELOG-1.35.md)
