# （停止维护）Kubernetes 1.25版本说明

来源：https://support.huaweicloud.com/usermanual-cce/cce_bulletin_0058.html
目录：用户指南 > 集群 > 集群版本发布说明 > Kubernetes版本发布记录 > （停止维护）Kubernetes 1.25版本说明
更新时间：2026-05-29 GMT+08:00

云容器引擎（CCE）严格遵循社区一致性认证。本文介绍Kubernetes 1.25版本相对于1.23版本所做的变更说明。

#### 索引

- [主要特性](https://support.huaweicloud.com/usermanual-cce/cce_bulletin_0058.html)
- [弃用和移除](https://support.huaweicloud.com/usermanual-cce/cce_bulletin_0058.html)
- [CCE对Kubernetes 1.25版本的增强](https://support.huaweicloud.com/usermanual-cce/cce_bulletin_0058.html)
- [参考链接](https://support.huaweicloud.com/usermanual-cce/cce_bulletin_0058.html)

#### 主要特性

**Kubernetes 1.25版本**

- Pod Security Admission进入稳定阶段，并移除PodSecurityPolicy

  PodSecurityPolicy被废弃，并提供Pod Security Admission取代，具体的迁移方法可参见[从PodSecurityPolicy迁移到内置的PodSecurity准入控制器](https://kubernetes.io/zh-cn/docs/tasks/configure-pod-container/migrate-from-psp/)。
- Ephemeral Containers进入稳定阶段

  [临时容器](https://kubernetes.io/zh-cn/docs/concepts/workloads/pods/ephemeral-containers/)是在现有的Pod中存在有限时间的容器。它对故障排除特别有用，特别是当需要检查另一个容器，但因为该容器已经崩溃或其镜像缺乏调试工具不能使用kubectl exec时。
- 对cgroups v2的支持进入稳定阶段

  Kubernetes支持cgroups v2，与cgroups v1相比提供了一些改进，详情请参见[cgroups v2](https://kubernetes.io/zh-cn/docs/concepts/architecture/cgroups/)。
- SeccompDefault提升到Beta状态

  如果要开启该特性，需要给kubelet增加启动参数为--seccomp-default=true，这样会默认开启seccomp为RuntimeDefault，提升整个系统的安全。1.25集群将不再支持使用注解“seccomp.security.alpha.kubernetes.io/pod”和“container.seccomp.security.alpha.kubernetes.io/annotation”来使用seccomp，请使用pod或container中“securityContext.seccompProfile”字段替代，详情请参见[为Pod或容器配置安全上下文](https://kubernetes.io/zh-cn/docs/tasks/configure-pod-container/security-context/)。

  ![](https://support.huaweicloud.com/intl/zh-cn/usermanual-cce/public_sys-resources/note_3.0-zh-cn.png)

  特性开启后可能应用所需的系统调用会被runtime限制，所以开启后应确保在测试环境调试，不会对应用造成影响。
- 网络策略中的EndPort进入稳定阶段

  Network Policy中的EndPort已进入稳定状态，该特性于1.21版本合入。主要是在NetworkPolicy新增EndPort，可以指定一个Port范围，避免声明每一个Port。
- 本地临时容器存储容量隔离进入稳定阶段

  本地临时存储容量隔离功能提供了对Pod之间本地临时存储容量隔离的支持，如EmptyDir。因此，如果一个Pod对本地临时存储容量的消耗超过该限制，就可以通过驱逐Pod来硬性限制其对共享资源的消耗。
- CRD验证表达式语言升级为Beta阶段

  CRD验证表达式语言已升级为 beta 版本， 这使得声明如何使用[通用表达式语言（CEL）](https://github.com/google/cel-spec)验证自定义资源成为可能。 请参考[验证规则](https://kubernetes.io/zh-cn/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definitions/#validation-rules)指导。
- 引入KMS v2 API

  在Kubernetes 1.25版本，引入KMS v2 alpha1 API以提升性能，实现轮替与可观察性改进。 此API使用AES-GCM替代了AES-CBC，通过DEK实现静态数据加密（Kubernetes Secrets），此过程中无需您额外操作，且支持通过AES-GCM和AES-CBC进行读取。 更多信息，请参考[使用 KMS provider进行数据加密指南](https://kubernetes.io/zh-cn/docs/tasks/administer-cluster/kms-provider/)。
- Pod新增网络就绪状况

  Kubernetes 1.25引入了对kubelet所管理的新的Pod状况PodHasNetwork的Alpha支持， 该状况位于Pod的status字段中 。详情请参见[Pod网络就绪](https://kubernetes.io/zh-cn/docs/concepts/workloads/pods/pod-lifecycle/#pod-has-network)。
- 应用滚动上线所用的两个特性进入稳定阶段
  - 在Kubernetes 1.25版本，StatefulSet的minReadySeconds进入稳定阶段，允许每个Pod等待一段预期时间来减缓StatefulSet的滚动上线。更多信息，请参见[最短就绪秒数](https://kubernetes.io/zh-cn/docs/concepts/workloads/controllers/statefulset/#minimum-ready-seconds)。
  - 在Kubernetes 1.25版本，DaemonSet的maxSurge进入稳定阶段，允许DaemonSet工作负载在滚动上线期间在一个节点上运行同一 Pod的多个实例，有助于将DaemonSet的停机时间降到最低。DaemonSet不允许maxSurge和hostPort同时使用，因为两个活跃的Pod无法共享同一节点的相同端口。更多信息，请参见[DaemonSet工作负载滚动上线](https://kubernetes.io/zh-cn/docs/tasks/manage-daemon/update-daemon-set/)。
- 对使用用户命名空间运行Pod提供Alpha支持

  对使用user namespace运行Pod提供alpha支持，将Pod内的root用户映射到容器外的非零ID，使得从容器角度看是root身份运行，而从主机角度看是常规的非特权用户。目前尚处于内测阶段，需要开启特性门控UserNamespacesStatelessPodsSupport， 且要求容器运行时必须能够支持此功能。更多信息，请参见[对使用user namespace运行Pod提供alpha支持](https://kubernetes.io/zh-cn/blog/2022/10/03/userns-alpha/)。

**Kubernetes 1.24版本**

- 从kubelet中删除 Dockershim

  Dockershim自1.20版本被标记为废弃以来，在1.24版本正式从Kubelet代码中移除。如果还想使用Docker作为容器运行时的话，需要切换到cri-dockerd，或者使用其他支持CRI的运行时比如Containerd/CRI-O等。

  从Docker Engine 切换到Containerd的流程请参见[将节点容器运行时从Docker迁移到Containerd](https://support.huaweicloud.com/usermanual-cce/cce_10_0601.html)。

  ![](https://support.huaweicloud.com/intl/zh-cn/usermanual-cce/public_sys-resources/note_3.0-zh-cn.png)

  您需要注意排查是否有agent或者应用强依赖Docker Engine的，比如在代码中使用docker ps，docker run， docker inspect等，需要注意兼容多种runtime，以及切换到标准cri接口。
- Beta APIs默认关闭

  在社区移除一些长期Beta API的过程中发现，90%的集群管理员并没有关心Beta API默认开始，其实Beta特性是不推荐在生产环境中使用，但是因为默认的打开策略，导致这些API在生产环境中都被默认开启，这样会因为Beta特性的bug带来一些风险，以及升级的迁移的风险。所以在1.24版本开始，Beta API默认关闭，之前已经默认开启的Beta API会保持默认开启。
- 支持OpenAPI v3

  在Kubernetes 1.24版本后，OpenAPI V3默认开启。
- 存储容量跟踪特性进入稳定阶段

  在Kubernetes 1.24版本后，CSIStorageCapacity API支持显示当前可用的存储大小，确保Pod调度到足够存储容量的节点上，减少Volumes创建和挂载失败导致的Pod调度延迟，详细信息请参见[存储容量](https://kubernetes.io/zh-cn/docs/concepts/storage/storage-capacity/)。
- gRPC 探针升级到Beta阶段

  在Kubernetes 1.24版本后，gRPC探针进入Beta，默认可用特性门控参数GRPCContainerProbe，使用方式请参见[配置探针](https://kubernetes.io/zh-cn/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/#configure-probes)。
- 特性门控LegacyServiceAccountTokenNoAutoGeneration默认启用

  LegacyServiceAccountTokenNoAutoGeneration特性门控进入beta状态，默认为开启状态，开启后将不再为Service Account自动生成Secret Token。如果需要使用永不过期的Token，需要自己新建Secrets并挂载，详情请参见[服务账号令牌Secret](https://kubernetes.io/zh-cn/docs/concepts/configuration/secret/#service-account-token-secrets)。
- 避免 IP 分配给服务的冲突

  Kubernetes 1.24引入了一项新功能，允许为服务的静态IP地址分配软保留范围。 通过手动启用此功能，集群将从服务IP地址池中自动分配IP，从而降低冲突风险。详情请参见[避免为Services分配IP地址时发生冲突](https://kubernetes.io/zh-cn/blog/2022/05/23/service-ip-dynamic-and-static-allocation/)。
- 基于Go 1.18编译

  在Kubernetes 1.24版本后，Kubernetes基于Go 1.18编译，默认不再支持SHA-1哈希算法验证证书签名，例如SHA1WithRSA、ECDSAWithSHA1算法，推荐使用SHA256算法生成的证书进行认证。
- StatefulSet支持设置最大不可用副本数

  在Kubernetes 1.24版本后，StatefulSets支持可配置maxUnavailable参数，使得滚动更新时可以更快地停止Pods。
- 节点非体面关闭进入Alpha阶段

  在Kubernetes 1.24中，节点非体面关闭特性是Alpha版。当kubelet的节点关闭管理器可以检测到即将到来的节点关闭操作时，节点关闭才被认为是体面的。详情请参见[处理节点非体面关闭](https://kubernetes.io/zh-cn/docs/concepts/cluster-administration/node-shutdown/#non-graceful-node-shutdown)。

#### 弃用和移除

**Kubernetes 1.25版本**

- 清理iptables链的所有权

  Kubernetes通常创建iptables链来确保这些网络数据包到达， 这些iptables链及其名称属于Kubernetes内部实现的细节，仅供内部使用场景，目前有些组件依赖于这些内部实现细节，Kubernetes总体上不希望支持某些工具依赖这些内部实现细节。详细信息，请参见[Kubernetes的iptables链不是API](https://kubernetes.io/zh-cn/blog/2022/09/07/iptables-chains-not-api/)。

  在Kubernetes 1.25版本后，Kubelet通过IPTablesCleanup特性门控分阶段完成迁移，是为了不在NAT表中创建iptables链，例如KUBE-MARK-DROP、KUBE-MARK-MASQ、KUBE-POSTROUTING。关于清理iptables链所有权的信息，请参见[清理IPTables链的所有权](https://github.com/kubernetes/enhancements/issues/3178)。
- 存储驱动的弃用和移除，移除云服务厂商的in-tree卷驱动。

**Kubernetes 1.24版本**

- 在Kubernetes 1.24版本后，Service.Spec.LoadBalancerIP被弃用，因为它无法用于双栈协议。请使用自定义annotation。
- 在Kubernetes 1.24版本后，kube-apiserver移除参数--address、--insecure-bind-address、--port、--insecure-port=0。
- 在Kubernetes 1.24版本后，kube-controller-manager和kube-scheduler移除启动参数--port=0和--address。
- 在Kubernetes 1.24版本后，kube-apiserver --audit-log-version和--audit-webhook-version仅支持audit.k8s.io/v1，Kubernetes 1.24移除audit.k8s.io/v1[alpha|beta]1，只能使用audit.k8s.io/v1。
- 在Kubernetes 1.24版本后，kubelet移除启动参数--network-plugin，仅当容器运行环境设置为Docker时，此特定于Docker的参数才有效，并会随着Dockershim一起删除。
- 在Kubernetes 1.24版本后，动态日志清理功能已经被废弃，并在Kubernetes 1.24版本移除。该功能引入了一个日志过滤器，可以应用于所有Kubernetes系统组件的日志，以防止各种类型的敏感信息通过日志泄漏。此功能可能导致日志阻塞，所以废弃，更多信息请参见[Dynamic log sanitization](https://github.com/kubernetes/kubernetes/pull/107207)和 [KEP-1753](https://github.com/kubernetes/enhancements/tree/master/keps/sig-instrumentation/1753-logs-sanitization#deprecation)。
- VolumeSnapshot v1beta1 CRD在Kubernetes 1.20版本中被废弃，在Kubernetes 1.24版本中移除，需改用v1版本。
- 在Kubernetes 1.24版本后，移除自1.11版本就废弃的service annotation tolerate-unready-endpoints，使用Service.spec.publishNotReadyAddresses代替。
- 在Kubernetes 1.24版本后，废弃metadata.clusterName字段，并将在下一个版本中删除。
- Kubernetes 1.24及以后的版本，去除了kube-proxy监听NodePort的逻辑，在NodePort与内核net.ipv4.ip\_local\_port\_range范围有冲突的情况下，可能会导致偶发的TCP无法连接的情况，导致健康检查失败、业务异常等问题。升级前，请确保集群没有NodePort端口与任意节点net.ipv4.ip\_local\_port\_range范围存在冲突。更多信息，请参见[Kubernetes社区PR](https://github.com/kubernetes/kubernetes/pull/108496)。

#### CCE对Kubernetes 1.25版本的增强

在版本维护周期中，CCE会对Kubernetes 1.25版本进行定期的更新，并提供功能增强。

关于CCE集群版本的更新说明，请参见[CCE集群版本发布说明](https://support.huaweicloud.com/usermanual-cce/cce_10_0405.html)。

#### 参考链接

关于Kubernetes 1.25与其他版本的性能对比和功能演进的更多信息，请参考：

- [Kubernetes v1.25 Release Notes](https://github.com/kubernetes/kubernetes/blob/master/CHANGELOG/CHANGELOG-1.25.md)
- [Kubernetes v1.24 Release Notes](https://github.com/kubernetes/kubernetes/blob/master/CHANGELOG/CHANGELOG-1.24.md)
