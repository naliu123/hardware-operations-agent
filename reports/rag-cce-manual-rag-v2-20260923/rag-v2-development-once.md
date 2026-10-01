# rag-v2-development-once

模式：REAL_RETRIEVAL_ONLY

```json
{
  "completed": 75,
  "expected": 75,
  "errors": 7,
  "service_failures": 0,
  "answer_accuracy": null,
  "llm_judged_answer_accuracy": null,
  "evidence_recall_at_5": 0.8623188405797102,
  "all_evidence_at_5": 0.8405797101449275,
  "document_hit_at_5": 0.8405797101449275,
  "latency_p95_ms": 5748.156749999907,
  "generation_usage": {
    "calls": 0,
    "prompt_tokens": 0,
    "completion_tokens": 0,
    "total_tokens": 0
  },
  "citation_integrity_rate": null
}
```

准确率仅在真实回答、语义评审和证据复核完成后报告；检索指标不代表答案正确。

## Bad cases（12）

### cce-manual-012 · 节点池

节点池默认规格A为0.0993USD/小时，但手动新增10台规格B为0.1923USD/小时，新增节点一小时按哪个价格算，总额多少？

参考要点：以实际创建的B规格计费；新增10台一小时为1.923USD；控制台展示默认A价格不改变实际计费

初步排查方向：未命中全部参考锚点，核对等价证据、分段及候选排序

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0648.html

召回章节：节点池多规格计费说明/计费样例、创建节点/操作步骤、创建节点/操作步骤、更新节点池/更新节点池、更新节点池/更新节点池

### cce-manual-018 · 工作负载

我想在集群每个节点部署日志采集器和节点监控，应选择哪类工作负载？

参考要点：DaemonSet守护进程集；日志采集进程和节点监控进程适合逐节点部署

初步排查方向：未命中全部参考锚点，核对等价证据、分段及候选排序

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0216.html

召回章节：备份中心常见问题/索引、备份中心常见问题/备份/恢复记录长时间处于删除中、插件检查异常处理/解决方案、工作负载监控/工作负载监控 / 功能入口、日志中心FAQ/索引

### cce-manual-034 · 弹性伸缩

在CCE使用HPA前，监控插件需要提供什么API？

参考要点：需要安装能够提供Metrics API的插件；按集群版本和需求选择

初步排查方向：未命中全部参考锚点，核对等价证据、分段及候选排序

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0208.html

召回章节：备份中心常见问题/索引、备份中心常见问题/备份/恢复记录长时间处于删除中、插件检查异常处理/解决方案、cce-hpa-controller插件限制检查异常处理/检查项内容 / 解决方案、日志中心FAQ/索引

### cce-manual-040 · 云原生观测

CCE云原生可观测性从下到上分哪四层？

参考要点：算力底座；数据采集；监控与日志；云原生观测

初步排查方向：服务/依赖错误

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0110.html

召回章节：

### cce-manual-041 · 云原生观测

CCE云原生观测授权的细粒度改造，是从什么粒度改为什么粒度？

参考要点：从系统策略粒度权限集改为Action粒度；Action对应依赖调用的接口

初步排查方向：服务/依赖错误

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0953.html

召回章节：

### cce-manual-050 · AI容器

CCE推理负载基于什么控制器和推理引擎，多角色分布式推理有哪些角色？

参考要点：Kthena ModelServing；vLLM；Entry和Worker

初步排查方向：服务/依赖错误

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_11511.html

召回章节：

### cce-manual-055 · 命名空间

想让开发、联调、测试环境共享同一CCE集群但逻辑隔离，可用什么方式组织？

参考要点：为不同环境建立对应命名空间；创建和查询工作负载时选择对应命名空间；不要宣称这等同于完整网络安全隔离

初步排查方向：未命中全部参考锚点，核对等价证据、分段及候选排序

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0285.html

召回章节：管理命名空间/命名空间使用实践、创建命名空间/操作场景、使用FlexNPU实现NPU资源虚拟化与隔离/前提条件、开启云原生混部/云原生混部配置、管理集群概述/集群配置与资源管理

### cce-manual-064 · 插件

集群Pod因工作节点资源不足而调度失败，CCE集群弹性引擎会怎样处理，利用率低时呢？

参考要点：基于Autoscaler扩容新节点；扩容节点资源利用率很低时自动删除节点；不保证其他原因导致的Pending都能靠扩容解决

初步排查方向：服务/依赖错误

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0154.html

召回章节：

### cce-manual-067 · 模板（Helm Chart）

同一Helm模板上传多个版本只算一个模板配额吗？模板包命名规则是什么？

参考要点：每个版本消耗对应模板配额；name-version.tgz；version为主版本号.次版本号.修订号

初步排查方向：未命中全部参考锚点，核对等价证据、分段及候选排序

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0146.html

召回章节：通过模板部署应用/约束与限制、通过Helm v3客户端部署应用/安装Helm模板包、模板概述/模板概述、通过Helm v3客户端部署应用/安装Helm模板包、通过模板部署应用/上传模板

### cce-manual-088 · 备份中心

Velero仓库提示invalid top-level directories，OBS桶根目录只允许哪些目录？

参考要点：只允许restores和backups；清理根目录其他文件或文件夹后可恢复；不声称已执行删除

初步排查方向：服务/依赖错误

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_1164.html

召回章节：

### cce-manual-091 · 弹性伸缩

metrics.k8s.io、custom.metrics.k8s.io和external.metrics.k8s.io分别提供什么指标？

参考要点：metrics提供Pod/Node的CPU内存；custom提供Kubernetes对象关联的自定义指标；external来自外部、不关联Kubernetes资源

初步排查方向：服务/依赖错误

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0290.html

召回章节：

### cce-negative-unprovided-cve · 集群

手册里CVE-2099-987654漏洞对应的CCE修复小版本是什么？

参考要点：没有该虚构漏洞及修复版本依据，不能编造版本。

初步排查方向：服务/依赖错误

实际回答：未运行回答

具体缺口：

证据来源：

召回章节：
