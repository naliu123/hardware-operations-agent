# asset-v3-dev-answers-r00

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
  "latency_p95_ms": 34127.52004200593,
  "generation_usage": {
    "calls": 81,
    "prompt_tokens": 2195906,
    "completion_tokens": 14485,
    "total_tokens": 2210391
  },
  "citation_integrity_rate": 1.0,
  "evidence_selection": {
    "responses": 40,
    "candidate_fragments_total": 1789,
    "expanded_fragments_total": 233,
    "input_bytes_total": 7185492,
    "final_context_bytes_max": 46961
  },
  "llm_judged_passed": 40,
  "wilson_95": [
    0.9123754607496077,
    1.0
  ],
  "audit_status": "completed",
  "real_model_calls": 81,
  "llm_judged_answerable_accuracy": 1.0,
  "llm_judged_abstention_accuracy": null,
  "audit_required_ids": [
    "cce-asset-image-059",
    "cce-asset-image-044",
    "cce-asset-table-data-029",
    "cce-asset-image-058",
    "cce-asset-image-038",
    "cce-asset-table-explanatory-014",
    "cce-asset-table-data-025",
    "cce-asset-table-explanatory-017"
  ],
  "judge_usage": {
    "prompt_tokens": 243514,
    "completion_tokens": 5544,
    "total_tokens": 249058
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
