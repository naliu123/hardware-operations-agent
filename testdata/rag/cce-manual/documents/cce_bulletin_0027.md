# （停止维护）Kubernetes 1.23版本说明

来源：https://support.huaweicloud.com/usermanual-cce/cce_bulletin_0027.html
目录：用户指南 > 集群 > 集群版本发布说明 > Kubernetes版本发布记录 > （停止维护）Kubernetes 1.23版本说明
更新时间：2025-07-18 GMT+08:00

云容器引擎（CCE）严格遵循社区一致性认证。本文介绍CCE发布Kubernetes 1.23版本所做的变更说明。

#### 资源变更与弃用

**社区1.23 ReleaseNotes**

- FlexVolume废弃，建议使用CSI。
- HorizontalPodAutoscaler v2版本GA，HorizontalPodAutoscaler API v2在1.23版本中逐渐稳定。不建议使用HorizontalPodAutoscaler v2beta2 API，建议使用新的v2版本API。
- [PodSecurity](https://kubernetes.io/docs/concepts/security/pod-security-admission/)支持beta，PodSecurity替代废弃的PodSecurityPolicy，PodSecurity是一个准入控制器，它根据设置实施级别的特定命名空间标签在命名空间中的Pod上实施Pod安全标准。在1.23中PodSecurity默认启用。

**社区1.22 ReleaseNotes**

- Ingress资源不再支持networking.k8s.io/v1beta1和extensions/v1beta1 API。如果使用旧版本API管理Ingress，会影响应用对外暴露服务，请尽快使用networking.k8s.io/v1替代。
- CustomResourceDefinition资源不再支持apiextensions.k8s.io/v1beta1 API。如果使用旧版本API创建自定义资源定义，会导致定义创建失败，进而影响调和（reconcile）该自定资源的控制器，请尽快使用apiextensions.k8s.io/v1替代。
- ClusterRole、ClusterRoleBinding、Role和RoleBinding资源不再支持rbac.authorization.k8s.io/v1beta1 API。如果使用旧版本API管理RBAC资源，会影响应用的权限服务，甚至无法在集群内正常使用，请尽快使用rbac.authorization.k8s.io/v1替代。
- Kubernetes版本发布周期由一年4个版本变为一年3个版本。
- StatefulSets 支持minReadySeconds。
- 缩容时默认根据Pod uid排序随机选择删除Pod（LogarithmicScaleDown）。基于该特性，可以增强Pod被缩容的随机性，缓解由于Pod拓扑分布约束带来的问题。更多信息，请参见[KEP-2185](https://github.com/kubernetes/enhancements/tree/master/keps/sig-apps/2185-random-pod-select-on-replicaset-downscale)和[issues 96748](https://github.com/kubernetes/kubernetes/issues/96748)。
- [BoundServiceAccountTokenVolume](https://kubernetes.io/zh-cn/docs/reference/access-authn-authz/service-accounts-admin/#bound-service-account-token-volume)特性已稳定，该特性能够提升服务账号（ServiceAccount）Token的安全性，改变了Pod挂载Token的方式，Kubernetes 1.21及以上版本的集群中会默认开启。

#### 非兼容变更

Kubernetes v1.23中更改kube-proxy启动的行为：当节点的内核参数值设置得比kube-proxy默认值更高时，kube-proxy不再尝试设置特定的内核参数值。如节点nf\_conntrack\_max内核参数值设置为1000000，而kube-proxy计算的值为131072，那么此内核参数会使用节点默认值1000000。

社区PR链接为：<https://github.com/kubernetes/kubernetes/pull/103174>

#### 参考链接

关于Kubernetes 1.23与其他版本的性能对比和功能演进的更多信息，请参考：

- [Kubernetes v1.23 Release Notes](https://github.com/kubernetes/kubernetes/blob/master/CHANGELOG/CHANGELOG-1.23.md)
- [Kubernetes v1.22 Release Notes](https://github.com/kubernetes/kubernetes/blob/master/CHANGELOG/CHANGELOG-1.22.md)
