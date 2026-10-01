# Kubernetes 1.34版本说明

来源：https://support.huaweicloud.com/usermanual-cce/cce_bulletin_0108.html
目录：用户指南 > 集群 > 集群版本发布说明 > Kubernetes版本发布记录 > Kubernetes 1.34版本说明
更新时间：2026-09-20 GMT+08:00

云容器引擎（CCE）严格遵循社区一致性认证，现已支持Kubernetes 1.34集群版本特性。本文介绍Kubernetes 1.34版本的变更说明。

#### 索引

- [新增特性及特性增强](https://support.huaweicloud.com/usermanual-cce/cce_bulletin_0108.html)
- [API变更与弃用](https://support.huaweicloud.com/usermanual-cce/cce_bulletin_0108.html)
- [CCE对Kubernetes 1.34版本的增强](https://support.huaweicloud.com/usermanual-cce/cce_bulletin_0108.html)
- [参考链接](https://support.huaweicloud.com/usermanual-cce/cce_bulletin_0108.html)

#### 新增特性及特性增强

- RecoverVolumeExpansionFailure （GA）

  在Kubernetes 1.34中，RecoverVolumeExpansionFailure进阶至GA。您可以减少PVC请求的大小，只要之前请求的扩展还未完成，就可以修改所请求的大小。Kubernetes会自动进行修正。详细使用方式请参考[卷扩容失败恢复](https://kubernetes.io/zh-cn/blog/2025/09/19/kubernetes-v1-34-recover-expansion-failure/)。
- JobPodReplacementPolicy（GA）

  在Kubernetes 1.34中，JobPodReplacementPolicy进阶至GA。此功能在Jobs中引入了.spec.podReplacementPolicy。您可以选择仅在Pod完全终止（.status.phase: Failed）时创建替代Pod。详细使用方式请参考[允许在完全终止时替换作业中的Pod](https://kubernetes.io/blog/2025/09/05/kubernetes-v1-34-pod-replacement-policy-for-jobs-goes-ga/)。
- Resilient watch cache initialization（GA）

  在Kubernetes 1.34中，Watch cache的初始化过程变得更加抗故障，提高了控制平面的鲁棒性，并确保控制器和客户端能够可靠地建立观察。详细介绍请参考[弹性监视缓存初始化](https://github.com/kubernetes/enhancements/tree/master/keps/sig-api-machinery/4568-resilient-watchcache-initialization)。
- Relaxing DNS search path validation（GA）

  在Kubernetes 1.34中，Relaxing DNS search path validation进阶至GA。过去Kubernetes在dnsConfig.searches（DNS搜索域列表）中根据RFC-1123验证搜索字符串，而RFC-1123对主机名设定了限制。但是这种验证对于dnsConfig.searches的使用而言，过于严格，现在允许以下设置：

  - 允许在连字符(-)的位置使用下划线(\_)
  - 允许单个点号(.)作为搜索域

  详细使用方式请参考[宽松的DNS搜索字符串验证](https://github.com/kubernetes/enhancements/tree/master/keps/sig-network/4427-relaxed-dns-search-validation)。
- Sleep action for Container lifecycle hooks（GA）

  在Kubernetes 1.34中，Sleep action for Container lifecycle hooks进阶至GA。该特性支持在容器生命周期回调中通过Sleep字段直接使用sleep命令，将容器暂停一段指定的时间，并支持填写sleep为零值。详细使用方式请参考[引入PreStop Hook睡眠操作](https://github.com/kubernetes/enhancements/tree/master/keps/sig-node/3960-pod-lifecycle-sleep-action)和[允许PreStop Hook的睡眠动作为零值](https://github.com/kubernetes/enhancements/blob/master/keps/sig-node/4818-allow-zero-value-for-sleep-action-of-prestop-hook/README.md)。
- Dynamic Resource Allocation（GA）

  动态资源分配Dynamic Resource Allocation（DRA）的核心功能进阶至GA，基于DRA，允许您在 Pod 之间请求和共享资源。详细使用方式请参考[动态资源分配](https://kubernetes.io/docs/concepts/scheduling-eviction/dynamic-resource-allocation/#about-dra)。
- kuberc file for kubectl user preferences（Beta）

  在Kubernetes 1.34中，kuberc file for kubectl user preferences进阶至Beta。kuberc 配置文件允许您为 kubectl 定义偏好设置，例如默认选项和命令别名。详细使用方式请参考[用户偏好设置（kuberc）可在 kubectl 1.34 中进行测试](https://kubernetes.io/blog/2025/08/28/kubernetes-v1-34-kubectl-kuberc-beta/)。
- Snapshottable API server cache（Beta）

  在Kubernetes 1.34中，Snapshottable API server cache进阶至Beta。在默认启用的 ListFromCacheSnapshot 功能下，kube-apiserver 会尝试从快照中提供响应。详细介绍请参考[kube-apiserver List请求优化](https://kubernetes.io/blog/2025/09/09/kubernetes-v1-34-snapshottable-api-server-cache/)。
- KubeletPSI（Beta）

  在Kubernetes 1.34中，KubeletPSI进阶至Beta。PSI是一种监控指标，用于测量系统在CPU、内存和I/O资源方面的压力。它能够提供服务的延迟和稳定性等信息。

  kubelet支持查询节点的压力指标。详细介绍请参考[理解PSI指标](https://kubernetes.io/docs/reference/instrumentation/understand-psi-metrics/)。
- KubeletPodResourcesGet和KubeletPodResourcesDynamicResources（Beta）

  在Kubernetes 1.34中，KubeletPodResourcesGet和KubeletPodResourcesDynamicResources特性进阶至Beta，新增了部分gRPC接口，支持通过kubelet获取一些资源的分配信息。详细介绍请参考[设备插件](https://kubernetes.io/zh-cn/docs/concepts/extend-kubernetes/compute-storage-net/device-plugins/)。
- PodLevelResources（Beta）

  在Kubernetes 1.34中，PodLevelResources进阶至Beta，该特性支持Pod级别的资源管理，使Kubernetes能够控制Pod的总资源消耗，从而减轻用户为每个容器精心配置资源的负担。详细介绍请参考[Pod级别资源规范](https://kubernetes.io/blog/2025/09/22/kubernetes-v1-34-pod-level-resources/)。

  ![](https://support.huaweicloud.com/intl/zh-cn/usermanual-cce/public_sys-resources/note_3.0-zh-cn.png)

  - 当您设置了Pod级别的资源申请或限制时，控制台界面展示的CPU和内存数据（包括申请值、限制值及使用率）依然会按照容器级别的数值来统计。
  - 当Pod级别设置了某资源的limit但未设置request时，API Server会自动填写request。该默认填充发生在Sidecar注入之前，Sidecar注入新增容器后，API Server不会重新执行默认填充，可能会导致Pod级别资源值设置非法从而校验失败无法创建，您可以通过在Pod级别显式设置所有资源的request和limit的方式规避。

#### API变更与弃用

- 在Kubernetes 1.34中，创建无头服务时，如果设置了loadBalancerIP、externalIPs或sessionAffinity，将增加警告信息。
- 在Kubernetes 1.34中，当容器未指定巨页（HugePage）资源时，将Pod级别的巨页cgroup传播到容器。并且添加了验证，以确保聚合的容器巨页限制小于或等于Pod级别的限制。
- 在Kubernetes 1.34中，在CustomResourceDefinition架构中引入了对Kubernetes短名称和长名称的OpenAPI格式支持。
- 在Kubernetes 1.34中，Kube-apiserver配置文件中的--authentication-config选项，其支持的AuthenticationConfiguration类型已提升至 apiserver.config.k8s.io/v1。
- 在Kubernetes 1.34中，JobPodReplacementPolicy特性开关已锁定为开启状态，并将在未来的Kubernetes版本中移除。
- 在Kubernetes 1.34中，AuthorizeWithSelectors和AuthorizeNodeWithSelectors特性开关已稳定并锁定为开启状态。
- 在Kubernetes 1.34中，Downward API的resourceFieldRef字段的回退行为已更新，以考虑Pod级别的资源：如果未设置容器级别的资源限制，现在会优先使用Pod级别的资源限制，然后再回退到节点可分配资源。
- 在Kubernetes 1.34中，零值的metadata.creationTimestamp现在被省略，不再在JSON、YAML和CBOR输出中序列化为显式的null。

#### CCE对Kubernetes 1.34版本的增强

在版本维护周期中，CCE会对Kubernetes 1.34版本进行定期的更新，并提供功能增强。

关于CCE集群版本的更新说明，请参见[补丁版本发布说明](https://support.huaweicloud.com/usermanual-cce/cce_10_0405.html)。

#### 参考链接

关于Kubernetes 1.34与其他版本的性能对比和功能演进的更多信息，请参考：[Kubernetes v1.34 Release Notes](https://github.com/kubernetes/kubernetes/blob/master/CHANGELOG/CHANGELOG-1.34.md)
