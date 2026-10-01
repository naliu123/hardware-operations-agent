# （停止维护）Kubernetes 1.11版本说明

来源：https://support.huaweicloud.com/usermanual-cce/cce_whsnew_0004.html
目录：用户指南 > 集群 > 集群版本发布说明 > Kubernetes版本发布记录 > （停止维护）Kubernetes 1.11版本说明
更新时间：2026-09-20 GMT+08:00

云容器引擎（CCE）严格遵循社区一致性认证。本文介绍CCE发布Kubernetes 1.11版本所做的变更说明。

**表1** v1.11版本集群说明

| Kubernetes版本（CCE增强版） | 版本说明 |
| --- | --- |
| v1.11.7-r2 | **主要特性：**   - GPU支持V100类型 - 集群支持权限管理 |
| v1.11.7-r0 | **主要特性：**   - Kubernetes同步社区1.11.7版本 - 支持创建节点池（nodepool），虚拟机/鲲鹏ARM集群均支持 - CCE集群支持创建裸金属节点（VPC网络），支持裸金属和虚机混合部署 - GPU支持V100类型 - 1.11集群对接AOM告警通知机制 - Service支持访问类型切换 - 支持服务网段 - 集群支持自定义每个节点分配的IP数（IP分配） |
| v1.11.3-r2 | **主要特性：**   - 集群支持IPv6双栈 - ELB负载均衡支持源IP跟后端服务会话保持 |
| v1.11.3-r1 | **主要特性：**   - Ingress的URL匹配支持Perl语法的正则表达式 |
| v1.11.3-r0 | **主要特性：**   - Kubernetes同步社区1.11.3版本 - 集群控制节点支持多可用区 - 容器存储支持对接SFS Turbo极速文件存储 |

#### 参考链接

社区v1.9与v1.11版本之间的CHANGELOG

- v1.10到v1.11的变化：

  <https://github.com/kubernetes/kubernetes/blob/master/CHANGELOG/CHANGELOG-1.11.md>
- v1.9到v1.10的变化：

  <https://github.com/kubernetes/kubernetes/blob/master/CHANGELOG/CHANGELOG-1.10.md>
