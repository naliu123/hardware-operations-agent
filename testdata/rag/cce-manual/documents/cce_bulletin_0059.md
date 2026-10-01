# （停止维护）Kubernetes 1.27版本说明

来源：https://support.huaweicloud.com/usermanual-cce/cce_bulletin_0059.html
目录：用户指南 > 集群 > 集群版本发布说明 > Kubernetes版本发布记录 > （停止维护）Kubernetes 1.27版本说明
更新时间：2026-05-26 GMT+08:00

云容器引擎（CCE）严格遵循社区一致性认证，现已支持Kubernetes 1.27集群版本特性。本文介绍Kubernetes 1.27版本相对于1.25版本所做的变更说明。

#### 索引

- [主要特性](https://support.huaweicloud.com/usermanual-cce/cce_bulletin_0059.html)
- [弃用和移除](https://support.huaweicloud.com/usermanual-cce/cce_bulletin_0059.html)
- [CCE对Kubernetes 1.27版本的增强](https://support.huaweicloud.com/usermanual-cce/cce_bulletin_0059.html)
- [参考链接](https://support.huaweicloud.com/usermanual-cce/cce_bulletin_0059.html)

#### 主要特性

**Kubernetes 1.27版本**

- SeccompDefault特性已进入稳定阶段

  如需使用SeccompDefault特性，您需要为每个节点的kubelet启用--seccomp-default[命令行标志](https://kubernetes.io/zh-cn/docs/reference/command-line-tools-reference/kubelet/)。如果启用该特性，kubelet将为所有工作负载默认使用RuntimeDefault seccomp配置文件，该配置文件由容器运行时定义，而不是使用Unconfined（禁用seccomp）模式。
- Job可变调度指令

  该特性在Kubernetes 1.22版本中引入，当前已进入稳定阶段。在大多数情况下，并行作业Pod希望在一定的约束下运行，例如希望所有Pod在同一可用区。该特性允许在Job开始前修改调度指令。您可以使用suspend字段挂起Job，在Job挂起阶段，Pod模板中的调度部分（例如节点选择器、节点亲和性、反亲和性、容忍度）允许修改。详情请参见[可变调度指令](https://kubernetes.io/zh-cn/docs/concepts/workloads/controllers/job/#mutable-scheduling-directives)。
- Downward API HugePages已进入稳定阶段

  在Kubernetes 1.20版本中，[Downward API](https://kubernetes.io/zh-cn/docs/concepts/workloads/pods/downward-api/)引入了*requests.hugepages-<pagesize>*和*limits.hugepages-<pagesize>*，HugePage可以和其他资源一样设置资源配额。
- Pod调度就绪态进入Beta阶段

  Pod创建后，Kubernetes调度程序会负责选择合适的节点运行pending状态的Pod。在实际使用时，一些Pod可能会由于资源不足长时间处于pending状态。这些Pod可能会影响集群中的其他组件运行（如Cluster Autoscaler）。通过指定/删除Pod的.spec.schedulingGates，您可以控制Pod何时准备好进行调度。详情请参见[Pod调度就绪态](https://kubernetes.io/zh-cn/docs/concepts/scheduling-eviction/pod-scheduling-readiness/)。
- 通过Kubernetes API访问节点日志

  此功能当前处于Alpha阶段。集群管理员可以直接查询节点上的服务日志，可以帮助调试节点上运行的服务问题。如需使用此功能，请确保在该节点上启用了NodeLogQuery[特性门控](https://kubernetes.io/zh-cn/docs/reference/command-line-tools-reference/feature-gates/)，并且kubelet配置选项enableSystemLogHandler和enableSystemLogQuery都设置为true。
- ReadWriteOncePod访问模式进入Beta阶段

  在Kubernetes 1.22版本中，PV和PVC提供了一种新的访问模式ReadWriteOncePod，该功能当前进入Beta阶段。卷可以被单个Pod以读写方式挂载。如果您想确保整个集群中只有一个Pod可以读取或写入该PVC，请使用ReadWriteOncePod访问模式，详情请参见[访问模式](https://kubernetes.io/zh-cn/docs/concepts/storage/persistent-volumes/#access-modes)。
- Pod拓扑分布约束中matchLabelKeys字段进入Beta阶段

  matchLabelKeys是一个Pod标签键的列表，用于选择需要计算分布方式的Pod集合。使用matchLabelKeys字段，您无需在变更Pod修订版本时更新pod.spec。控制器或Operator只需要将不同修订版的标签键设为不同的值。调度器将根据matchLabelKeys自动确定取值。详情请参见[Pod拓扑分布约束](https://kubernetes.io/zh-cn/docs/concepts/scheduling-eviction/topology-spread-constraints/#topologyspreadconstraints-field)。
- 快速标记SELinux卷标签功能进入Beta阶段

  默认情况下，容器运行时递归地将SELinux标签赋予所有Pod卷上的所有文件。 为了加快该过程，Kubernetes使用挂载可选项*-o context=<label>*可以立即改变卷的SELinux标签。详情请参见[快速标记SELinux卷标签](https://kubernetes.io/zh-cn/docs/tasks/configure-pod-container/security-context/#高效重打-selinux-卷标签)。
- VolumeManager重构进入Beta阶段

  重构的VolumeManager后，如果启用NewVolumeManagerReconstruction[特性门控](https://kubernetes.io/zh-cn/docs/reference/command-line-tools-reference/feature-gates/)，将会在kubelet启动期间使用更有效的方式来获取已挂载卷。
- 服务器端字段校验和OpenAPI V3已进入稳定阶段

  Kubernetes 1.23中添加了对OpenAPI v3的支持，1.24版本中已进入Beta阶段，1.27已进入稳定阶段。
- 控制StatefulSet启动序号

  Kubernetes 1.26为StatefulSet引入了一个新的Alpha级别特性，可以控制Pod副本的序号。 从Kubernetes 1.27开始，此特性进入Beta阶段，序数可以从任意非负数开始。详情请参见[Kubernetes 1.27: StatefulSet启动序号简化了迁移](https://kubernetes.io/zh-cn/blog/2023/04/28/statefulset-start-ordinal/)。
- HorizontalPodAutoscaler ContainerResource类型指标进入Beta阶段

  Kubernetes 1.20在HorizontalPodAutoscaler (HPA) 中引入了[ContainerResource类型指标](https://kubernetes.io/zh-cn/docs/concepts/workloads/autoscaling/horizontal-pod-autoscale/#container-resource-metrics)。在Kubernetes 1.27中，此特性进阶至Beta，相应的特性门控 (HPAContainerMetrics) 默认被启用。
- StatefulSet PVC自动删除进入Beta阶段

  Kubernetes v1.27提供一种新的策略机制，用于控制StatefulSets的PersistentVolumeClaims（PVCs）的生命周期。这种新的PVC保留策略允许用户指定当删除StatefulSet或者缩减StatefulSet中的副本时，是自动删除还是保留从StatefulSet规约模板生成的PVC。详情请参见[PersistentVolumeClaim保留](https://kubernetes.io/zh-cn/docs/concepts/workloads/controllers/statefulset/#persistentvolumeclaim-retention)。
- 磁盘卷组快照

  磁盘卷组快照在Kubernetes 1.27中作为Alpha特性被引入。此特性允许用户对多个卷进行快照，以保证在发生故障时数据的一致性。 它使用标签选择器来将多个PersistentVolumeClaims分组以进行快照。 这个新特性仅支持CSI卷驱动器。详情请参见[Kubernetes 1.27：介绍用于磁盘卷组快照的新API](https://kubernetes.io/zh-cn/blog/2023/05/08/kubernetes-1-27-volume-group-snapshot-alpha/)。
- kubectl apply裁剪更安全、更高效

  在Kubernetes 1.5版本中，kubectl apply引入了--prune标志来删除不再需要的资源，允许kubectl apply自动清理从当前配置中删除的资源。然而，现有的--prune实现存在设计缺陷，会降低性能并导致意外行为。Kubernetes 1.27中，kubectl apply提供基于ApplySet的剪裁方式，当前处于Alpha阶段，详情请参见[使用配置文件对Kubernetes对象进行声明式管理](https://kubernetes.io/zh-cn/docs/tasks/manage-kubernetes-objects/declarative-config/#替代方式-kubectl-apply-f-目录-prune)。
- 为NodePort Service分配端口时避免冲突

  在Kubernetes 1.27中，您可以启用新的[特性门控](https://kubernetes.io/zh-cn/docs/reference/command-line-tools-reference/feature-gates/)ServiceNodePortStaticSubrange，为NodePort Service使用不同的端口分配策略，减少冲突的风险。当前该特性处于Alpha阶段。
- 原地调整Pod资源

  在Kubernetes 1.27中，允许用户调整分配给Pod的CPU和内存资源大小，而无需重新启动容器。当前该特性处于Alpha阶段，详情请参见[纵向弹性伸缩](https://kubernetes.io/zh-cn/blog/2023/05/12/in-place-pod-resize-alpha/)。
- 加快Pod启动

  在Kubernetes 1.27中进行了一系列的参数调整，以提高Pod的启动速度，例如并行镜像拉取、提高Kubelet默认API每秒查询限值等。详情请参见[Kubernetes 1.27：关于加快Pod启动的进展](https://kubernetes.io/zh-cn/blog/2023/05/15/speed-up-pod-startup/)。
- KMS V2进入Beta阶段

  Kubernetes中的密钥管理KMS v2 API进入Beta阶段，对KMS加密提供程序的性能进行了重大改进。详情请参见[使用KMS驱动进行数据加密](https://kubernetes.io/zh-cn/docs/tasks/administer-cluster/kms-provider/)。

**Kubernetes 1.26版本**

- 移除CRI v1alpha2

  Kubernetes 1.26版本不再支持CRI v1alpha2，请使用v1（要求containerd版本>=1.5.0） 。这意味着Kubernetes 1.26将不支持containerd 1.5.x及更早的版本；需要升级到containerd 1.6.x或更高版本后，才能将该节点的kubelet升级到1.26。

  ![](https://support.huaweicloud.com/intl/zh-cn/usermanual-cce/public_sys-resources/note_3.0-zh-cn.png)

  CCE目前使用的containerd版本为1.6.14，已满足要求。如存量的节点不满足containerd版本要求，请将节点重置为最新版本。
- 动态资源分配 Alpha API

  在Kubernetes 1.26版本，新增[动态资源分配](https://kubernetes.io/zh-cn/docs/concepts/scheduling-eviction/dynamic-resource-allocation/)功能，用于Pod之间和Pod内部容器之间请求和共享资源，支持用户提供参数初始化资源。该功能尚处于alpha阶段，需要启用DynamicResourceAllocation特性门禁和resource.k8s.io/v1alpha1 API组，需要为要管理的特定资源安装驱动程序。更多信息，请参见[Kubernetes 1.26: 动态资源分配 Alpha API](https://kubernetes.io/zh-cn/blog/2022/12/15/dynamic-resource-allocation/)。
- 节点非体面关闭进入Beta阶段

  在Kubernetes 1.26 中，节点非体面关闭特性是Beta版，默认被启用。当kubelet的节点关闭管理器可以检测到即将到来的节点关闭操作时，节点关闭才被认为是体面的。详情请参见[处理节点非体面关闭](https://kubernetes.io/zh-cn/docs/concepts/architecture/nodes/#non-graceful-node-shutdown)。
- 支持在挂载时将Pod fsGroup传递给CSI驱动程序

  将fsGroup委托给CSI驱动程序管理首先在Kubernetes 1.22中作为Alpha特性引入， 并在Kubernetes 1.25中进阶至Beta状态。该特性在Kubernetes 1.26已进入正式发布阶段，详情请参见[将卷权限和所有权更改委派给CSI驱动程序](https://kubernetes.io/zh-cn/docs/tasks/configure-pod-container/security-context/#将卷权限和所有权更改委派给-csi-驱动程序)。
- Pod调度就绪态

  Kubernetes 1.26引入了一个新的Pod特性schedulingGates，可以让调度器感知到何时可以进行Pod调度。详情请参见[Pod调度就绪态](https://kubernetes.io/zh-cn/docs/concepts/scheduling-eviction/pod-scheduling-readiness/)。
- CPU Manager正式发布

  CPU管理器是kubelet的一部分，从Kubernetes 1.10[进阶至 Beta](https://kubernetes.io/blog/2018/07/24/feature-highlight-cpu-manager/)，能够将独占CPU分配给容器。该特性在Kubernetes 1.26已进入稳定阶段，详情请参见[控制节点上的CPU管理策略](https://kubernetes.io/zh-cn/docs/tasks/administer-cluster/cpu-management-policies/#static-policy-options)。
- Kubernetes中流量工程的进步

  [优化内部节点本地流量](https://kubernetes.io/zh-cn/blog/2022/12/30/advancements-in-kubernetes-traffic-engineering/#optimizing-internal-node-local-traffic)和[支持EndpointSlice终止状况](https://kubernetes.io/zh-cn/blog/2022/12/30/advancements-in-kubernetes-traffic-engineering/#endpointslice-conditions)升级为正式发布版本， [ProxyTerminatingEndpoints](https://kubernetes.io/zh-cn/blog/2022/12/30/advancements-in-kubernetes-traffic-engineering/#traffic-loss-from-load-balancers-during-rolling-updates)功能升级为Beta版本。
- 支持跨命名空间存储数据源

  Kubernetes 1.26允许在源数据属于不同的命名空间时为PersistentVolumeClaim指定数据源。当前该特性处于Alpha阶段，详情请参见[跨命名空间数据源](https://kubernetes.io/zh-cn/docs/concepts/storage/persistent-volumes/#cross-namespace-data-sources)。
- 可追溯的默认StorageClass进入Beta阶段

  Kubernetes 1.25引入了一个Alpha特性来更改默认StorageClass被分配到 PersistentVolumeClaim (PVC) 的方式。 启用此特性后，您不再需要先创建默认StorageClass，再创建PVC来分配类。 此外，任何未分配StorageClass的PVC都可以在后续被更新。此特性在Kubernetes 1.26 中已进入Beta阶段，详情请参见[可追溯的默认StorageClass赋值](https://kubernetes.io/zh-cn/docs/concepts/storage/persistent-volumes/#retroactive-default-storageclass-assignment)。
- PodDisruptionBudget支持指定不健康Pod的驱逐策略

  Kubernetes 1.26允许针对[PodDisruptionBudget](https://kubernetes.io/zh-cn/docs/concepts/workloads/pods/disruptions/#pod-disruption-budgets) (PDB) 指定不健康Pod驱逐策略，这有助于在节点执行管理操作期间保持可用性。当前该特性处于Beta阶段，详情请参见[不健康的Pod驱逐策略](https://kubernetes.io/zh-cn/docs/tasks/run-application/configure-pdb/#unhealthy-pod-eviction-policy)。
- 支持设置水平伸缩Pod控制器的数量

  kube-controller-manager支持flag --concurrent-horizontal-pod-autoscaler-syncs设置水平伸缩Pod控制器的worker数量。详情请参见[集群配置管理](https://support.huaweicloud.com/usermanual-cce/cce_10_0213.html)。

#### 弃用和移除

**Kubernetes 1.27版本**

- 在Kubernetes 1.27版本，针对卷扩展GA特性的以下特性门禁将被移除，且不得再在--feature-gates标志中引用。（**ExpandCSIVolumes****，****ExpandInUsePersistentVolumes****，****ExpandPersistentVolumes**）
- 在Kubernetes 1.27版本，移除--master-service-namespace 命令行参数。该参数支持指定在何处创建名为kubernetes的Service来表示API服务器。自v1.26版本已被弃用，1.27版本正式移除。
- 在Kubernetes 1.27版本，移除ControllerManagerLeaderMigration特性门禁。[Leader Migration](https://github.com/kubernetes/enhancements/issues/2436)提供了一种机制，让HA集群在升级多副本的控制平面时通过在kube-controller-manager和cloud-controller-manager这两个组件之间共享的资源锁，安全地迁移“特定于云平台”的控制器。特性自v1.24正式发布，被无条件启用， 在v1.27版本中此特性门禁选项将被移除。
- 在Kubernetes 1.27版本，移除--enable-taint-manager命令行参数。该参数支持的特性基于污点的驱逐已被默认启用，且在标志被移除时也将继续被隐式启用。
- 在Kubernetes 1.27版本，移除--pod-eviction-timeout 命令行参数。弃用的命令行参数--pod-eviction-timeout将被从kube-controller-manager中移除。
- 在Kubernetes 1.27版本，移除CSI Migration特性门禁。[CSI migration](https://github.com/kubernetes/enhancements/issues/625)程序允许从树内卷插件移动到树外CSI驱动程序。CSI迁移自Kubernetes v1.16起正式发布，关联的CSIMigration特性门禁将在v1.27中被移除。
- 在Kubernetes 1.27版本，移除CSIInlineVolume特性门禁。[CSI Ephemeral Volume](https://github.com/kubernetes/kubernetes/pull/111258)特性允许在Pod规约中直接指定CSI卷作为临时使用场景。这些CSI卷可用于使用挂载的卷直接在Pod内注入任意状态，例如配置、Secret、身份、变量或类似信息。 此特性在v1.25中进阶至正式发布。因此，此特性门禁CSIInlineVolume将在v1.27版本中移除。
- 在Kubernetes 1.27版本，移除EphemeralContainers特性门禁。对于Kubernetes v1.27，临时容器的API支持被无条件启用；EphemeralContainers特性门禁将被移除。
- 在Kubernetes 1.27版本，移除LocalStorageCapacityIsolation特性门禁。[Local Ephemeral Storage Capacity Isolation](https://github.com/kubernetes/kubernetes/pull/111513)特性在 v1.25 中进阶至正式发布。此特性支持emptyDir卷这类Pod之间本地临时存储的容量隔离，因此可以硬性限制Pod对共享资源的消耗。如果本地临时存储的消耗超过了配置的限制，kubelet将驱逐 Pod。特性门禁LocalStorageCapacityIsolation将在v1.27版本中被移除。
- 在Kubernetes 1.27版本，移除NetworkPolicyEndPort特性门禁。Kubernetes v1.25版本将NetworkPolicy中的endPort进阶至正式发布。支持endPort字段的NetworkPolicy提供程序可用于指定一系列端口以应用NetworkPolicy。
- 在Kubernetes 1.27版本，移除StatefulSetMinReadySeconds特性门禁。对于作为StatefulSet一部分的Pod，只有当Pod至少在[minReadySeconds](https://kubernetes.io/zh-cn/docs/concepts/workloads/controllers/statefulset/#minimum-ready-seconds)中指定的持续期内可用（并通过检查）时，Kubernetes才会将此Pod标记为只读。 该特性在Kubernetes v1.25中正式发布，StatefulSetMinReadySeconds特性门禁将锁定为true，并在v1.27版本中被移除。
- 在Kubernetes 1.27版本，移除IdentifyPodOS特性门禁。启用该特性门禁，您可以为Pod指定操作系统，此项特性支持自v1.25版本进入稳定。IdentifyPodOS特性门禁将在Kubernetes v1.27中被移除。
- 在Kubernetes 1.27版本，移除DaemonSetUpdateSurge特性门禁。Kubernetes v1.25版本还稳定了对DaemonSet Pod的浪涌支持，其实现是为了最大限度地减少部署期间DaemonSet的停机时间。DaemonSetUpdateSurge特性门禁将在Kubernetes v1.27中被移除。
- 在Kubernetes 1.27版本，移除--container-runtime 命令行参数。kubelet 接受一个已弃用的命令行参数--container-runtime， 并且在移除dockershim代码后，唯一有效的值将是remote。 Kubernetes v1.27将移除该参数，该参数自v1.24版本以来已被弃用。

**Kubernetes 1.26版本**

- 移除v2beta2版本的HorizontalPodAutoscaler API

  HorizontalPodAutoscaler的autoscaling/v2beta2 API版本将不再在1.26版本中提供，详情请参见[各发行版本中移除的API](https://kubernetes.io/zh-cn/docs/reference/using-api/deprecation-guide/#horizontalpodautoscaler-v126)。 用户应迁移至autoscaling/v2版本的API。
- 移除v1beta1版本的流量控制API组

  在Kubernetes 1.26版本后，开始不再提供flowcontrol.apiserver.k8s.io/v1beta1 API版本的FlowSchema和PriorityLevelConfiguration，详情请参见[各发行版本中移除的API](https://kubernetes.io/zh-cn/docs/reference/using-api/deprecation-guide/#horizontalpodautoscaler-v126)。但此API从Kubernetes 1.23版本开始，可以使用flowcontrol.apiserver.k8s.io/v1beta2；从Kubernetes 1.26版本开始，可以使用flowcontrol.apiserver.k8s.io/v1beta3。
- 存储驱动的弃用和移除，移除云服务厂商的in-tree卷驱动。
- 移除kube-proxy userspace模式

  在Kubernetes 1.26版本，Userspace代理模式已被移除，已弃用的Userspace代理模式不再受Linux或Windows支持。Linux用户应使用Iptables或IPVS，Windows用户应使用Kernelspace，现在使用--mode userspace会失败。

  - Windows winkernel kube-proxy不再支持Windows HNS v1 APIs。
- 弃用--prune-whitelist标志

  在Kubernetes 1.26版本，为了支持[Inclusive Naming Initiative](https://www.cncf.io/announcements/2021/10/13/inclusive-naming-initiative-announces-new-community-resources-for-a-more-inclusive-future/)，--prune-whitelist标志将被[弃用](https://github.com/kubernetes/kubernetes/pull/113116)，并替换为--prune-allowlist，该标志在未来将彻底移除。
- 移除动态Kubelet配置

  DynamicKubeletConfig特性门控移除，通过API动态更新节点上的Kubelet配置。在Kubernetes 1.24版本中从Kubelet移除相关代码，在Kubernetes 1.26版本从APIServer移除相关代码，移除该逻辑有助于简化代码提升可靠性，推荐方式是修改Kubelet配置文件然后重启Kubelet。更多信息，请参见[在Kubernetes 1.26版本从APIServer移除相关代码](https://github.com/kubernetes/kubernetes/pull/112643)。
- 弃用kube-apiserver命令行参数

  在Kubernetes 1.26版本，正式标记[弃用 --master-service-namespace](https://github.com/kubernetes/kubernetes/pull/38186) 命令行参数，它对APIServer没有任何效果。
- 弃用kubectl run命令行参数

  在Kubernetes 1.26版本，kubectl run未使用的几个子命令将被标记为[弃用](https://github.com/kubernetes/kubernetes/pull/112261)，并在未来某个版本移除，包括--cascade、--filename、--force、--grace-period、--kustomize、--recursive、--timeout、--wait等这些子命令。
- 移除与日志相关的原有命令行参数

  在Kubernetes 1.26版本，将[移除](https://github.com/kubernetes/kubernetes/pull/112120)一些与日志相关的命令行参数，这些参数在之前的版本已被[弃用](https://github.com/kubernetes/enhancements/tree/3cb66bd0a1ef973ebcc974f935f0ac5cba9db4b2/keps/sig-instrumentation/2845-deprecate-klog-specific-flags-in-k8s-components#removed-klog-flags)。

#### CCE对Kubernetes 1.27版本的增强

在版本维护周期中，CCE会对Kubernetes 1.27版本进行定期的更新，并提供功能增强。

关于CCE集群版本的更新说明，请参见[CCE集群版本发布说明](https://support.huaweicloud.com/usermanual-cce/cce_10_0405.html)。

#### 参考链接

关于Kubernetes 1.27与其他版本的性能对比和功能演进的更多信息，请参考：

- [Kubernetes v1.27 Release Notes](https://github.com/kubernetes/kubernetes/blob/master/CHANGELOG/CHANGELOG-1.27.md)
- [Kubernetes v1.26 Release Notes](https://github.com/kubernetes/kubernetes/blob/master/CHANGELOG/CHANGELOG-1.26.md)
