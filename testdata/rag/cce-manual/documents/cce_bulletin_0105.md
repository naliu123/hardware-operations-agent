# Kubernetes 1.33版本说明

来源：https://support.huaweicloud.com/usermanual-cce/cce_bulletin_0105.html
目录：用户指南 > 集群 > 集群版本发布说明 > Kubernetes版本发布记录 > Kubernetes 1.33版本说明
更新时间：2026-05-29 GMT+08:00

云容器引擎（CCE）严格遵循社区一致性认证，现已支持Kubernetes 1.33集群版本特性。本文介绍Kubernetes 1.33版本的变更说明。

#### 索引

- [新增特性及特性增强](https://support.huaweicloud.com/usermanual-cce/cce_bulletin_0105.html)
- [API变更与弃用](https://support.huaweicloud.com/usermanual-cce/cce_bulletin_0105.html)
- [CCE对Kubernetes 1.33版本的增强](https://support.huaweicloud.com/usermanual-cce/cce_bulletin_0105.html)
- [参考链接](https://support.huaweicloud.com/usermanual-cce/cce_bulletin_0105.html)

#### 新增特性及特性增强

- 允许微调CPU Manager的资源分配策略（GA）

  在Kubernetes 1.33中，CPUManagerPolicyOptions进阶至GA。该特性支持允许微调CPU Manager的资源分配策略。详细使用方式请参考[CPU管理策略](https://kubernetes.io/zh-cn/docs/tasks/administer-cluster/cpu-management-policies/#cpu-policy-static--options)。
- ServiceCIDR动态增加ClusterIP的可分配范围（GA）

  在Kubernetes 1.33中，MultiCIDRServiceAllocator进阶至GA。引入ServiceCIDR和IPAddress资源来记录Service的ClusterIP分配情况，支持通过ServiceCIDR来动态增加ClusterIP的可分配范围。

  ![](https://support.huaweicloud.com/intl/zh-cn/usermanual-cce/public_sys-resources/note_3.0-zh-cn.png)

  - 新增的服务网段不能和所属集群的子网网段和容器网段重叠。
  - CCE Turbo集群由于Service网络组网的特殊性，暂不支持此特性。
- JobBackoffLimitPerIndex（GA）

  在Kubernetes 1.33中，JobBackoffLimitPerIndex进阶至GA。允许为索引作业中的每个索引指定Pod的最大重试次数。详细使用方式请参考[Kubernetes v1.33：Job逐索引的回退限制进阶至GA](https://kubernetes.io/zh-cn/blog/2025/05/13/kubernetes-v1-33-jobs-backoff-limit-per-index-goes-ga/)。
- JobSuccessPolicy（GA）

  在Kubernetes 1.33中，JobSuccessPolicy进阶至GA。允许自定义Job的成功策略，例如通过指定某些索引是否成功和成功的索引数量来判断Job是否完成。详细使用方式请参考 [Job的SuccessPolicy](https://kubernetes.io/zh-cn/blog/2025/05/15/kubernetes-1-33-jobs-success-policy-goes-ga/)。
- MatchLabelKeys（GA）

  在Kubernetes 1.33中，MatchLabelKeysInPodAffinity进阶至GA。在Pod亲和性规则中增加了matchLabelKeys和mismatchLabelKeys。
- Pod拓扑分布约束（GA）

  在Kubernetes 1.33中，NodeInclusionPolicyInPodTopologySpread进阶至GA。允许在[Pod拓扑分布约束](https://kubernetes.io/zh-cn/docs/concepts/scheduling-eviction/topology-spread-constraints/)使用nodeAffinityPolicy和nodeTaintsPolicy动态筛选可调度节点。

  - nodeAffinityPolicy：默认为Honor，仅将匹配Pod的nodeSelector或nodeAffinity的节点纳入拓扑分布计算。
  - nodeTaintsPolicy：默认为Ignore，忽略nodeAffinity和nodeSelector规则，将所有节点纳入拓扑分布计算。

  详细使用方式请参考[Pod拓扑分布约束](https://kubernetes.io/zh-cn/docs/concepts/scheduling-eviction/topology-spread-constraints/)。
- HonorPVReclaimPolicy（GA）

  在Kubernetes 1.33中，HonorPVReclaimPolicy进阶至GA。用于确保当PV的reclaimPolicy设置为Delete时，无论PV或PVC的删除顺序如何，都会严格按照策略删除底层存储资源，避免存储资源泄露。
- 镜像作为volume进行挂载（Beta）

  在Kubernetes 1.33中，ImageVolume进阶至Beta。允许在Pod中使用image卷源，将容器镜像作为只读卷挂载到Pod中。详细使用方式请参考[镜像卷](https://kubernetes.io/zh-cn/blog/2025/04/29/kubernetes-v1-33-image-volume-beta/)。
- UserNamespacesSupport（Beta）

  在Kubernetes 1.33中，UserNamespacesSupport进阶至Beta。允许Pod使用Linux用户命名空间。详细使用方式请参考[Pod配置user命名空间](https://kubernetes.io/zh-cn/docs/tasks/configure-pod-container/user-namespaces/)。
- 通过流式处理大规模资源列表请求（Beta）

  在Kubernetes 1.33中，StreamingCollectionEncodingToProtobuf进阶至Beta。kube-apiserver禁用WatchList机制，转而采用流式编码机制。对于大量资源的List请求场景，可有效降低内存占用并提升系统稳定性。详细使用方式请参考[流式List响应](https://kubernetes.io/zh-cn/blog/2025/05/09/kubernetes-v1-33-streaming-list-responses/)。
- 调度器性能优化（Beta）

  在Kubernetes 1.33中，SchedulerPopFromBackoffQ进阶至Beta。该特性优化调度队列的处理逻辑，允许activeQ为空时，直接从backoffQ中弹出Pod，显著减少Pod的调度延迟。详细使用方式请参考[调度器性能优化](https://github.com/kubernetes/kubernetes/pull/130772)。
- ProcMountType（Beta）

  在Kubernetes 1.33中，ProcMountType进阶至Beta。允许通过Pod的securityContext.procMount字段自定义容器中/proc文件系统的挂载类型，以精细化控制/proc文件系统的访问，提升Pod安全性和隔离性。此功能适用于需要在用户命名空间中运行非特权容器的场景，通过放宽对/proc的限制，可增强兼容性与灵活性。
- PodLifecycleSleepAction（Beta）

  在Kubernetes 1.33中，PodLifecycleSleepAction进阶至Beta。默认开启，允许用户创建持续时间为零秒的休眠生命周期作的容器 。详细使用方式请参考[Introducing Sleep Action for PreStop Hook](https://github.com/kubernetes/enhancements/blob/master/keps/sig-node/3960-pod-lifecycle-sleep-action/README.md)。
- 限制特定Volume Attributes Class关联的PVC数量。

  在Kubernetes 1.33中，允许使用ResourceQuota限制特定Volume Attributes Class关联的PVC数量。

#### API变更与弃用

- 在Kubernetes 1.33中，EndpointSlice的annotation service.kubernetes.io/topology-mode废弃，保持向后兼容，由spec.trafficDistribution替换。
- 在Kubernetes 1.33中，apidiscovery.k8s.io/v2beta1 API组废弃。此API用于客户端查询集群中所有已注册的API资源信息。建议使用v2稳定版本。
- 在Kubernetes 1.33中，WatchFromStorageWithoutResourceVersion功能已弃用。该功能允许在没有resourceVersion的情况下基于etcd提供watch服务。
- 在Kubernetes 1.33中，v1版本的Endpoints API正式废弃，但仍然支持，推荐使用EndpointSlice API代替。EndpointSlice API自1.21起已进入稳定状态，并引入了双栈网络支持等特性。详细使用方式请参考[Kubernetes v1.33: Continuing the transition from Endpoints to EndpointSlices](https://kubernetes.io/blog/2025/04/24/endpoints-deprecation/)。
- 在Kubernetes 1.33中，Pod的status.resize字段现已弃用，将不再设置,调整大小操作的状态现在通过两个Pod状况暴露：
  - PodResizePending：表示 kubelet 无法立即批准调整大小 （例如，如果暂时不能，则 reason: Deferred；如果在节点上不可能，则 reason: Infeasible）。
  - PodResizeInProgress：表示调整大小已被接受并正在应用。 在此阶段遇到的错误现在会在此状况的消息中报告为 reason: Error。

  详细使用方式请参考[原地调整Pod资源特性升级为Beta](https://kubernetes.io/zh-cn/blog/2025/05/16/kubernetes-v1-33-in-place-pod-resize-beta/)。

#### CCE对Kubernetes 1.33版本的增强

在版本维护周期中，CCE会对Kubernetes 1.33版本进行定期的更新，并提供功能增强。

关于CCE集群版本的更新说明，请参见[补丁版本发布说明](https://support.huaweicloud.com/usermanual-cce/cce_10_0405.html)。

#### 参考链接

关于Kubernetes 1.33与其他版本的性能对比和功能演进的更多信息，请参考：[Kubernetes v1.33 Release Notes](https://github.com/kubernetes/kubernetes/blob/master/CHANGELOG/CHANGELOG-1.33.md)
