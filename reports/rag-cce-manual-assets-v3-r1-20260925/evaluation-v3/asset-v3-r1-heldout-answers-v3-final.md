# asset-v3-r1-heldout-answers-v3-final

模式：LIVE_ANSWERS

```json
{
  "completed": 20,
  "expected": 20,
  "errors": 0,
  "service_failures": 0,
  "answer_accuracy": 1.0,
  "llm_judged_answer_accuracy": 1.0,
  "evidence_recall_at_5": 0.95,
  "all_evidence_at_5": 0.95,
  "document_hit_at_5": 0.95,
  "latency_p95_ms": 15313.808916980634,
  "generation_usage": {
    "calls": 20,
    "prompt_tokens": 176726,
    "completion_tokens": 4727,
    "total_tokens": 181453
  },
  "citation_integrity_rate": 1.0,
  "llm_judged_passed": 20,
  "wilson_95": [
    0.8388698745050667,
    1.0
  ],
  "audit_status": "completed",
  "real_model_calls": 20,
  "llm_judged_answerable_accuracy": 1.0,
  "llm_judged_abstention_accuracy": null,
  "audit_required_ids": [
    "cce-asset-image-058",
    "cce-asset-image-035",
    "cce-asset-table-explanatory-015",
    "cce-asset-table-explanatory-020"
  ],
  "judge_usage": {
    "prompt_tokens": 246210,
    "completion_tokens": 2573,
    "total_tokens": 248783
  },
  "audited_passed": 20,
  "audited_count": 4,
  "audited_wilson_95": [
    0.8388698745050667,
    1.0
  ],
  "answerable_accuracy": 1.0,
  "abstention_accuracy": null
}
```

准确率仅在真实回答、语义评审和证据复核完成后报告；检索指标不代表答案正确。

## Bad cases（0）
