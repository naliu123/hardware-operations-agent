# asset-v3-r1-dev-answers-v3-r01

模式：LIVE_ANSWERS

```json
{
  "completed": 40,
  "expected": 40,
  "errors": 0,
  "service_failures": 0,
  "answer_accuracy": 1.0,
  "llm_judged_answer_accuracy": 1.0,
  "evidence_recall_at_5": 1.0,
  "all_evidence_at_5": 1.0,
  "document_hit_at_5": 1.0,
  "latency_p95_ms": 13381.717999989633,
  "generation_usage": {
    "calls": 40,
    "prompt_tokens": 354885,
    "completion_tokens": 9564,
    "total_tokens": 364449
  },
  "citation_integrity_rate": 1.0,
  "llm_judged_passed": 40,
  "wilson_95": [
    0.9123754607496077,
    1.0
  ],
  "audit_status": "completed",
  "real_model_calls": 40,
  "llm_judged_answerable_accuracy": 1.0,
  "llm_judged_abstention_accuracy": null,
  "audit_required_ids": [
    "cce-asset-image-059",
    "cce-asset-image-044",
    "cce-asset-image-038",
    "cce-asset-table-explanatory-014",
    "cce-asset-table-explanatory-028",
    "cce-asset-table-explanatory-017",
    "cce-asset-image-043",
    "cce-asset-image-051"
  ],
  "judge_usage": {
    "prompt_tokens": 489883,
    "completion_tokens": 5511,
    "total_tokens": 495394
  },
  "audited_passed": 40,
  "audited_count": 8,
  "audited_wilson_95": [
    0.9123754607496077,
    1.0
  ],
  "answerable_accuracy": 1.0,
  "abstention_accuracy": null
}
```

准确率仅在真实回答、语义评审和证据复核完成后报告；检索指标不代表答案正确。

## Bad cases（0）
