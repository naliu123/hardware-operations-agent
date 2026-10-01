# Kubernetes 1.31版本说明

来源：https://support.huaweicloud.com/usermanual-cce/cce_bulletin_0099.html
目录：用户指南 > 集群 > 集群版本发布说明 > Kubernetes版本发布记录 > Kubernetes 1.31版本说明
更新时间：2025-11-05 GMT+08:00

云容器引擎（CCE）严格遵循社区一致性认证，现已支持Kubernetes 1.31集群版本特性。本文介绍Kubernetes 1.31版本的变更说明。

#### 索引

- [新增特性及特性增强](https://support.huaweicloud.com/usermanual-cce/cce_bulletin_0099.html)
- [API变更与弃用](https://support.huaweicloud.com/usermanual-cce/cce_bulletin_0099.html)
- [CCE对Kubernetes 1.31版本的增强](https://support.huaweicloud.com/usermanual-cce/cce_bulletin_0099.html)
- [参考链接](https://support.huaweicloud.com/usermanual-cce/cce_bulletin_0099.html)

#### 新增特性及特性增强

- StatefulSet起始序号（GA）

  在Kubernetes 1.31中，StatefulSetStartOrdinal特性进阶至GA。默认情况下，StatefulSet中Pod的序号是从0开始，该特性引入后允许用户自定义Pod的起始序号。详细使用方式请参考[起始序号](https://kubernetes.io/zh-cn/docs/concepts/workloads/controllers/statefulset/#start-ordinal)。
- 弹性索引Job（GA）

  在Kubernetes 1.31中，ElasticIndexedJob特性进阶至GA。该特性允许用户在索引Job创建后修改其.spec.completions和.spec.parallelism字段，使之具备弹性伸缩能力。详细使用方式请参考[弹性索引Job](https://kubernetes.io/zh-cn/docs/concepts/workloads/controllers/job/#elastic-indexed-jobs)。
- Pod失效策略（GA）

  在Kubernetes 1.31中，JobPodFailurePolicy特性进阶至GA。该特性允许用户根据Pod失效的原因来分别指定处理方式（重试或者忽略），以优化避免不必要的Pod重启带来的运行成本。详细使用方式请参考[Pod失效策略](https://kubernetes.io/zh-cn/docs/concepts/workloads/controllers/job/#pod-failure-policy)。
- Pod干扰状况（GA）

  在Kubernetes 1.31中，PodDisruptionConditions特性进阶至GA。该特性在Pod的Condition中新增了DisruptionTarget类型，表示Pod失效的原因，例如被高优先级的Pod抢占、因节点删除而被清理、被kubelet终止等。若Pod是Job或者CronJob控制器创建的，可以与[Pod失效策略](https://kubernetes.io/zh-cn/docs/concepts/workloads/controllers/job/#pod-failure-policy)一起使用，通过该Condition定义失效时的行为。更多关于本特性的详细信息请参考[Pod干扰状况](https://kubernetes.io/zh-cn/docs/concepts/workloads/pods/disruptions/#pod-disruption-conditions)。
- 定制资源的可选择字段（Beta）

  在Kubernetes 1.31中，CustomResourceFieldSelectors特性进阶至Beta。该特性支持对CRD配置selectableFields，并支持使用Field Selectors过滤List、Watch和DeleteCollection请求，方便用户定位或管理符合特定条件的CRD资源。详细使用方式请参考[定制资源的可选择字段](https://kubernetes.io/zh-cn/docs/concepts/extend-kubernetes/api-extension/custom-resources/#crd-selectable-fields)。
- Job成功策略（Beta）

  在Kubernetes 1.31中，JobSuccessPolicy特性进阶至Beta。该特性允许用户基于成功的Pod个数为Job配置成功策略。详细使用方式请参考[成功策略](https://kubernetes.io/zh-cn/docs/concepts/workloads/controllers/job/#success-policy)。
- podAffinity中的matchLabelKeys（Beta）

  在Kubernetes 1.31中，MatchLabelKeysInPodAffinity特性进阶至Beta。该特性在podAffinity和podAntiAffinity中引入了更为精细的配置字段matchLabelKeys和mismatchLabelKeys，以解决调度器在Deployment滚动更新期间无法区分新老Pod，继而导致调度结果不符合亲和性和反亲和性预期的问题。详细使用方式请参考[matchLabelKeys](https://kubernetes.io/zh-cn/docs/concepts/scheduling-eviction/assign-pod-node/#matchlabelkeys)。
- ServiceAccountTokenNodeBinding（Beta）

  在Kubernetes 1.31中，ServiceAccountTokenNodeBinding特性进阶至Beta。该特性支持创建绑定到节点的ServiceAccount Token：在Token中包含节点信息的声明，并在使用Token时验证节点的存在，若节点被删除，则Token将会失效。详细使用方式请参考[手动为 ServiceAccount 创建 API 令牌](https://kubernetes.io/zh-cn/docs/tasks/configure-pod-container/configure-service-account/#manually-create-an-api-token-for-a-serviceaccount)。

#### API变更与弃用

- 在Kubernetes 1.31版本中，在没有**--**分隔符的情况下将无法运行**kubectl exec [POD] [COMMAND]**命令，您需要使用**kubectl exec [POD] -- [COMMAND]**命令。
- 在Kubernetes 1.31版本中，在CustomResourceDefinition（CRD）中指定caBundle字段时，如果caBundle非空，但内容无效或不包含任何CA证书，那么该CRD将不会提供服务。CRD的caBundle设置为有效状态后，将不再允许通过更新操作将其变为无效或内容为空的状态（直接更新将报错invalid field value），以避免中断CRD的正常服务。

#### CCE对Kubernetes 1.31版本的增强

在版本维护周期中，CCE会对Kubernetes 1.31版本进行定期的更新，并提供功能增强。

关于CCE集群版本的更新说明，请参见[补丁版本发布说明](https://support.huaweicloud.com/usermanual-cce/cce_10_0405.html)。

#### 参考链接

关于Kubernetes 1.31与其他版本的性能对比和功能演进的更多信息，请参考：[Kubernetes v1.31 Release Notes](https://github.com/kubernetes/kubernetes/blob/master/CHANGELOG/CHANGELOG-1.31.md)
