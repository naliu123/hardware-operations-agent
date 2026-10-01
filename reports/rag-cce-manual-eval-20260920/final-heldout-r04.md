# final-heldout-r04

模式：LIVE_ANSWERS

```json
{
  "completed": 36,
  "expected": 36,
  "errors": 0,
  "service_failures": 2,
  "answer_accuracy": 0.9444444444444444,
  "llm_judged_answer_accuracy": 0.9166666666666666,
  "evidence_recall_at_5": 0.9333333333333333,
  "all_evidence_at_5": 0.9333333333333333,
  "document_hit_at_5": 0.9333333333333333,
  "latency_p95_ms": 49957.28704200155,
  "generation_usage": {
    "calls": 70,
    "prompt_tokens": 880981,
    "completion_tokens": 33420,
    "total_tokens": 914401
  },
  "citation_integrity_rate": 1.0,
  "evidence_selection": {
    "responses": 36,
    "candidate_fragments_total": 1481,
    "expanded_fragments_total": 239,
    "input_bytes_total": 2707049,
    "final_context_bytes_max": 24022
  },
  "llm_judged_passed": 33,
  "wilson_95": [
    0.7817298972308114,
    0.9712519107287082
  ],
  "audit_status": "completed",
  "real_model_calls": 70,
  "llm_judged_answerable_accuracy": 0.9333333333333333,
  "llm_judged_abstention_accuracy": 0.8333333333333334,
  "audit_required_ids": [
    "cce-manual-020",
    "cce-manual-021",
    "cce-negative-runtime-pods",
    "cce-manual-027",
    "cce-manual-056",
    "cce-manual-060",
    "cce-manual-061",
    "cce-manual-014",
    "cce-manual-075",
    "cce-manual-037"
  ],
  "judge_usage": {
    "prompt_tokens": 166763,
    "completion_tokens": 6224,
    "total_tokens": 172987
  },
  "audited_passed": 34,
  "audited_count": 18,
  "audited_wilson_95": [
    0.8185502589238878,
    0.9846303362329332
  ],
  "answerable_accuracy": 0.9333333333333333,
  "abstention_accuracy": 1.0
}
```

准确率仅在真实回答、语义评审和证据复核完成后报告；检索指标不代表答案正确。

## Bad cases（2）

### cce-manual-020 · 调度

应用对CPU上下文切换和跨Socket内存访问敏感，CCE文档建议用什么策略改善？

参考要点：使用CPU管理策略分配独占CPU核/绑核；减少调度延迟；CPU manager优先同Socket和完整物理核

初步排查方向：未命中全部参考锚点，核对等价证据、分段及候选排序

实际回答：

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0351.html

召回章节：

评审：答案为空，且状态为FAILED（服务错误），未提供任何内容，也未指出具体缺口，因此不满足任何必答点，不属于正确拒答。

证据复核（以绑定当前答案的复核为准）：外部模型调用失败，结果为FAILED/MODEL_UNAVAILABLE，无答案和引用。服务失败按固定验收口径直接计失败，不重试、不使用参考答案替代。

### cce-manual-021 · 调度

enhanced-static相比static新增支持哪种Burstable Pod，CPU requests和limits需满足什么条件？

参考要点：兼容static绑核能力；新增符合条件的Burstable Pod优先使用CPU；requests和limits均为正整数；不应凭空要求两者必须相等

初步排查方向：未命中全部参考锚点，核对等价证据、分段及候选排序

实际回答：

具体缺口：

证据来源：https://support.huaweicloud.com/usermanual-cce/cce_10_0552.html

召回章节：

评审：答案为空，未提供任何内容，无法满足任何必答点，也未直接回答问题。状态为FAILED属于服务错误，不是正确拒答。

证据复核（以绑定当前答案的复核为准）：证据选择完成但生成输出未通过校验，结果为FAILED/INVALID_MODEL_OUTPUT，无答案和引用。按固定验收口径直接计失败。
