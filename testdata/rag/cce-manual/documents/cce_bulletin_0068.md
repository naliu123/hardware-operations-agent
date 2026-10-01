# （停止维护） Kubernetes 1.28版本说明

来源：https://support.huaweicloud.com/usermanual-cce/cce_bulletin_0068.html
目录：用户指南 > 集群 > 集群版本发布说明 > Kubernetes版本发布记录 > （停止维护）Kubernetes 1.28版本说明
更新时间：2026-04-03 GMT+08:00

云容器引擎（CCE）严格遵循社区一致性认证，现已支持Kubernetes 1.28集群版本特性。本文介绍Kubernetes 1.28版本的变更说明。

#### 索引

- [重要说明](https://support.huaweicloud.com/usermanual-cce/cce_bulletin_0068.html)
- [新增特性及特性增强](https://support.huaweicloud.com/usermanual-cce/cce_bulletin_0068.html)
- [API变更与弃用](https://support.huaweicloud.com/usermanual-cce/cce_bulletin_0068.html)
- [特性门禁及命令行参数](https://support.huaweicloud.com/usermanual-cce/cce_bulletin_0068.html)
- [CCE对Kubernetes 1.28版本的增强](https://support.huaweicloud.com/usermanual-cce/cce_bulletin_0068.html)
- [参考链接](https://support.huaweicloud.com/usermanual-cce/cce_bulletin_0068.html)

#### 重要说明

- 在Kubernetes 1.28版本，调度框架发生变化，减少无用的重试，从而提高调度程序的整体性能。如果开发人员在集群中使用了自定义调度程序插件，请参见[调度框架变化](https://github.com/kubernetes/kubernetes/blob/master/CHANGELOG/CHANGELOG-1.28.md#no-really-you-must-read-this-before-you-upgrade)进行适配升级。
- 在Kubernetes 1.28 版本，Ceph FS 树内插件已在 v1.28 中弃用，并计划在 v1.31 中删除（社区没有计划进行 CSI 迁移）。 建议使用 [Ceph CSI](https://github.com/ceph/ceph-csi) 第三方存储驱动程序作为替代方案。
- 在Kubernetes 1.28 版本，Ceph RBD 树内插件已在 v1.28 中弃用，并计划在 v1.31 中删除（社区没有计划进行 CSI 迁移）。建议使用 RBD 模式的 [Ceph CSI](https://github.com/ceph/ceph-csi) 第三方存储驱动程序作为替代方案。

#### 新增特性及特性增强

社区特性的Alpha阶段默认禁用、Beta阶段一般默认启用、GA阶段将一直默认启用，且不能禁用（会在后续版本中删除这个开关功能）。CCE对新特性的策略与社区保持一致。

- 版本偏差策略扩展至3个版本

  从1.28控制平面/1.25工作节点开始，Kubernetes版本偏差策略将支持的控制平面/工作节点偏差扩展到3个版本。 这使得节点的年度次要版本升级成为可能，同时保持受支持的次要版本。更多细节请参考[版本偏差策略](https://kubernetes.io/zh-cn/releases/version-skew-policy/)。
- 可追溯的默认StorageClass进阶至GA

  在Kubernetes 1.28版本，可追溯默认StorageClass赋值现已进阶至GA。 这项增强特性极大地改进了默认的StorageClasses为PersistentVolumeClaim（PVC）赋值的方式。

  PersistentVolume（PV）控制器已修改为：当未设置storageClassName时，自动向任何未绑定的PersistentVolumeClaim分配一个默认的StorageClass。此外，API 服务器中的PersistentVolumeClaim准入验证机制也已调整为允许将值从未设置状态更改为实际的StorageClass名称。更多使用细节请参考[默认StorageClass赋值](https://kubernetes.io/zh-cn/docs/concepts/storage/persistent-volumes/#retroactive-default-storageclass-assignment)。
- 原生边车容器（Alpha）

  在Kubernetes 1.28版本，原生边车容器以Alpha版本正式发布。其在Init容器中添加了一个新的restartPolicy字段， 该字段在SidecarContainers特性门控启用时可用。需要注意的是，原生边车容器目前仍有些问题需要解决，因此K8S社区建议仅在Alpha阶段的[短期测试集群](https://kubernetes.io/zh-cn/docs/reference/command-line-tools-reference/feature-gates/#feature-stages)中使用边车功能。 更多使用细节请参考[原生边车容器](https://kubernetes.io/zh-cn/blog/2023/08/25/native-sidecar-containers/)。
- 混合版本代理（Alpha）

  在Kubernetes 1.28版本，发布了用于改进集群安全升级的新机制（混合版本代理）。该特性为Alpha特性。当集群进行升级时，集群中不同版本的kube-apiserver为不同的内置资源集（组、版本、资源）提供服务。在这种情况下资源请求如果由任一可用的apiserver提供服务，请求可能会到达无法解析此请求资源的apiserver中，导致请求失败。该特性能解决该问题。（主要注意的是，CCE本身提供的升级能力即可做到无损升级，因此不存在该特性涉及的场景）。更多使用细节请参考[混合版本代理](https://kubernetes.io/zh-cn/blog/2023/08/28/kubernetes-1-28-feature-mixed-version-proxy-alpha/)。
- 节点非体面关闭特性达到GA

  在Kubernetes 1.28版本，节点非体面关闭特性达到GA阶段。当一个节点被关闭但没有被Kubelet的Node Shutdown Manager检测到时，StatefulSet的Pod将会停留在终止状态，并且不能移动到新运行的节点上。当用户确认该节点已经处于不可恢复的情况下，可以手动为Node打上out-of-service的污点，以使得该节点上的StatefulSet的Pod和VolumeAttachments被强制删除，并在健康的Node上创建相应的Pod。更多使用细节请参考[节点非体面关闭](https://kubernetes.io/zh-cn/blog/2023/08/16/kubernetes-1-28-non-graceful-node-shutdown-ga/)。
- NodeSwap特性达到Beta

  在Kubernetes 1.28版本，NodeSwap能力进阶至Beta版本。目前仍然处于默认关闭状态，需要使用NodeSwap门控打开。该特性可以为Linux节点上运行的Kubernetes工作负载逐个节点地配置内存交换。需要注意的是，该特性虽然进阶至Beta特性，但仍然存在一些需要增强的问题和安全风险。更多使用细节请参考[NodeSwap特性](https://kubernetes.io/zh-cn/blog/2023/08/24/swap-linux-beta/)。
- Job相关特性

  在Kubernetes 1.28版本，增加了[Pod更换策略](https://kubernetes.io/zh-cn/docs/concepts/workloads/controllers/job/#pod-replacement-policy)和基于[带索引Job的回退限制](https://kubernetes.io/zh-cn/docs/concepts/workloads/controllers/job/#backoff-limit-per-index)两个alpha特性。

  - Pod更换策略

    默认情况下，当Pod进入终止（Terminating）状态（例如由于抢占或驱逐机制）时，Kubernetes会立即创建一个替换的Pod，因此这时会有两个Pod同时运行。

    在Kubernetes 1.28版本中可以使用JobPodReplacementPolicy 来启用该特性。可以在Job的Spec中定义podReplacementPolicy，目前仅可设置为Failed。在设置为Failed之后，Pod仅在达到Failed阶段时才会被替换，而不是在它们处于终止过程中（Terminating）时被替换。此外，您可以检查Job的.status.termination字段。该字段的值表示终止过程中的Job所关联的Pod数量。
  - 带索引Job的回退限制

    默认情况下，带索引的Job（Indexed Job）的 Pod 失败情况会被记录下来，受.spec.backoffLimit字段所设置的全局重试次数限制。这意味着，如果存在某个索引值的Pod一直持续失败，则Pod会被重新启动，直到重试次数达到限制值。一旦达到限制值，整个Job将被标记为失败，并且对应某些索引的Pod甚至可能从不曾被启动。

    在Kubernetes 1.28版本中，可以通过启用集群的JobBackoffLimitPerIndex特性门控来启用此特性。开启之后，允许在创建带索引的Job（Indexed Job）时指定.spec.backoffLimitPerIndex字段。当某个Job的失败次数超过设定的上限时，将不再进行重试。
- CEL相关特性

  在Kubernetes 1.28版本，CEL能力进行了相应的增强。

  - CRD使用CEL进行Validate的特性进阶至Beta

    该特性在v1.25版本就已经升级为Beta版本。通过将CEL表达式直接集成在CRD中，可以使开发者在不使用Webhook的情况下解决大部分对CR实例进行验证的用例。在未来的版本，将继续扩展CEL表达式的功能，以支持默认值和 CRD转换。
  - 基于CEL的准入控制进阶至Beta

    基于通用表达式语言 (CEL) 的准入控制是可定制的，对于kube-apiserver接收到的请求，可以使用CEL表达式来决定是否接受或拒绝请求，可作为Webhook准入控制的一种替代方案。在 v1.28 中，CEL准入控制被升级为Beta，同时添加了一些新功能，包括但不限于：

    - ValidatingAdmissionPolicy类型检查现在可以正确处理CEL表达式中的“authorizer”变量。
    - ValidatingAdmissionPolicy支持对messageExpression字段进行类型检查。
    - kube-controller-manager组件新增ValidatingAdmissionPolicy控制器，用来对ValidatingAdmissionPolicy中的CEL表达式做类型检查，并将原因保存在状态字段中。
    - 支持变量组合，可以在ValidatingAdmissionPolicy中定义变量，然后在定义其他变量时使用它。
    - 新增CEL库函数支持对Kubernetes的resource.Quantity类型进行解析。
- 其它特性说明
  - 在Kubernetes 1.28版本，ServiceNodePortStaticSubrange 特性为beta，允许保留静态端口范围，避免与动态分配端口冲突。具体细节请参考[为NodePort Service分配端口时避免冲突](https://kubernetes.io/zh-cn/blog/2023/05/11/nodeport-dynamic-and-static-allocation/)。
  - 在Kubernetes 1.28版本，增加了alpha特性ConsistentListFromCache，允许kube-apiserver从缓存中提供一致性列表，Get和List请求可以从缓存中读取数据，而不需要从etcd中获取。
  - 在Kubernetes 1.28版本，kubelet能够配置drop-in目录（alpha特性）。该特性允许向kubelet添加对“--config-dir”标志的支持，以允许用户指定一个插入目录，该目录将覆盖位于/etc/kubernetes/kubelet.conf位置的Kubelet的配置。
  - 在Kubernetes 1.28版本，ExpandedDNSConfig升级至GA，默认会被打开。该参数用于允许扩展DNS的配置。
  - 在Kubernetes 1.28版本，提供Alpha特性CRD Validation Ratcheting。该特性允许Patch或者Update请求没有更改任何不合法的字段，将允许CR验证失败。
  - 在Kubernetes 1.28版本，kube-controller-manager添加了--concurrent-cron-job-syncs flag用来设置cron job controller的workers数。

#### API变更与弃用

- 在Kubernetes 1.28版本，移除特性NetworkPolicyStatus，因此Network Policy不再有status属性。
- 在Kubernetes 1.28版本，Job对象中增加了新的annotationbatch.kubernetes.io/cronJob-scheduled-timestamp，表示Job的创建时间。
- 在Kubernetes 1.28版本，Job API中添加podReplacementPolicy和terminating字段，当前一旦先前创建的pod终止，Job就会立即启动替换pod。添加字段允许用户指定是在先前的Pod终止后立即更换Pod（原行为），还是在现有的Pod完全终止后才替换Pod（新行为）。这是一项Alpha级别特性，您可以通过在集群中启用[JobPodReplacementPolicy](https://kubernetes.io/zh-cn/blog/2023/08/21/kubernetes-1-28-jobapi-update/) 来启用该特性。
- 在Kubernetes 1.28版本，Job支持BackoffLimitPerIndex字段。当前使用的运行Job的策略是Job中的整个Pod共享一个Backoff机制，当Job达到Backoff的限制时，整个Job都会被标记为失败，并清理资源，包括尚未运行的index。此字段允许对单个的index设置Backoff。更多信息请参见[带索引Job的Backoff限制](https://kubernetes.io/zh-cn/docs/concepts/workloads/controllers/job/#backoff-limit-per-index)。
- 在Kubernetes 1.28版本，添加ServedVersions字段到 StorageVersion API中。该变化由混合代理版本特性引入。该增加字段ServedVersions用于表明API服务器可以提供的版本。
- 在Kubernetes 1.28版本，SelfSubjectReview 添加到authentication.k8s.io/v1中，并且kubectl auth whoami走向GA。
- 在Kubernetes 1.28版本，PersistentVolume有了一个新的字段LastPhaseTransitionTime，用来保存最近一次volume转变Phase的时间。
- 在Kubernetes 1.28版本，PVC.Status中移除resizeStatus，使用AllocatedResourceStatus替代。resizeStatus表示调整存储大小操作的状态，默认为空字符串。
- 在Kubernetes 1.28版本，设置了hostNetwork: true并且定义了ports的Pods，自动设置hostport字段。
- 在Kubernetes 1.28版本，StatefulSet的Pod索引设置为Pod的标签statefulset.kubernetes.io/pod-index。
- 在Kubernetes 1.28版本，Pod的Condition字段中的PodHasNetwork重命名为PodReadyToStartContainers，用来表明网络、卷等已成功创建，sandbox pod已经创建完成，可以启动容器。
- 在Kubernetes 1.28版本，在KubeSchedulerConfiguration中添加了新的配置选项delayCacheUntilActive，用于指定何时开始缓存数据，默认为False。该参数为true时，则启用Leader选举，用于动态选择Leader节点。此时，调度器会等到控制节点成为Leader节点之后，才开始缓存调度信息，从而降低控制节点的内存开销，但这也会导致故障转移速度变慢。
- 在Kubernetes 1.28版本，在admissionregistration.k8s.io/v1alpha1.ValidatingAdmissionPolicy中添加namespaceParamRef字段。
- 在Kubernetes 1.28版本，在CRD validation rules中添加reason和fieldPath，允许用户指定验证失败的原因和字段路径。
- 在Kubernetes 1.28版本，ValidatingAdmissionPolicy的CEL表达式通过namespaceObject支持namespace访问。
- 在Kubernetes 1.28版本，将API groups ValidatingAdmissionPolicy 和ValidatingAdmissionPolicyBinding提升到betav1。
- 在Kubernetes 1.28版本，ValidatingAdmissionPolicy 扩展了messageExpression字段，用来检查已解析类型。

#### 特性门禁及命令行参数

- 在Kubernetes 1.28版本，kubelet移除了flag –short。因此kubectl version 默认输出与kubectl version –short相同。
- 在Kubernetes 1.28版本，kube-controller-manager废弃flag--volume-host-cidr-denylist和--volume-host-allow-local-loopback。--volume-host-cidr-denylist是用逗号分隔的一个CIDR范围列表，禁止使用这些地址上的卷插件。--volume-host-allow-local-loopback为false时，禁止本地回路IP地址和--volume-host-cidr-denylist中所指定的CIDR范围。
- 在Kubernetes 1.28版本，kubelet --azure-container-registry-config被弃用并在未来的版本中会被删除。 请使用--image-credential-provider-config和--image-credential-provider-bin-dir来设置。
- 在Kubernetes 1.28版本，kube-scheduler: 删除了--lock-object-namespace和--lock-object-name。 请使用--leader-elect-resource-namespace和--leader-elect-resource-name或ComponentConfig来配置这些参数。（--lock-object-namespace用来定义锁对象的命名空间，--lock-object-name用来定义锁对象的名称）
- 在Kubernetes 1.28版本，KMSv1已弃用，以后只会接收安全更新。请改用KMSv2。在未来版本中，设置--feature-gates=KMSv1=true以使用已弃用的KMSv1功能。
- 在Kubernetes 1.28版本，移除了如下特性门禁：DelegateFSGroupToCSIDriver、DevicePlugins、KubeletCredentialProviders、MixedProtocolLBService、ServiceInternalTrafficPolicy、ServiceIPStaticSubrange、EndpointSliceTerminatingCondition。

#### CCE对Kubernetes 1.28版本的增强

在版本维护周期中，CCE会对Kubernetes 1.28版本进行定期的更新，并提供功能增强。

关于CCE集群版本的更新说明，请参见[CCE集群版本发布说明](https://support.huaweicloud.com/usermanual-cce/cce_10_0405.html)。

#### 参考链接

关于Kubernetes 1.28与其他版本的性能对比和功能演进的更多信息，请参考：[Kubernetes v1.28 Release Notes](https://github.com/kubernetes/kubernetes/blob/master/CHANGELOG/CHANGELOG-1.28.md)
