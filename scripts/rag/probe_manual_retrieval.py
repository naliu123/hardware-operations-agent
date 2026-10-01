"""Small public-HTTP regression probe for observed development retrieval failures."""

import argparse
import json

import httpx

from evaluate_manual import STATE, call, load_cases, metrics

parser = argparse.ArgumentParser()
parser.add_argument("--ids", nargs="+", default=["cce-manual-081", "cce-manual-093"])
parser.add_argument("--strategy", default="hybrid", choices=["bm25", "dense", "hybrid"])
args = parser.parse_args()
cases, _ = load_cases()
cases = {c["id"]: c for c in cases if c["split"] == "development"}
failed = []
with httpx.Client(base_url="http://127.0.0.1:18080", timeout=60,
                  headers={"Authorization": "Bearer " + (STATE / "api-token").read_text().strip()}) as client:
    for case_id in args.ids:
        case = cases[case_id]
        result = call(client, "POST", "/v1/knowledge/search",
                      {"query": case["question"], "strategy": args.strategy, "top_k": 5})
        score = metrics(case, result["documents"])
        print(case_id, json.dumps(score), [d["title"] for d in result["documents"]])
        if score["all_evidence_at_5"] != 1:
            failed.append(case_id)
if failed:
    raise SystemExit("MISSING_EVIDENCE: " + ", ".join(failed))
