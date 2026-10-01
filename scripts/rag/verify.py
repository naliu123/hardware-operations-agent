"""Read back ES, Phoenix datasets/runs/traces and export reproducible evidence."""

import json
import os
import time

from common import PHOENIX, REPORT, ROOT, frozen_cases, save
import httpx
from phoenix.client import Client


def main():
    pc = Client(base_url=PHOENIX)
    ingestion = json.loads((REPORT / "ingestion.json").read_text())
    experiments = [json.loads(path.read_text()) for path in sorted(REPORT.glob("*.json"))
                   if path.name.startswith(("dev-", "heldout-"))]
    refs = {row["result"]["trace_id"] for exp in experiments for row in exp["rows"]}
    refs |= {row["judge"]["trace_id"] for exp in experiments for row in exp["rows"] if "judge" in row}
    for attempt in range(5):
        df = pc.spans.get_spans_dataframe(project_identifier="hwops-cce-rag", limit=10000)
        traced = set(df["context.trace_id"])
        if refs <= traced:
            break
        time.sleep(2)
    assert refs <= traced, f"{len(refs-traced)} experiment traces missing from Phoenix"
    kinds = df["attributes.openinference.span.kind"].value_counts().to_dict()
    assert {"CHAIN", "RETRIEVER", "EMBEDDING", "LLM", "EVALUATOR"} <= set(kinds)
    raw = df.to_json(orient="records", date_format="iso", force_ascii=False)
    key_file = ROOT / ".cache/rag/deepseek.key"
    key = os.environ.get("HWOPS_MODEL_API_KEY") or (key_file.read_text().strip() if key_file.exists() else "")
    if key:
        assert key not in raw, "Credential found in traces"
        for path in REPORT.rglob("*"):
            if path.is_file() and path.suffix in [".json", ".csv", ".md", ".html"]:
                assert key not in path.read_text(), f"Credential in {path.name}"
    (REPORT / "phoenix-spans.json").write_text(raw)
    df.to_csv(REPORT / "phoenix-spans.csv", index=False)
    experiment_checks = []
    for exp in experiments:
        restored = pc.experiments.get_experiment(experiment_id=exp["id"])
        # RanExperiment uses public mapping fields.
        count = len(restored["task_runs"])
        assert count == len(exp["rows"]), f"Phoenix run count mismatch: {exp['name']}"
        experiment_checks.append({"name": exp["name"], "id": exp["id"], "runs": count})
    dataset_checks = []
    for split, spec in ingestion["datasets"].items():
        dataset = pc.datasets.get_dataset(dataset=spec["id"], version_id=spec["version_id"])
        assert len(dataset.examples) == len(frozen_cases(split))
        dataset_checks.append({"split": split, "id": dataset.id, "version_id": dataset.version_id, "count": len(dataset)})
    with httpx.Client(timeout=30) as client:
        response = client.get("http://127.0.0.1:19200/hwops-cce-v1/_count")
        response.raise_for_status()
        count = response.json()["count"]
        assert count == ingestion["fragment_count"] == 56
        docs = client.post("http://127.0.0.1:19200/hwops-cce-v1/_search",
                           json={"size": 100, "_source": {"excludes": ["embedding"]}}).json()
        save(REPORT / "elasticsearch-documents.json", docs)
        assert client.get("http://127.0.0.1:19200/hwops-cce-v1-revisions/_count").json()["count"] == 1
    checks = {"elasticsearch_fragments": count, "datasets": dataset_checks, "experiments": experiment_checks,
              "phoenix_span_count": len(df), "span_kinds": kinds,
              "experiment_trace_count": len(refs), "all_experiment_traces_present": True,
              "credential_scan": "passed"}
    save(REPORT / "verification.json", checks)
    print(json.dumps(checks, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
