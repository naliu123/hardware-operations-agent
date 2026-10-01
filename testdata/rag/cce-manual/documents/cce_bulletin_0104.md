# Kubernetes 1.32版本说明

来源：https://support.huaweicloud.com/usermanual-cce/cce_bulletin_0104.html
目录：用户指南 > 集群 > 集群版本发布说明 > Kubernetes版本发布记录 > Kubernetes 1.32版本说明
更新时间：2025-11-05 GMT+08:00

云容器引擎（CCE）严格遵循社区一致性认证，现已支持Kubernetes 1.32集群版本特性。本文介绍Kubernetes 1.32版本的变更说明。

#### 索引

- [新增特性及特性增强](https://support.huaweicloud.com/usermanual-cce/cce_bulletin_0104.html)
- [API变更与弃用](https://support.huaweicloud.com/usermanual-cce/cce_bulletin_0104.html)
- [CCE对Kubernetes 1.32版本的增强](https://support.huaweicloud.com/usermanual-cce/cce_bulletin_0104.html)
- [参考链接](https://support.huaweicloud.com/usermanual-cce/cce_bulletin_0104.html)

#### 新增特性及特性增强

- SizeMemoryBackedVolumes（GA）

  在Kubernetes 1.32中，SizeMemoryBackedVolumes特性进阶至GA。该特性支持对基于内存的卷（如EmptyDir）指定大小限制，防止Pod过多占用节点内存。详细使用方式请参考[emptyDir](https://kubernetes.io/zh-cn/docs/concepts/storage/volumes/#emptydir)。
- Pod生命周期Sleep（GA）

  在Kubernetes 1.32中，PodLifecycleSleepAction特性进阶至GA。该特性支持在容器生命周期回调中通过Sleep字段直接使用**sleep**命令，将容器暂停一段指定的时间。详细使用方式请参考[容器回调](https://kubernetes.io/zh-cn/docs/concepts/containers/container-lifecycle-hooks/#container-hooks)。
- StatefulSet自动删除PVC（GA）

  在Kubernetes 1.32中，StatefulSetAutoDeletePVC特性进阶至GA。该特性支持设置StatefulSet在删除和缩容时PVC的保留策略，允许在StatefulSet删除或缩容时同步删除PVC。详细使用方式请参考[PersistentVolumeClaim保留](https://kubernetes.io/zh-cn/docs/concepts/workloads/controllers/statefulset/#persistentvolumeclaim-retention)。
- CronJobsScheduledAnnotation（GA）

  在Kubernetes 1.32中，CronJobsScheduledAnnotation特性进阶至GA。该特性会将Cronjob被调度的时间写到Job的注解batch.kubernetes.io/cronjob-scheduled-timestamp中。
- PodIndexLabel（GA）

  在Kubernetes 1.32中，PodIndexLabel特性进阶至GA。该特性会将Index Job的完成顺序写到Pod的标签batch.kubernetes.io/job-completion-index中。
- 定制资源的可选择字段（GA）

  在Kubernetes 1.32中，CustomResourceFieldSelectors特性进阶至GA。该特性支持对CRD配置selectableFields，并支持使用Field Selectors过滤List、Watch和DeleteCollection请求，方便用户定位或管理符合特定条件的CRD资源。详细使用方式请参考[定制资源的可选择字段](https://kubernetes.io/zh-cn/docs/concepts/extend-kubernetes/api-extension/custom-resources/#crd-selectable-fields)。
- JobManagedBy（Beta）

  在Kubernetes 1.32中，JobManagedBy特性进阶至Beta。该特性支持通过Job的spec.managedBy字段自定义Job的controller。详细使用方式请参考[将管理Job对象的任务委托给外部控制器](https://kubernetes.io/zh-cn/docs/concepts/workloads/controllers/job/#delegation-of-managing-a-job-object-to-external-controller)。
- RelaxedEnvironmentVariableValidation（Beta）

  在Kubernetes 1.32中，RelaxedEnvironmentVariableValidation特性进阶至Beta。该特性支持在环境变量名中使用所有可打印的ASCII字符（"="除外），增加了环境变量使用的灵活性。详细使用方式请参考[在配置中使用环境变量](https://kubernetes.io/zh-cn/docs/tasks/inject-data-application/define-environment-variable-container/#using-env-var-inside-of-your-config)。
- WatchList（Beta）

  在Kubernetes 1.32中，WatchList特性进阶至Beta并默认开启。对于使用client-go的用户，在开启client-go的WatchListClient特性后，使用流式请求而不是全量List访问Kube-apiserver，降低控制面资源消耗。

#### API变更与弃用

- 在Kubernetes 1.32中，ServiceAccount 的元数据注解 kubernetes.io/enforce-mountable-secrets 从 v1.32 开始被弃用，但尚未设定移除时间。建议尽快切换使用单独的命名空间来隔离对Secret的访问。
- 在Kubernetes 1.32中，开放了kube-controller-manager的配置参数concurrent-daemonset-syncs，用于设置DaemonSet的并发处理数，可通过配置管理进行设置。

#### CCE对Kubernetes 1.32版本的增强

在版本维护周期中，CCE会对Kubernetes 1.32版本进行定期的更新，并提供功能增强。

关于CCE集群版本的更新说明，请参见[补丁版本发布说明](https://support.huaweicloud.com/usermanual-cce/cce_10_0405.html)。

#### 参考链接

关于Kubernetes 1.32与其他版本的性能对比和功能演进的更多信息，请参考：[Kubernetes v1.32 Release Notes](https://github.com/kubernetes/kubernetes/blob/master/CHANGELOG/CHANGELOG-1.32.md)
