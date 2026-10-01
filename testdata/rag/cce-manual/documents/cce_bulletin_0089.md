# Kubernetes 1.29版本说明

来源：https://support.huaweicloud.com/usermanual-cce/cce_bulletin_0089.html
目录：用户指南 > 集群 > 集群版本发布说明 > Kubernetes版本发布记录 > Kubernetes 1.29版本说明
更新时间：2026-08-12 GMT+08:00

云容器引擎（CCE）严格遵循社区一致性认证，现已支持Kubernetes 1.29集群版本特性。本文介绍Kubernetes 1.29版本的变更说明。

#### 索引

- [新增特性及特性增强](https://support.huaweicloud.com/usermanual-cce/cce_bulletin_0089.html)
- [API变更与弃用](https://support.huaweicloud.com/usermanual-cce/cce_bulletin_0089.html)
- [非兼容变更](https://support.huaweicloud.com/usermanual-cce/cce_bulletin_0089.html)
- [CCE对Kubernetes 1.29版本的增强](https://support.huaweicloud.com/usermanual-cce/cce_bulletin_0089.html)
- [参考链接](https://support.huaweicloud.com/usermanual-cce/cce_bulletin_0089.html)

#### 新增特性及特性增强

- Service的负载均衡IP模式（Alpha）

  在Kubernetes1.29版本，Service的负载均衡IP模式以Alpha版本正式发布。其在Service的status中新增字段ipMode，用于配置集群内Service到Pod的流量转发模式。当设置为VIP时，目的地址为负载均衡IP和端口的流量将由kube-proxy重定向到目标节点，当设置为Proxy时，流量将被发送到负载均衡器，然后由负载均衡器转发到目标节点。这项特性将有助于解决流量绕过负载均衡器所导致的负载均衡器功能缺失问题。更多使用细节请参考[Service的负载均衡IP模式](https://kubernetes.io/zh-cn/blog/2023/12/18/kubernetes-1-29-feature-loadbalancer-ip-mode-alpha/)。
- NFTables代理模式（Alpha）

  在Kubernetes1.29版本，NFTables代理模式以Alpha版本正式发布。该特性允许kube-proxy运行在NFTables模式，在该模式下，kube-proxy使用内核netfilters子系统的nftables API来配置数据包转发规则。更多使用细节请参考[NFTables代理模式](https://kubernetes.io/docs/reference/networking/virtual-ips/#proxy-mode-nftables)。
- 未使用容器镜像的垃圾收集（Alpha）

  在Kubernetes1.29版本，未使用容器镜像的垃圾收集以Alpha版本正式发布。该特性允许用户为每个节点配置本地镜像未被使用的最长时间，超过这个时间镜像将被垃圾回收。配置方法为使用kubelet配置文件中的ImageMaximumGCAge字段。更多使用细节请参考[未使用容器镜像的垃圾收集](https://kubernetes.io/zh-cn/docs/concepts/architecture/garbage-collection/#image-maximum-age-gc)。
- PodLifecycleSleepAction（Alpha）

  在Kubernetes1.29版本，PodLifecycleSleepAction以Alpha版本正式发布。该特性在容器生命周期回调中引入了Sleep回调程序，可以配置让容器在启动后和停止前暂停一段指定的时间。更多使用细节请参考[PodLifecycleSleepAction](https://kubernetes.io/zh-cn/docs/concepts/containers/container-lifecycle-hooks/#回调处理程序的实现)。
- KubeletSeparateDiskGC（Alpha）

  在Kubernetes1.29版本，KubeletSeparateDiskGC以Alpha版本正式发布。该特性启用后，即使在容器镜像和容器位于独立文件系统的情况下，也能进行垃圾回收。
- matchLabelKeys/mismatchLabelKeys（Alpha）

  在Kubernetes1.29版本，matchLabelKeys/mismatchLabelKeys以Alpha版本正式发布。该特性启用后，在Pod的亲和/反亲和配置中新增了matchLabelKeys/mismatchLabelKeys字段，可配置更丰富的Pod间亲和/反亲和策略。更多使用细节请参考[matchLabelKeys/mismatchLabelKeys](https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/#matchlabelkeys)。
- clusterTrustBundle投射卷（Alpha）

  在Kubernetes1.29版本，clusterTrustBundle投射卷以Alpha版本正式发布。该特性启用后，支持将ClusterTrustBundle对象以自动更新的文件的形式注入卷。更多使用细节请参考[clusterTrustBundle投射卷](https://kubernetes.io/docs/concepts/storage/projected-volumes/#clustertrustbundle)。
- 基于运行时类的镜像拉取（Alpha）

  在Kubernetes1.29版本中，基于运行时类的镜像拉取以Alpha版本正式发布。该特性启用后， kubelet 会通过一个元组（镜像名称，运行时处理程序）而不仅仅是镜像名称或镜像摘要来引用容器镜像。 您的容器运行时可能会根据选定的运行时处理程序调整其行为。基于运行时类来拉取镜像对于基于VM的容器会有帮助。更多使用细节请参考[基于运行时类的镜像拉取](https://kubernetes.io/zh-cn/docs/concepts/containers/images/#image-pull-per-runtime-class)。
- PodReadyToStartContainers状况达到Beta

  在Kubernetes1.29版本，PodReadyToStartContainers状况特性达到Beta版本。其在Pod的status中新增了一个名为PodReadyToStartContainers的Condition，该Condition为true表示Pod的沙箱已就绪，可以开始创建业务容器。该特性使得集群管理员可以更清晰和全面地查看 Pod 沙箱的创建完成和容器的就绪状态，增强了指标监控和故障排查能力。更多使用细节请参考[PodReadyToStartContainersCondition](https://kubernetes.io/zh-cn/blog/2023/12/19/pod-ready-to-start-containers-condition-now-in-beta/)。
- Job相关特性
  - Pod更换策略达到Beta

    在Kubernetes1.29版本，Pod更换策略特性达到Beta版本。该特性确保只有Pod达到Failed阶段（status.phase: Failed）才会被替换，而不是当删除时间戳不为空时，Pod仍处于删除过程中就重建Pod，以此避免出现2个Pod同时占用索引和节点资源。
  - 逐索引的回退限制达到Beta

    在Kubernetes1.29版本，逐索引的回退限制特性达到Beta版本。默认情况下，带索引的 Job（Indexed Job）的 Pod 失败情况会被统计下来，受 .spec.backoffLimit 字段所设置的全局重试次数限制。这意味着，如果存在某个索引值的 Pod 一直持续失败，则会 Pod 会被重新启动，直到重试次数达到限制值。 一旦达到限制值，整个 Job 将被标记为失败，并且对应某些索引的 Pod 甚至可能从不曾被启动。该特性可以在某些索引值的 Pod 失败的情况下，仍完成执行所有索引值的 Pod，并且通过避免对持续失败的、特定索引值的Pod进行不必要的重试，更好地利用计算资源。
- 原生边车容器达到Beta

  在Kubernetes1.29版本，原生边车容器特性达到Beta版本。其在initContainers中新增了restartPolicy字段，当配置为Always时表示启用边车容器。边车容器和业务容器部署在同一个Pod中，但并不会延长Pod的生命周期。边车容器常用于网络代理、日志收集等场景。更多使用细节请参考[边车容器](https://kubernetes.io/zh-cn/docs/concepts/workloads/pods/sidecar-containers/)。
- 传统ServiceAccount令牌清理器达到Beta

  在Kubernetes1.29版本，传统ServiceAccount令牌清理器特性达到Beta版本。其作为kube-controller-manager的一部分运行，每24小时检查一次，查看是否有任何自动生成的传统ServiceAccount令牌在特定时间段内（默认为一年，通过--legacy-service-account-token-clean-up-period指定）未被使用。如果有的话，清理器会将这些令牌标记为无效，并添加kubernetes.io/legacy-token-invalid-since标签，其值为当前日期。如果一个无效的令牌在特定时间段（默认为1年，通过--legacy-service-account-token-clean-up-period指定）内未被使用，清理器将会删除它。更多使用细节请参考[传统ServiceAccount令牌清理器](https://kubernetes.io/zh-cn/docs/reference/access-authn-authz/service-accounts-admin/#传统-serviceaccount-令牌清理器)。
- DevicePluginCDIDevices达到Beta

  在Kubernetes1.29版本，DevicePluginCDIDevices特性达到Beta版本。该特性在DeviceRunContainerOptions增加CDIDevices字段，使得设备插件开发者可以直接将CDI设备名称传递给支持CDI的容器运行时。
- PodHostIPs达到Beta

  在Kubernetes1.29版本中，PodHostIPs特性达到Beta版本。该特性在Pod和downward API的Status中增加hostIPs字段，用于将节点IP地址暴露给工作负载。该字段是hostIP的双栈协议版本，第一个IP始终与hostIP相同。
- API优先级和公平性达到GA

  在Kubernetes1.29版本，API优先级和公平性（APF）特性达到GA版本。APF以更细粒度的方式对请求进行分类和隔离，提升最大并发限制，并且它还引入了空间有限的排队机制，因此在非常短暂的突发情况下，API 服务器不会拒绝任何请求。 通过使用公平排队技术从队列中分发请求，这样，一个行为不佳的控制器就不会导致其他控制器异常 （即使优先级相同）。更多使用细节请参考[API优先级和公平性](https://kubernetes.io/zh-cn/docs/concepts/cluster-administration/flow-control/)。
- APIListChunking达到GA

  在Kubernetes1.29版本，APIListChunking特性达到GA版本。该特性允许客户端在List请求中进行分页，避免一次性返回过多数据而导致的性能问题。
- ServiceNodePortStaticSubrange达到GA

  在Kubernetes1.29版本，ServiceNodePortStaticSubrange特性达到GA版本。该特性kubelet会根据Nodeport范围计算出预留地址大小，并将Nodeport划分为静态段和动态段。在Nodeport自动分配时，优先分配在动态段，这有助于减小指定静态段分配时的端口冲突。更多使用细节请参考[ServiceNodePortStaticSubrange](https://kubernetes.io/zh-cn/blog/2023/05/11/nodeport-dynamic-and-static-allocation/)
- PersistentVolume的阶段转换时间戳达到Beta

  在Kubernetes1.29版本，PersistentVolume的阶段转换时间戳特性达到Beta版本。该特性在PV的status中添加了一个lastPhaseTransitionTime字段，表示PV上一次phase变化的时间。通过该字段，集群管理员可以跟踪PV上次转换到不同阶段的时间，从而实现更高效、更明智的资源管理。更多使用细节请参考[PersistentVolume的阶段转换时间戳](https://kubernetes.io/zh-cn/blog/2023/10/23/persistent-volume-last-phase-transition-time/)。
- ReadWriteOncePod达到GA

  在Kubernetes1.29版本中，ReadWriteOncePod特性达到GA版本。该特性允许用户在PVC中配置访问模式为ReadWriteOncePod，确保同时只有一个 Pod能够修改存储中的数据，以防止数据冲突或损坏。更多使用细节请参考[ReadWriteOncePod](https://kubernetes.io/docs/tasks/administer-cluster/change-pv-access-mode-readwriteoncepod/)。
- CSINodeExpandSecret达到GA

  在Kubernetes1.29版本中，CSINodeExpandSecret特性达到GA版本。该特性允许在添加节点时将Secret身份验证数据传递到CSI驱动以供后者使用。
- CRD验证表达式语言达到GA

  在Kubernetes1.29版本中，CRD验证表达式语言特性达到GA版本。该特性允许用户在CRD中使用通用表达式语言（CEL）定义校验规则，相比webhook更加高效。更多使用细节请参考[CRD校验规则](https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definitions/#validation-rules)。

#### API变更与弃用

- 在Kubernetes1.29版本中，新创建的CronJob不再支持在.spec.schedule中通过TZ或者CRON\_TZ配置时区，请使用.spec.timeZone替代。已经创建的CronJob不受此影响。
- 在Kubernetes1.29版本中，移除了alpha API ClusterCIDR。
- 在Kubernetes1.29版本中，kube-apiserver新增启动参数--authentication-config，用于指定AuthenticationConfiguration文件地址，该启动参数与--oidc-\*启动参数互斥。
- 在Kubernetes1.29版本中，移除了KubeSchedulerConfiguration的API版本kubescheduler.config.k8s.io/v1beta3，请迁移至kubescheduler.config.k8s.io/v1。
- 在Kubernetes1.29版本中，将CEL表达式添加到v1alpha1 AuthenticationConfiguration中。
- 在Kubernetes1.29版本中，新增对象ServiceCIDR，允许用户动态配置集群分配Service的ClusterIP时所使用的地址范围。
- 在Kubernetes1.29版本中，kube-proxy新增启动参数--conntrack-udp-timeout、--conntrack-udp-timeout-stream，可对nf\_conntrack\_udp\_timeout和nf\_conntrack\_udp\_timeout\_stream内核参数进行设置 。
- 在Kubernetes1.29版本中，将CEL表达式的支持添加到v1alpha1 AuthenticationConfiguration的WebhookMatchCondition中。
- 在Kubernetes1.29版本中，PVC.spec.Resource的类型由原先的ResourceRequirements替换为VolumeResourceRequirements。
- 在Kubernetes1.29版本中，将PodFailurePolicyRule中的OnPodConditions转变为可选字段。
- 在Kubernetes1.29版本中，FlowSchema与PriorityLevelConfiguration的API版本flowcontrol.apiserver.k8s.io/v1beta3已升级至flowcontrol.apiserver.k8s.io/v1，并进行了以下更改
  - PriorityLevelConfiguration：.spec.limited.nominalConcurrencyShares字段在省略时自动设为默认值30，并且为了确保与1.28兼容，在1.29中v1版本该字段不允许显示指定为0。在1.30中，将允许v1版本该字段显示指定为0。flowcontrol.apiserver.k8s.io/v1beta3已废弃，并在1.32中不再支持。
- 在Kubernetes1.29版本中， 优化了kube-proxy的命令行文档，kube-proxy实际上不会将任何socket绑定到由--bind-address启动参数指定的IP。
- 在Kubernetes1.29版本中，移除了selectorSpread调度器插件，使用podTopologySpread替代。
- 在Kubernetes1.29版本中，当CSI-Node-Driver没有在运行时，NodeStageVolume操作会重试。
- 在Kubernetes1.29版本中，ValidatingAdmissionPolicy支持对CRD资源进行校验。使用该特性需开启特性门控ValidatingAdmissionPolicy。
- 在Kubernetes1.29版本中，kube-proxy新增启动参数--nf-conntrack-tcp-be-liberal，可对内核参数nf\_conntrack\_tcp\_be\_liberal进行配置。
- 在Kubernetes1.29版本中，kube-proxy新增启动参数--init-only，设置后使kube-proxy的init容器在特权模式下运行，进行一些初始化配置，然后退出。
- 在Kubernetes1.29版本中，CRI的返回体中新增容器的fileSystem字段，表示容器的fileSystem使用信息，而原先只包含镜像的fileSystem。
- 在Kubernetes1.29版本中，所有内置的CloudProvider全部默认设置为关闭，如果仍需使用，可通过配置DisableCloudProviders和DisableKubeletCloudCredentialProvider特性门控来选择性关闭或者打开。
- 在Kubernetes1.29版本中，kubelet可通过--node-ips来设置双栈IP（kubelet设置--cloud-provider=external，此时可以使用--node-ips参数来设置节点地址(IPv4/IPv6)），使用该特性需开启特性门控CloudDualStackNodeIPs。

#### 非兼容变更

Kubernetes v1.29中更改kube-proxy启动的行为，允许kube-proxy设置低于节点sysctl设置的参数值。如节点nf\_conntrack\_max内核参数值设置为1000000，而kube-proxy计算的值为131072，那么此内核参数会使用kube-proxy计算的值131072。

社区PR链接为：<https://github.com/kubernetes/kubernetes/pull/120448>

#### CCE对Kubernetes 1.29版本的增强

在版本维护周期中，CCE会对Kubernetes 1.29版本进行定期的更新，并提供功能增强。

关于CCE集群版本的更新说明，请参见[补丁版本发布说明](https://support.huaweicloud.com/usermanual-cce/cce_10_0405.html)。

#### 参考链接

关于Kubernetes 1.29与其他版本的性能对比和功能演进的更多信息，请参考：[Kubernetes v1.29 Release Notes](https://github.com/kubernetes/kubernetes/blob/master/CHANGELOG/CHANGELOG-1.29.md)
