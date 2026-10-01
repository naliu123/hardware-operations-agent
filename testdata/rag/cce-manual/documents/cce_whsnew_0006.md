# （停止维护）Kubernetes 1.15版本说明

来源：https://support.huaweicloud.com/usermanual-cce/cce_whsnew_0006.html
目录：用户指南 > 集群 > 集群版本发布说明 > Kubernetes版本发布记录 > （停止维护）Kubernetes 1.15版本说明
更新时间：2025-04-25 GMT+08:00

云容器引擎（CCE）严格遵循社区一致性认证。本文介绍CCE发布Kubernetes 1.15版本所做的变更说明。

为了能够更好地方便您使用容器服务，确保您使用稳定又可靠的Kubernetes版本，请您务必在维护周期结束之前升级您的Kubernetes集群。

#### 版本说明

CCE针对Kubernetes v1.15版本提供了全链路的组件优化和升级，v1.15版本包含两个小版本，即v1.15.11和v1.15.6-r1。

#### 资源变更与弃用

- extensions/v1beta1中Ingress已弃用，1.19正式暂停使用，迁移到networking.k8s.io/v1beta1
- extensions/v1beta1中NetworkPolicy 1.16正式暂停使用，迁移到networking.k8s.io/v1
- extensions/v1beta1中PodSecurityPolicy 1.16正式暂停使用，迁移到policy/v1beta1
- extensions/v1beta1、apps/v1beta1或apps/v1beta2的DaemonSet、Deployment、和ReplicaSet，迁移至apps/v1，1.16版本暂停使用
- PriorityClass升级到scheduling.k8s.io/v1，scheduling.k8s.io/v1beta1和scheduling.k8s.io/v1alpha1 1.17正式废弃

- events.k8s.io/v1beta1 Event API中series.state字段废弃，将在1.18版本中移除

#### 参考链接

社区v1.13与v1.15版本之间的CHANGELOG

- v1.14到v1.15的变化：

  <https://github.com/kubernetes/kubernetes/blob/master/CHANGELOG/CHANGELOG-1.15.md>
- v1.13到v1.14的变化：

  <https://github.com/kubernetes/kubernetes/blob/master/CHANGELOG/CHANGELOG-1.14.md>
