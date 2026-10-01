# asset-v3-dev-r01-no-rewrite

模式：REAL_RETRIEVAL_ONLY

```json
{
  "completed": 40,
  "expected": 40,
  "errors": 0,
  "service_failures": 0,
  "answer_accuracy": null,
  "llm_judged_answer_accuracy": null,
  "evidence_recall_at_5": 0.975,
  "all_evidence_at_5": 0.975,
  "document_hit_at_5": 0.975,
  "latency_p95_ms": 961.6499170078896,
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

## Bad cases（1）

### cce-asset-image-032 · IMAGE

在“负载监听器配置”表单的“更多配置”区域中，安全策略下拉框当前选中的具体策略名称是什么？

参考要点：安全策略下拉框选择“安全策略 tls-1-2”。

初步排查方向：未命中全部参考锚点，核对等价证据、分段及候选排序

实际回答：未运行回答

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0842.html

召回章节：为负载均衡类型的Service配置服务器名称指示（SNI）/步骤二：创建负载均衡并配置SNI、为负载均衡类型的Service配置黑名单/白名单访问策略/创建负载均衡并配置黑名单/白名单访问策略、ELB监听器访问控制配置项检查异常处理/解决方案、为负载均衡类型的Service配置HTTP/HTTPS协议/步骤二：创建负载均衡并配置HTTP/HTTPS协议、为负载均衡类型的Service配置QUIC监听器/步骤二：创建负载均衡并配置QUIC协议
