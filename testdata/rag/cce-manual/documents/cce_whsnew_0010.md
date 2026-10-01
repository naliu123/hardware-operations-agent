# （停止维护）Kubernetes 1.19版本说明

来源：https://support.huaweicloud.com/usermanual-cce/cce_whsnew_0010.html
目录：用户指南 > 集群 > 集群版本发布说明 > Kubernetes版本发布记录 > （停止维护）Kubernetes 1.19版本说明
更新时间：2024-08-17 GMT+08:00

云容器引擎（CCE）严格遵循社区一致性认证。本文介绍CCE发布Kubernetes 1.19版本所做的变更说明。

#### 资源变更与弃用

**社区1.19 ReleaseNotes**

- 增加对vSphere in-tree卷迁移至vSphere CSI驱动的支持。in-tree vSphere Volume插件将不再使用，并在将来的版本中删除。
- apiextensions.k8s.io/v1beta1已弃用，推荐使用apiextensions.k8s.io/v1。
- apiregistration.k8s.io/v1beta1已弃用，推荐使用apiregistration.k8s.io/v1。
- authentication.k8s.io/v1beta1、authorization.k8s.io/v1beta1已弃用，1.22将移除，推荐使用authentication.k8s.io/v1、authorization.k8s.io/v1。
- autoscaling/v2beta1已弃用，推荐使用autoscaling/v2beta2。
- coordination.k8s.io/v1beta1在1.19中已弃用，1.22将移除，推荐使用v1。
- Kube-apiserver: componentstatus API已弃用。
- Kubeadm：kubeadm config view命令已被弃用，并将在未来版本中删除，请使用kubectl get cm -o yaml -n kube-system kubeadm-config来直接获取kubeadm配置。
- Kubeadm：弃用kubeadm alpha kubelet config enable-dynamic命令。
- Kubeadm：kubeadm alpha certs renew命令--use-api参数已弃用。
- Kubernetes不再支持构建hyperkube镜像。
- Remove --export flag from kubectl get command - kubectl get中移除 --export参数。
- alpha特性“ResourceLimitsPriorityFunction”已完全删除。
- storage.k8s.io/v1beta1已弃用，推荐使用storage.k8s.io/v1。

**社区1.18 ReleaseNotes**

- kube-apiserver
  - apps/v1beta1 and apps/v1beta2下所有资源不再提供服务，使用apps/v1替代。
  - extensions/v1beta1下daemonsets，deployments，replicasets不再提供服务，使用apps/v1替代。
  - extensions/v1beta1下networkpolicies不再提供服务，使用networking.k8s.io/v1替代。
  - extensions/v1beta1下podsecuritypolicies不再提供服务，使用policy/v1beta1替代。

- kubelet
  - --redirect-container-streaming不推荐使用，v1.20会正式废弃。
  - 资源度量端点 /metrics/resource/v1alpha1以及此端点下的所有度量标准均已弃用。请转换为端点 /metrics/resource下的度量标准：
    - scrape\_error --> scrape\_error
    - node\_cpu\_usage\_seconds\_total --> node\_cpu\_usage\_seconds
    - node\_memory\_working\_set\_bytes --> node\_memory\_working\_set\_bytes
    - container\_cpu\_usage\_seconds\_total --> container\_cpu\_usage\_seconds
    - container\_memory\_working\_set\_bytes --> container\_memory\_working\_set\_bytes
    - scrape\_error --> scrape\_error
  - 在将来的发行版中，kubelet将不再根据CSI规范创建CSI NodePublishVolume目标目录。可能需要相应地更新CSI驱动程序，以正确创建和处理目标路径。

- kube-proxy
  - --healthz-port和--metrics-port参数不建议使用，请使用--healthz-bind-address和--metrics-bind-address。
  - 增加EndpointSliceProxying功能选项以控制kube-proxy中EndpointSlices的使用，默认情况下已禁用此功能。

- kubeadm
  - kubeadm upgrade node的--kubelet-version参数已弃用，将在后续版本中删除。
  - kubeadm alpha certs renew命令中--use-api参数已弃用。
  - kube-dns已弃用，在将来的版本中将不再受支持。
  - kubeadm-config ConfigMap中存在的ClusterStatus结构体已废弃，将在后续版本中删除。

- kubectl
  - --dry-run不建议使用boolean和unset values，新版本中server|client|none会被使用。
  - kubectl apply --server-dry-run已弃用，替换为--dry-run=server。

- add-ons

删除cluster-monitoring插件。

- kube-scheduler
  - scheduling\_duration\_seconds指标已弃用。
  - scheduling\_algorithm\_predicate\_evaluation\_seconds和scheduling\_algorithm\_priority\_evaluation\_seconds指标已弃用，使用framework\_extension\_point\_duration\_seconds[extension\_point="Filter"]和framework\_extension\_point\_duration\_seconds[extension\_point="Score"]替代。
  - 调度器策略AlwaysCheckAllPredicates已弃用。

- 其他变化
  - k8s.io/node-api组件不再更新。作为替代，可以使用位于k8s.io/api中的RuntimeClass类型和位于k8s.io/client-go中的generated clients。
  - 已从apiserver\_request\_total中删除“client”标签。

#### 参考链接

关于Kubernetes 1.19与其他版本的性能对比和功能演进的更多信息，请参考：

- [Kubernetes v1.19.0 Release Notes](https://github.com/kubernetes/kubernetes/blob/master/CHANGELOG/CHANGELOG-1.19.md#changes)
- [Kubernetes v1.18.0 Release Notes](https://github.com/kubernetes/kubernetes/blob/master/CHANGELOG/CHANGELOG-1.18.md#changes)
