"""Live Go HTTP evaluation, fixed k, immutable cases; logs each run to Phoenix."""

import argparse
from datetime import datetime, timezone
import json
import math
import statistics
import time

from common import DATA, PHOENIX, REPORT, STATE, api, call, frozen_cases, save
from phoenix.client import Client


def retrieval_metrics(retrieved, relevant):
    relevant = set(relevant)
    if not relevant:
        return {}
    seen = set()
    gains = []
    for evidence_id in retrieved[:5]:
        gains.append(int(evidence_id in relevant and evidence_id not in seen))
        seen.add(evidence_id)
    hits = sum(gains)
    dcg = sum(g / math.log2(i + 2) for i, g in enumerate(gains))
    ideal = sum(1 / math.log2(i + 2) for i in range(min(5, len(relevant))))
    return {
        "recall_at_1": len(set(retrieved[:1]) & relevant) / len(relevant),
        "recall_at_3": len(set(retrieved[:3]) & relevant) / len(relevant),
        "recall_at_5": hits / len(relevant),
        "precision_at_5": hits / 5,
        "hit_at_5": float(hits > 0),
        "mrr_at_5": next((1 / (i + 1) for i, g in enumerate(gains) if g), 0),
        "ndcg_at_5": dcg / ideal,
    }


def aggregate(rows):
    keys = sorted({key for row in rows for key in row["metrics"]})
    means = {key: statistics.mean(row["metrics"][key] for row in rows if key in row["metrics"]) for key in keys}
    latency = sorted(row["latency_ms"] for row in rows)
    return {"count": len(rows), "answerable_count": sum(row["answerable"] for row in rows),
            "means": means, "latency_ms": {
                "mean": statistics.mean(latency),
                "p50": statistics.median(latency),
                "p95": latency[max(0, math.ceil(len(latency)*.95)-1)],
            }}


def answer(client, question):
    conversation = call(client, "POST", "/v1/conversations", {})
    pending = call(client, "POST", f"/v1/conversations/{conversation['id']}/messages", {"text": question})
    deadline = time.monotonic() + 65
    while time.monotonic() < deadline:
        result = call(client, "GET", f"/v1/responses/{pending['id']}")
        if result["status"] not in ["QUEUED", "RUNNING"]:
            return result
        time.sleep(.1)
    raise TimeoutError("Go response deadline exceeded")


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--name", required=True)
    parser.add_argument("--split", choices=["development", "heldout"], required=True)
    parser.add_argument("--strategy", choices=["bm25", "dense", "hybrid"], required=True)
    parser.add_argument("--answers", action="store_true")
    args = parser.parse_args()
    cases = frozen_cases(args.split)
    if args.split == "heldout":
        selection = json.loads((REPORT / "selection.json").read_text())
        assert selection["strategy"] == args.strategy, "Heldout requires frozen selected configuration"
    if args.answers:
        processes = json.loads((STATE / "processes.json").read_text())
        assert next(p["strategy"] for p in processes if p["name"] == "hwopsd") == args.strategy
    ingestion = json.loads((REPORT / "ingestion.json").read_text())
    dataset = ingestion["datasets"][args.split]
    mapping = ingestion["fragment_to_evidence"]
    result_path = REPORT / f"{args.name}.json"
    pc = Client(base_url=PHOENIX)
    config = {"split": args.split, "strategy": args.strategy, "answers": args.answers, "top_k": 5,
              "candidate_k": 20, "rrf_constant": 60, "data_mode": "LIVE",
              "dataset_sha256": json.loads((DATA / "dataset-manifest.json").read_text())["sha256"]}
    if result_path.exists():
        experiment = json.loads(result_path.read_text())
        assert experiment["config"] == config, "Experiment name cannot be reused with a changed config"
    else:
        created = pc.experiments.create(
            dataset_id=dataset["id"], dataset_version_id=dataset["version_id"],
            experiment_name=args.name, experiment_metadata=config,
            experiment_description="Go public HTTP + actual ES/local BGE" + (" + DeepSeek Flash" if args.answers else "; retrieval only"),
        )
        experiment = {"id": created["id"], "name": args.name, "config": config, "rows": []}
        save(result_path, experiment)
    completed = {row["id"] for row in experiment["rows"]}
    with api() as client:
        for case in cases:
            if case["id"] in completed:
                continue
            start_time = datetime.now(timezone.utc)
            started = time.perf_counter()
            if args.answers:
                result = answer(client, case["question"])
                fragment_ids = result.get("retrieved_fragment_ids", [])
            else:
                result = call(client, "POST", "/v1/knowledge/search",
                              {"query": case["question"], "top_k": 5, "strategy": args.strategy})
                fragment_ids = [doc["id"] for doc in result["documents"]]
            latency = (time.perf_counter() - started)*1000
            ended = datetime.now(timezone.utc)
            evidence_ids = [mapping.get(fid, "source-preamble") for fid in fragment_ids]
            metrics = retrieval_metrics(evidence_ids, case["relevant_evidence_ids"])
            if args.answers:
                metrics["processing_success"] = float(result["status"] != "FAILED")
                metrics["answer_rate"] = float(result["status"] == "ANSWERED")
                citations = result.get("citations", [])
                if not case["answerable"]:
                    metrics["abstention_accuracy"] = float(result["status"] == "UNRESOLVED")
                if case["answerable"]:
                    cited = {mapping.get(c["fragment_id"]) for c in citations}
                    relevant = set(case["relevant_evidence_ids"])
                    metrics["citation_recall"] = len(cited & relevant) / len(relevant)
                    metrics["citation_precision"] = len(cited & relevant) / len(cited) if cited else 0
                    metrics["citation_hit"] = float(bool(cited & relevant))
                if citations:
                    valid = []
                    for citation in citations:
                        source = call(client, "GET", citation["url"])
                        valid.append(source["content_hash"] == citation["content_hash"] and
                                     source["publication_status"] == "PUBLISHED")
                    metrics["citation_integrity"] = sum(valid) / len(valid)
            row = {"id": case["id"], "question": case["question"], "answerable": case["answerable"],
                   "question_type": case["question_type"], "section": case["section"],
                   "relevant_evidence_ids": case["relevant_evidence_ids"], "retrieved_evidence_ids": evidence_ids,
                   "latency_ms": latency, "metrics": metrics, "result": result,
                   "started_at": start_time.isoformat(), "ended_at": ended.isoformat()}
            run = pc.experiments.log_run(experiment_id=experiment["id"],
                                        dataset_example_id=dataset["examples"][case["id"]],
                                        output=result, start_time=start_time, end_time=ended,
                                        trace_id=result.get("trace_id") or None)
            row["phoenix_run_id"] = run["id"]
            for key, score in metrics.items():
                pc.experiments.log_evaluation(experiment_run_id=run["id"], name=key, score=score, annotator_kind="CODE")
            pc.experiments.log_evaluation(experiment_run_id=run["id"], name="latency_ms", score=latency, annotator_kind="CODE")
            experiment["rows"].append(row)
            experiment["summary"] = aggregate(experiment["rows"])
            save(result_path, experiment)
            print(f"{args.name} {len(experiment['rows'])}/{len(cases)} {case['id']} R@5={metrics.get('recall_at_5', 'n/a')}", flush=True)
    print(json.dumps(experiment["summary"], indent=2), flush=True)


if __name__ == "__main__":
    main()
