"""Verify complete CCE corpus coverage and representative public searches."""

import argparse
import hashlib
import json
from pathlib import Path
import time

import httpx

from common import ROOT, STATE, save

DEFAULT_DATA = ROOT / "testdata/rag/cce-manual"
DEFAULT_REPORT = ROOT / "reports/rag-cce-manual-20260920"
DEFAULT_INDEX = "hwops-cce-manual-v1"
DEFAULT_ALIAS = "hwops-cce-manual"
SEARCH_CASES = [
    ("Kubernetes 1.36版本说明", "cce_bulletin_0131"),
    ("购买Standard Turbo集群", "cce_10_0028"),
    ("纳管具备安全启动能力的ECS实例", "cce_10_1156"),
    ("CCE节点kubelet和runtime组件路径与社区原生配置差异说明", "cce_10_0883"),
    ("使用kubectl部署带文件存储卷的有状态工作负载", "cce_10_0321"),
]


def api_call(client, query, strategy):
    response = client.post("/v1/knowledge/search", json={
        "query": query, "strategy": strategy, "top_k": 5,
    })
    response.raise_for_status()
    return response.json()


def replace_alias(client, alias, index):
    existing = client.get(f"/_alias/{alias}")
    actions = []
    if existing.status_code == 200:
        actions.extend({"remove": {"index": old, "alias": alias}} for old in existing.json())
    elif existing.status_code != 404:
        existing.raise_for_status()
    actions.append({"add": {"index": index, "alias": alias}})
    response = client.post("/_aliases", json={"actions": actions})
    response.raise_for_status()


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--data", type=Path, default=DEFAULT_DATA)
    parser.add_argument("--report", type=Path, default=DEFAULT_REPORT)
    parser.add_argument("--index", default=DEFAULT_INDEX)
    parser.add_argument("--alias", default=DEFAULT_ALIAS)
    args = parser.parse_args()
    corpus = json.loads((args.data / "manifest.json").read_text())
    navigation = json.loads((args.data / "navigation.json").read_text())
    ingestion = json.loads((args.report / "ingestion.json").read_text())
    documents = {item["id"]: item for item in corpus["documents"]}
    checks = []
    for item in corpus["documents"]:
        raw = (args.data / item["file"]).read_bytes()
        if hashlib.sha256(raw).hexdigest() != item["content_sha256"]:
            raise RuntimeError(f"corpus file changed: {item['id']}")
    if corpus["captured_documents"] != navigation["document_count"]:
        raise RuntimeError("not every navigation document was captured")
    if set(documents) != set(ingestion["documents"]):
        raise RuntimeError("captured and ingested document sets differ")

    with httpx.Client(base_url="http://127.0.0.1:19200", timeout=30) as es:
        fragment_count = es.get(f"/{args.index}/_count").raise_for_status().json()["count"]
        revision_count = es.get(f"/{args.index}-revisions/_count").raise_for_status().json()["count"]
        if fragment_count != ingestion["fragment_count"] or revision_count != ingestion["revision_count"]:
            raise RuntimeError("Elasticsearch counts differ from ingestion checkpoint")
        representation_count = None
        if ingestion.get("representation_count"):
            aggregate = es.post(f"/{args.index}/_search", json={
                "size": 0,
                "aggs": {"representations": {"sum": {"field": "representation_count"}}},
            }).raise_for_status().json()
            representation_count = int(aggregate["aggregations"]["representations"]["value"])
            expected = ingestion.get(
                "vector_representation_count",
                ingestion["representation_count"] + fragment_count,
            )
            if representation_count != expected:
                raise RuntimeError(
                    f"Elasticsearch representation count differs: {representation_count}/{expected}"
                )
        replace_alias(es, args.alias, args.index)
        replace_alias(es, args.alias + "-revisions", args.index + "-revisions")
        aliases = es.get(f"/_alias/{args.alias},{args.alias}-revisions").raise_for_status().json()

    token = (STATE / "api-token").read_text().strip()
    with httpx.Client(base_url="http://127.0.0.1:18080", timeout=120,
                      headers={"Authorization": "Bearer " + token}) as client:
        for query, document_id in SEARCH_CASES:
            expected = documents[document_id]["url"]
            for strategy in ["bm25", "dense"]:
                result = api_call(client, query, strategy)
                sources = [item["source"] for item in result["documents"]]
                passed = expected in sources
                checks.append({
                    "query": query, "strategy": strategy, "expected_source": expected,
                    "retrieved_sources": sources, "trace_id": result["trace_id"], "passed": passed,
                })
                if not passed:
                    raise RuntimeError(f"{strategy} search missed {document_id}: {sources}")

    result = {
        "verified_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        "navigation_entries": navigation["entry_count"],
        "category_entries": navigation["category_count"],
        "document_entries": navigation["document_count"],
        "captured_documents": corpus["captured_documents"],
        "ingested_documents": ingestion["document_count"],
        "revision_count": revision_count,
        "fragment_count": fragment_count,
        "representation_count": representation_count,
        "corpus_sha256": corpus["corpus_sha256"],
        "index": args.index,
        "aliases": {name: sorted(data["aliases"]) for name, data in aliases.items()},
        "public_search_checks": checks,
        "all_checks_passed": all(item["passed"] for item in checks),
    }
    save(args.report / "verification.json", result)
    print(json.dumps(result, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
