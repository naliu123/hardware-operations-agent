# （停止维护）Kubernetes 1.13版本说明

来源：https://support.huaweicloud.com/usermanual-cce/cce_whsnew_0009.html
目录：用户指南 > 集群 > 集群版本发布说明 > Kubernetes版本发布记录 > （停止维护）Kubernetes 1.13版本说明
更新时间：2026-09-20 GMT+08:00

云容器引擎（CCE）严格遵循社区一致性认证。本文介绍CCE发布Kubernetes 1.13版本所做的变更说明。

**表1** v1.13版本集群说明

| Kubernetes版本（CCE增强版） | 版本说明 |
| --- | --- |
| v1.13.10-r0 | **主要特性：**   - CCE集群支持添加ARM节点 - 负载均衡支持设置名称 - 4层负载均衡支持健康检查，7层负载均衡支持健康检查/分配策略/会话保持 - CCE集群支持创建裸金属节点（容器隧道网络） - 支持AI加速型节点（搭载海思Ascend 310 AI处理器），适用于图像识别、视频处理、推理计算以及机器学习等场景 - 支持配置docker baseSize - 支持命名空间亲和调度 - 支持节点数据盘划分用户空间 - 支持集群cpu管理策略 - 支持集群下的节点跨子网（容器隧道网络） |
| v1.13.7-r0 | **主要特性：**   - Kubernetes同步社区1.13.7版本 - 支持网络平面（NetworkAttachmentDefinition） |

#### 参考链接

社区v1.11与v1.13版本之间的CHANGELOG

- v1.12到v1.13的变化：

  <https://github.com/kubernetes/kubernetes/blob/master/CHANGELOG/CHANGELOG-1.13.md>
- v1.11到v1.12的变化：

  <https://github.com/kubernetes/kubernetes/blob/master/CHANGELOG/CHANGELOG-1.12.md>
