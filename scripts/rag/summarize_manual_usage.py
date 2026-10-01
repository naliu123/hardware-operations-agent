"""Summarize retained 03c model usage without assuming unrecorded calls were free."""

import json

from evaluate_manual import REPORT, now, save

FIELDS = ("prompt_tokens", "completion_tokens", "total_tokens")


def total(usages):
    return {field: sum(u.get(field, 0) for u in usages) for field in FIELDS}


def main():
    groups = []
    for path in sorted(REPORT.glob("*.json")):
        exp = json.loads(path.read_text())
        if not isinstance(exp, dict) or "config" not in exp or "rows" not in exp:
            continue
        if exp["config"].get("mode") != "LIVE_ANSWERS":
            continue
        rows = exp["rows"]
        usages = [r["result"].get("model_usage") or {} for r in rows]
        groups.append({"artifact": path.name, "kind": "public_http_generation_including_selection",
                       "status": exp["status"], "completed": len(rows), "expected": exp["expected"],
                       "calls": sum(u.get("calls", 0) for u in usages), **total(usages)})
        attempts = path.with_name(path.stem + "-judge-attempts.jsonl")
        calls = []
        retained = set()
        seen = set()
        if attempts.exists():
            for line in attempts.read_text().splitlines():
                attempt = json.loads(line)
                response = attempt["response"]
                identity = response.get("id") or line
                if identity not in seen:
                    calls.append(response.get("usage", {}))
                    retained.add(attempt["case_id"])
                    seen.add(identity)
        # Early runs preceded the attempt log. Their accepted totals remain
        # usable; never add them again when raw attempts already cover a row.
        fallback = [r["judge"] for r in rows if "judge" in r and r["id"] not in retained]
        groups.append({"artifact": attempts.name if attempts.exists() else path.name,
                       "kind": "judge", "calls": len(calls) + sum(j.get("recorded_attempts", 1) for j in fallback),
                       **total(calls + [j.get("usage", {}) for j in fallback])})
    seen_responses = set()
    seen_public = set()
    for path in sorted(REPORT.glob("*probe*.json")):
        value = json.loads(path.read_text())
        rows = value if isinstance(value, list) else value.get("rows", [])
        usages = []
        calls = 0
        for row in rows:
            if "response" in row and "choices" in row["response"]:
                response = row["response"]
                identity = response.get("id")
                if identity and identity not in seen_responses:
                    seen_responses.add(identity)
                    usages.append(response.get("usage", {}))
                    calls += 1
            elif "result" in row and row["result"].get("data_mode") == "LIVE":
                response = row["result"]
                identity = response.get("id")
                if identity and identity not in seen_public:
                    seen_public.add(identity)
                    usage = response.get("model_usage") or {}
                    usages.append(usage)
                    calls += usage.get("calls", 0)
        if calls:
            groups.append({"artifact": path.name, "kind": "diagnostic_probe_not_accuracy",
                           "calls": calls, **total(usages)})
    output = {
        "generated_at": now(),
        "scope": "03c retained LIVE generation, selection, judging and diagnostic probes",
        "currency_cost": None,
        "complete_cost_accounting": False,
        "limitations": [
            "Tokens and calls below are retained usage, not billed currency cost; no pricing assumptions.",
            "LIVE-R01 v2 first invalid-format judge attempt predates attempt logging: one call, unknown tokens.",
            "LIVE-R03 cce-manual-082 FAILED before failed generation usage was retained: unknown calls/tokens.",
            "Connection failures before provider responses may not have usage; a missing value is not evidence of zero cost.",
            "Running experiment totals cover only persisted completed rows, not in-flight calls.",
            "Generation totals include the R04 evidence-selection call; separate stage usage was not stored in these runs."
        ],
        "known_total": {"calls": sum(g["calls"] for g in groups), **total(groups)},
        "groups": groups,
    }
    save(REPORT / "usage-summary.json", output)
    lines = ["# 03c已保存模型用量", "", f"生成时间：{output['generated_at']}", "",
             "下表仅汇总已保存usage。存在早期未记录的失败请求，不能当作精确费用总账。", "",
             "| 产物 | 类型 | 已记录调用 | tokens |", "| --- | --- | ---: | ---: |"]
    for group in groups:
        lines.append(f"| [{group['artifact']}]({group['artifact']}) | {group['kind']} | {group['calls']} | {group['total_tokens']} |")
    lines += ["", f"已记录合计：{output['known_total']['calls']}次，{output['known_total']['total_tokens']} tokens。",
              "", "缺失：R01一次格式失败评审的tokens；R03题082失败生成的调用和tokens。",
              "诊断探针不计为问答准确率；运行中的轮次仅统计已落盘结果。"]
    (REPORT / "usage-summary.md").write_text("\n".join(lines) + "\n")
    print(json.dumps(output["known_total"], ensure_ascii=False))


if __name__ == "__main__":
    main()
