# Kubernetes 1.30版本说明

来源：https://support.huaweicloud.com/usermanual-cce/cce_bulletin_0095.html
目录：用户指南 > 集群 > 集群版本发布说明 > Kubernetes版本发布记录 > Kubernetes 1.30版本说明
更新时间：2026-01-28 GMT+08:00

云容器引擎（CCE）严格遵循社区一致性认证，现已支持Kubernetes 1.30集群版本特性。本文介绍Kubernetes 1.30版本的变更说明。

#### 索引

- [新增特性及特性增强](https://support.huaweicloud.com/usermanual-cce/cce_bulletin_0095.html)
- [API变更与弃用](https://support.huaweicloud.com/usermanual-cce/cce_bulletin_0095.html)
- [CCE对Kubernetes 1.30版本的增强](https://support.huaweicloud.com/usermanual-cce/cce_bulletin_0095.html)
- [参考链接](https://support.huaweicloud.com/usermanual-cce/cce_bulletin_0095.html)

#### 新增特性及特性增强

- Webhook匹配表达式（GA）

  在Kubernetes1.30版本中，Webhook匹配表达式特性进阶至GA。此特性允许对准入Webhook支持根据特定的条件进行匹配，更细粒度地控制Webhook的触发条件。详细使用方式请参考[动态准入控制](https://kubernetes.io/zh-cn/docs/reference/access-authn-authz/extensible-admission-controllers/#matching-requests-matchConditions)。
- Pod调度就绪态（GA）

  在Kubernetes1.30版本中，Pod调度就绪态特性进阶至GA。此特性允许对Pod添加自定义的schedulingGates，并由用户控制何时移除这些gate，当所有gates移除后，Pod才会被认为调度就绪。详细使用方式请参考[Pod调度就绪态](https://kubernetes.io/zh-cn/docs/concepts/scheduling-eviction/pod-scheduling-readiness/)。
- 验证准入策略（GA）

  在Kubernetes1.30版本中，验证准入策略（ValidatingAdmissionPolicy）特性进阶至GA。该特性支持通过CEL表达式声明资源的验证准入策略。详细使用方式请参考[验证准入策略](https://kubernetes.io/zh-cn/docs/reference/access-authn-authz/validating-admission-policy/)。
- 基于ContainerResource指标的Pod水平自动扩缩容（GA）

  在Kubernetes1.30版本中，基于ContainerResource指标的Pod水平自动扩缩容特性进阶至GA。该特性允许HPA根据Pod中各个容器的资源使用情况来配置自动伸缩，而不仅是Pod的整体资源使用情况，便于为Pod中最重要的容器配置扩缩容阈值。详细使用方式请参考[容器资源指标](https://kubernetes.io/zh-cn/docs/concepts/workloads/autoscaling/horizontal-pod-autoscale/#container-resource-metrics)。
- 传统ServiceAccount令牌清理器（GA）

  在Kubernetes1.30版本中，传统ServiceAccount令牌清理器特性进阶至GA。其作为kube-controller-manager的一部分运行，每24小时检查一次，查看是否有任何自动生成的传统ServiceAccount令牌在特定时间段内（默认为一年，通过--legacy-service-account-token-clean-up-period指定）未被使用。如果有的话，清理器会将这些令牌标记为无效，并添加kubernetes.io/legacy-token-invalid-since标签，其值为当前日期。如果一个无效的令牌在特定时间段（默认为1年，通过--legacy-service-account-token-clean-up-period指定）内未被使用，清理器将会删除它。更多使用细节请参考[传统ServiceAccount令牌清理器](https://kubernetes.io/zh-cn/docs/reference/access-authn-authz/service-accounts-admin/#legacy-serviceaccount-token-cleaner)。
- Pod拓扑分布中的最小域（GA）

  在Kubernetes1.30版本中，Pod拓扑分布中的最小域特性进阶至GA。此特性允许通过Pod的minDomains字段配置符合条件的域的最小数量。负载拓扑约束匹配到的域的数量如果大于minDomains，则该字段没有影响；如果小于minDomains，则会将全局最小值（符合条件的域中匹配 Pod 的最小数量）设为0，该字段必须结合whenUnsatisfiable: DoNotSchedule一起使用，实现在不满足拓扑约束的情况下让Pod不进行调度。详细使用方式参考[Pod拓扑分布](https://kubernetes.io/zh-cn/docs/concepts/scheduling-eviction/topology-spread-constraints/#spread-constraint-definition)。

#### API变更与弃用

- 在Kubernetes1.30版本中，kubectl移除了apply命令的prune-whitelist参数，使用prune-allowlist替代。
- 在Kubernetes1.30版本中，移除了在1.27版本已废弃的准入插件SecurityContextDeny，使用[Pod安全性准入插件](https://kubernetes.io/zh-cn/docs/concepts/security/pod-security-admission/)（PodSecurity）替代。

#### CCE对Kubernetes 1.30版本的增强

在版本维护周期中，CCE会对Kubernetes 1.30版本进行定期的更新，并提供功能增强。

关于CCE集群版本的更新说明，请参见[补丁版本发布说明](https://support.huaweicloud.com/usermanual-cce/cce_10_0405.html)。

#### 参考链接

关于Kubernetes 1.30与其他版本的性能对比和功能演进的更多信息，请参考：[Kubernetes v1.30 Release Notes](https://github.com/kubernetes/kubernetes/blob/master/CHANGELOG/CHANGELOG-1.30.md)
