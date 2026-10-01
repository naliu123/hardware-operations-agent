"""Publish the captured CCE manual through the Go HTTP API and verify ES."""

import argparse
import hashlib
import json
from pathlib import Path
import re
import time

import httpx

from common import ROOT, STATE, save

DEFAULT_DATA = ROOT / "testdata/rag/cce-manual"
DEFAULT_REPORT = ROOT / "reports/rag-cce-manual-20260920"
DEFAULT_INDEX = "hwops-cce-manual-v1"
MAX_REVISION_BYTES = 220 * 1024


def split_large_text(text, limit):
    parts = []
    current = []
    size = 0
    for char in text:
        encoded = len(char.encode())
        if current and size + encoded > limit:
            parts.append("".join(current))
            current, size = [], 0
        current.append(char)
        size += encoded
    if current:
        parts.append("".join(current))
    return parts


def split_document(text, limit=MAX_REVISION_BYTES):
    if len(text.encode()) <= limit:
        return [text]
    paragraphs = re.split(r"\n{2,}", text)
    parts, current, size = [], [], 0
    for paragraph in paragraphs:
        pieces = split_large_text(paragraph, limit) if len(paragraph.encode()) > limit else [paragraph]
        for piece in pieces:
            added = len(piece.encode()) + (2 if current else 0)
            if current and size + added > limit:
                parts.append("\n\n".join(current).strip() + "\n")
                current, size = [], 0
            current.append(piece)
            size += len(piece.encode()) + (2 if len(current) > 1 else 0)
    if current:
        parts.append("\n\n".join(current).strip() + "\n")
    if not parts or any(len(part.encode()) > limit for part in parts):
        raise ValueError("failed to bound revision content")
    return parts


def request(client, method, path, body=None):
    response = client.request(method, path, json=body)
    response.raise_for_status()
    return response.json()


def atomic_save(path, value):
    temporary = path.with_suffix(path.suffix + ".tmp")
    save(temporary, value)
    temporary.replace(path)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--data", type=Path, default=DEFAULT_DATA)
    parser.add_argument("--report", type=Path, default=DEFAULT_REPORT)
    parser.add_argument("--index", default=DEFAULT_INDEX)
    parser.add_argument("--enrichment", type=Path)
    args = parser.parse_args()
    corpus = json.loads((args.data / "manifest.json").read_text())
    if corpus["failed_documents"] or corpus["captured_documents"] != corpus["expected_documents"]:
        raise SystemExit("capture manifest is incomplete")
    report = args.report.resolve()
    report.mkdir(parents=True, exist_ok=True)
    state_path = report / "ingestion.json"
    enrichment = None
    enrichment_hash = None
    if args.enrichment:
        enrichment_raw = args.enrichment.read_bytes()
        enrichment_hash = hashlib.sha256(enrichment_raw).hexdigest()
        enrichment = json.loads(enrichment_raw)
        if (enrichment.get("status") != "complete" or
                enrichment.get("corpus_sha256") != corpus["corpus_sha256"] or
                set(enrichment.get("documents", {})) != {item["id"] for item in corpus["documents"]}):
            raise SystemExit("semantic enrichment is incomplete or belongs to another corpus")
    state = json.loads(state_path.read_text()) if state_path.exists() else {
        "schema_version": 2 if enrichment else 1,
        "index": args.index,
        "corpus_sha256": corpus["corpus_sha256"],
        "navigation_sha256": corpus["navigation_sha256"],
        "enrichment_sha256": enrichment_hash,
        "documents": {},
    }
    if state["index"] != args.index:
        raise SystemExit("existing ingestion uses another index")
    if state["corpus_sha256"] != corpus["corpus_sha256"]:
        raise SystemExit("captured corpus changed; use a new index/report version")
    if state.get("enrichment_sha256") != enrichment_hash:
        raise SystemExit("semantic enrichment changed; use a new index/report version")
    token = (STATE / "api-token").read_text().strip()
    with httpx.Client(base_url="http://127.0.0.1:18080", timeout=300,
                      headers={"Authorization": "Bearer " + token}) as client:
        for position, source in enumerate(corpus["documents"], 1):
            path = args.data / source["file"]
            raw = path.read_text()
            if hashlib.sha256(raw.encode()).hexdigest() != source["content_sha256"]:
                raise RuntimeError(f"captured content changed: {source['id']}")
            enriched = enrichment["documents"][source["id"]] if enrichment else None
            parts = [raw] if enriched else split_document(raw)
            entry = state["documents"].setdefault(source["id"], {
                "title": source["title"], "source": source["url"],
                "content_sha256": source["content_sha256"], "parts": [],
            })
            if entry["content_sha256"] != source["content_sha256"] or len(entry["parts"]) > len(parts):
                raise RuntimeError(f"ingestion checkpoint differs: {source['id']}")
            for part_number, content in enumerate(parts, 1):
                while len(entry["parts"]) < part_number:
                    entry["parts"].append({"part": len(entry["parts"]) + 1})
                part = entry["parts"][part_number - 1]
                part_hash = hashlib.sha256(content.encode()).hexdigest()
                if part.get("content_sha256") not in (None, part_hash):
                    raise RuntimeError(f"part checkpoint differs: {source['id']}:{part_number}")
                part["content_sha256"] = part_hash
                title = source["title"]
                if len(parts) > 1:
                    title += f"（第{part_number}/{len(parts)}部分）"
                if "revision_id" not in part:
                    payload = {
                        "title": title, "source": source["url"], "content": content,
                        "applicability": {"scope": "GENERAL"},
                    }
                    if enriched:
                        payload["fragments"] = enriched["fragments"]
                    revision = request(client, "POST", "/v1/knowledge/revisions", payload)
                    part.update({"revision_id": revision["id"], "status": revision["status"]})
                    atomic_save(state_path, state)
                if part.get("status") != "PUBLISHED":
                    revision = request(
                        client, "POST",
                        f"/v1/knowledge/revisions/{part['revision_id']}/publication",
                        {"decision": "PUBLISH"},
                    )
                    part.update({
                        "status": revision["status"],
                        "fragment_count": len(revision["fragments"]),
                        "fragment_ids": [fragment["id"] for fragment in revision["fragments"]],
                    })
                    atomic_save(state_path, state)
            if position % 10 == 0:
                fragments = sum(
                    part.get("fragment_count", 0)
                    for document in state["documents"].values()
                    for part in document["parts"]
                )
                print(f"Ingest {position}/{len(corpus['documents'])} fragments={fragments}", flush=True)

    parts = [part for document in state["documents"].values() for part in document["parts"]]
    if not parts or any(part.get("status") != "PUBLISHED" for part in parts):
        raise RuntimeError("not every revision is published")
    expected_fragments = sum(part["fragment_count"] for part in parts)
    generated_representations = (
        sum(item["representation_count"] for item in enrichment["documents"].values())
        if enrichment else 0
    )
    content_representations = (
        sum(
            not any(rep["kind"] == "PASSAGE" for rep in fragment["representations"])
            for item in enrichment["documents"].values()
            for fragment in item["fragments"]
        )
        if enrichment else expected_fragments
    )
    with httpx.Client(timeout=30) as client:
        fragment_count = client.get(f"http://127.0.0.1:19200/{args.index}/_count").raise_for_status().json()["count"]
        revision_count = client.get(f"http://127.0.0.1:19200/{args.index}-revisions/_count").raise_for_status().json()["count"]
    if fragment_count != expected_fragments or revision_count != len(parts):
        raise RuntimeError(
            f"ES count mismatch: fragments={fragment_count}/{expected_fragments}, revisions={revision_count}/{len(parts)}"
        )
    state.update({
        "completed_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        "document_count": len(state["documents"]), "revision_count": len(parts),
        "fragment_count": fragment_count,
        "representation_count": generated_representations,
        "content_representation_count": content_representations,
        "vector_representation_count": generated_representations + content_representations,
        "status": "complete",
    })
    atomic_save(state_path, state)
    print(json.dumps({key: state[key] for key in [
        "index", "document_count", "revision_count", "fragment_count",
        "corpus_sha256", "status",
    ]}, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
