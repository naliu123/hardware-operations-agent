# Kubernetes 1.36版本说明

来源：https://support.huaweicloud.com/usermanual-cce/cce_bulletin_0131.html
目录：用户指南 > 集群 > 集群版本发布说明 > Kubernetes版本发布记录 > Kubernetes 1.36版本说明
更新时间：2026-06-05 GMT+08:00

云容器引擎（CCE）严格遵循社区一致性认证，现已支持Kubernetes 1.36集群版本特性。本文介绍Kubernetes 1.36版本的变更说明。

#### 索引

- [新增特性及特性增强](https://support.huaweicloud.com/usermanual-cce/cce_bulletin_0131.html)
- [API变更与弃用](https://support.huaweicloud.com/usermanual-cce/cce_bulletin_0131.html)
- [CCE对Kubernetes 1.36版本的增强](https://support.huaweicloud.com/usermanual-cce/cce_bulletin_0131.html)
- [参考链接](https://support.huaweicloud.com/usermanual-cce/cce_bulletin_0131.html)

#### 新增特性及特性增强

- Kubelet细粒度API授权（GA）

  在Kubernetes 1.36中，Kubelet细粒度API授权（Fine-grained Kubelet API Authorization）进阶至GA。该特性Kubelet API新增configz、healthz、pods独立RBAC授权子资源，保留原有nodes/proxy子资源实现向下兼容。详细使用方式请参考[Kubelet细粒度API授权](https://github.com/kubernetes/enhancements/blob/master/keps/sig-node/2862-fine-grained-kubelet-authz/README.md)。
- 卷组快照（GA）

  在Kubernetes 1.36中，卷组快照（Volume Group Snapshot）进阶至GA。此功能支持通过标签选择器聚合多个 PVC，为多卷应用整体创建崩溃一致性组快照，避免多卷分时快照导致的数据不一致问题。CCE存储插件将在未来适配该功能。详细介绍请参考[卷组快照](https://github.com/kubernetes/enhancements/blob/master/keps/sig-storage/3476-volume-group-snapshot/README.md)。
- 可变更CSINode可分配容量（GA）

  在Kubernetes 1.36中，可变更CSINode可分配容量（Mutable CSINode Allocatable）进阶至GA。此功能支持动态修改CSINode.Spec.Drivers[\*].Allocatable.Count字段，在卷挂载因容量不足失败时自动修正容量信息，避免调度器基于过时数据导致 Pod 卡住。详细介绍请参考[可变更CSINode可分配容量](https://github.com/kubernetes/enhancements/blob/master/keps/sig-storage/4876-mutable-csinode-allocatable/README.md)。
- 服务账号令牌外部签名（GA）

  在Kubernetes 1.36中，服务账号令牌外部签名（Support external signing of service account tokens）进阶至GA。此功能支持kube-apiserver通过Unix域套接字对接外部密钥管理系统，将ServiceAccount JWT令牌签名逻辑外置处理。详细介绍请参考[服务账号令牌外部签名](https://github.com/kubernetes/enhancements/blob/master/keps/sig-auth/740-service-account-external-signing/README.md)。
- 可变准入策略（GA）

  在Kubernetes 1.36中，可变准入策略（Mutating Admission Policies）进阶至GA。此功能新增MutatingAdmissionPolicy与MutatingAdmissionPolicyBinding两类 API 资源，基于CEL表达式实现内置式可变准入能力，作为传统可变准入Webhook的轻量替代方案。详细使用方式请参考[可变准入策略](https://github.com/kubernetes/enhancements/blob/master/keps/sig-api-machinery/3962-mutating-admission-policies/README.md)。
- 节点日志查询（GA）

  在Kubernetes 1.36中，节点日志查询（Node log query）进阶至GA。此功能扩展kubelet /proxy/logs接口能力，可以无需登录节点，直接通过 kubectl API远程查询节点的系统服务日志。详细介绍请参考[节点日志查询](https://github.com/kubernetes/enhancements/blob/master/keps/sig-windows/2258-node-log-query/README.md)。
- 用户命名空间支持（GA）

  在Kubernetes 1.36中，用户命名空间支持（User Namespaces Support）进阶至GA。此功能通过pod.spec.hostUsers字段启用用户命名空间隔离，提升Pod安全隔离性。该功能对操作系统内核和运行时有一定要求，建议在CCE将要支持的HCE 3.0操作系统上使用。详细介绍请参考[用户命名空间支持](https://github.com/kubernetes/enhancements/blob/master/keps/sig-node/127-user-namespaces/README.md)。
- PSI压力指标暴露（GA）

  在Kubernetes 1.36中，PSI压力指标暴露（Expose PSI Metrics）进阶至GA。此功能支持kubelet基于cgroupv2从 runc、cAdvisor 原生采集CPU、内存、IO三类PSI资源阻塞压力指标，指标直接暴露在kubelet /stats/summary API和cAdvisor Prometheus监控接口。详细介绍请参考[PSI压力指标暴露](https://github.com/kubernetes/enhancements/blob/master/keps/sig-node/4205-psi-metric/README.md)。
- OCI卷源支持（GA）

  在Kubernetes 1.36中，OCI卷源支持（OCI Volume Source）进阶至GA。此功能为Pod新增image类型卷源，支持直接将OCI镜像/制品挂载为只读卷。使用此功能时，containerd需为v2.1或更高版本。详细使用方式请参考[OCI卷源支持](https://github.com/kubernetes/enhancements/tree/master/keps/sig-node/4639-oci-volume-source)。
- 卷的SELinux标签提速（GA）

  在Kubernetes 1.36中，卷的SELinux标签提速（Speed up SELinux volume relabeling using mounts）进阶至GA。此特性优化SELinux强制模式下存储卷权限处理机制，使用**mount -o context**选项替代了递归文件重标记，在挂载时为整个卷应用正确的SELinux标签。详细介绍请参考[卷的SELinux标签提速](https://github.com/kubernetes/enhancements/blob/master/keps/sig-storage/1710-selinux-relabeling/README.md)。
- IP/CIDR验证改进（Beta）

  在Kubernetes 1.36中，IP/CIDR验证改进（IP/CIDR Validation Improvements）进阶至Beta。该特性收紧Kubernetes原生API中IP与CIDR字段的校验规则。详细介绍请参考[IP/CIDR验证改进](https://github.com/kubernetes/enhancements/blob/master/keps/sig-network/4858-ip-cidr-validation/README.md)。
- 允许更新暂停Job资源，包括CPU、内存、扩展资源（Beta）

  在Kubernetes 1.36中，允许更新暂停Job资源进阶至Beta。此功能允许处于暂停状态（spec.suspend=true）的Job直接修改Pod模板中CPU、内存、GPU及各类扩展资源的requests和limits配置。详细使用方式请参考[允许更新暂停Job资源](https://github.com/kubernetes/enhancements/blob/master/keps/sig-apps/5440-mutable-job-pod-resource-updates/README.md)。
- 受限身份模拟（Beta）

  在Kubernetes 1.36中，受限身份模拟（Constrained Impersonation）进阶至 Beta。此特性对集群原有身份模拟机制进行细粒度权限加固，实现模拟主体身份与目标资源操作双重鉴权校验。详细介绍请参考[受限身份模拟](https://github.com/kubernetes/enhancements/blob/master/keps/sig-auth/5284-constrained-impersonation/README.md)。
- Statusz组件和Flagz组件（Beta）

  在Kubernetes 1.36中，Statusz组件和Flagz组件进阶至Beta。支持通过statusz和flagz端点暴露组件基本信息和生效命令参数信息。详细介绍请参考[组件Statusz](https://github.com/kubernetes/enhancements/blob/master/keps/sig-instrumentation/4827-component-statusz/README.md)和[组件Flagz](https://github.com/kubernetes/enhancements/blob/master/keps/sig-instrumentation/4828-component-flagz/README.md)。
- 混合版本代理（Beta）

  在Kubernetes 1.36中，混合版本代理（Unknown Version Interoperability Proxy）进阶至Beta。此特性支持将本地APIServer无法处理的资源请求自动代理到集群具备对应服务能力的对等APIServer上，规避接口404错误。详细介绍请参考[混合版本代理](https://github.com/kubernetes/enhancements/blob/master/keps/sig-api-machinery/4020-unknown-version-interoperability-proxy/README.md)。
- Service名称校验放开（Beta）

  在Kubernetes 1.36中，Service名称校验放开（Relaxed validation for Services names）进阶至Beta。此特性将Service名称的校验规则从NameIsDNS1035Label改为NameIsDNSLabel，允许Service名称以数字开头。详细介绍请参考[Service名称校验放开](https://github.com/kubernetes/enhancements/blob/master/keps/sig-network/5311-relaxed-validation-for-service-names/README.md)。
- 容器退出触发整Pod容器重启（Beta）

  在Kubernetes 1.36中，容器退出触发整Pod容器重启（Restart All Containers on Container Exits）进阶至Beta。此功能RestartAllContainers重启动作，支持配置容器指定退出码后触发Pod原地全容器重启。详细介绍请参考[容器退出触发整Pod容器重启](https://github.com/kubernetes/enhancements/blob/master/keps/sig-node/5532-restart-all-containers-on-container-exits/README.md)。
- 节点声明特性（Beta）

  在Kubernetes 1.36中，节点声明特性（Node Declared Features）进阶至 Beta。此功能新增node.status.declaredFeatures状态字段，允许kubelet基于自身特性门控、静态配置自动上报本机支持的K8s新能力特性。详细介绍请参考[节点声明特性](https://github.com/kubernetes/enhancements/blob/master/keps/sig-node/5328-node-declared-features/README.md)。
- Pod级资源原地伸缩（Beta）

  在Kubernetes 1.36中，Pod级资源原地伸缩（In-Place Pod-Level Resources Resize）进阶至 Beta。此特性支持原地变更Pod级别的资源配额和限制，仅兼容cgroupv2节点。详细介绍请参考[Pod级资源原地伸缩](https://github.com/kubernetes/enhancements/blob/master/keps/sig-node/5419-pod-level-resources-in-place-resize/README.md)。

#### API变更与弃用

- 在Kubernetes 1.36中，gitRepo卷插件被永久禁用，且无法重新启用。
- 在Kubernetes 1.36中，Service .spec.externalIPs字段被弃用，若仍使用将有告警提示。
- 在Kubernetes 1.36中，etcd监控指标完成重命名，etcd\_bookmark\_counts改为etcd\_bookmark\_total。
- 在Kubernetes 1.36中，调度域上线scheduling.k8s.io/v1alpha2版本Workload与PodGroup API，彻底废弃旧版v1alpha1 API。
- 在Kubernetes 1.36中，Pod级资源原地伸缩特性升至Beta并默认启用，支持直接调整运行中Pod容器资源。
- 在Kubernetes 1.36中，默认开启 StrictIPCIDRValidation 严格 IP 与 CIDR 格式校验，拦截非法格式网络地址。
- 在Kubernetes 1.36中，kube-controller-manager的指标volume\_operation\_total\_errors重命名为volume\_operation\_errors\_total。

#### CCE对Kubernetes 1.36版本的增强

在版本维护周期中，CCE会对Kubernetes 1.36版本进行定期的更新，并提供功能增强。

关于CCE集群版本的更新说明，请参见[补丁版本发布说明](https://support.huaweicloud.com/usermanual-cce/cce_10_0405.html)。

#### 参考链接

关于Kubernetes 1.36与其他版本的性能对比和功能演进的更多信息，请参考：[Kubernetes v1.36 Release Notes](https://github.com/kubernetes/kubernetes/blob/master/CHANGELOG/CHANGELOG-1.36.md)
