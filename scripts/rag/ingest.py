"""Publish captured source via Go, then create and round-trip Phoenix datasets."""

import json

from common import DATA, PHOENIX, REPORT, api, call, frozen_cases, save
from phoenix.client import Client


def main():
    manifest_path = REPORT / "ingestion.json"
    manifest = json.loads(manifest_path.read_text()) if manifest_path.exists() else {}
    with api() as client:
        if "revision_id" not in manifest:
            revision = call(client, "POST", "/v1/knowledge/revisions", {
                "title": "华为云 CCE 高危操作一览",
                "source": "https://support.huaweicloud.com/usermanual-cce/cce_10_0054.html",
                "content": (DATA / "document.md").read_text(),
                "applicability": {"scope": "GENERAL"},
            })
            manifest["revision_id"] = revision["id"]
            save(manifest_path, manifest)
        revision = call(client, "POST", f"/v1/knowledge/revisions/{manifest['revision_id']}/publication",
                        {"decision": "PUBLISH"})
        save(REPORT / "published-revision.json", revision)
        manifest["fragment_to_evidence"] = {
            f["id"]: f["section"].split()[0] for f in revision["fragments"]
            if f["section"].startswith("cce-t")
        }
        assert len(manifest["fragment_to_evidence"]) == 55
        manifest["fragment_count"] = len(revision["fragments"])
        save(manifest_path, manifest)
    pc = Client(base_url=PHOENIX)
    manifest.setdefault("datasets", {})
    for split in ["development", "heldout"]:
        cases = frozen_cases(split)
        if split not in manifest["datasets"]:
            dataset = pc.datasets.create_dataset(
                name=f"hwops-cce-v1-{split}",
                dataset_description="Agent-authored, source-grounded Chinese questions; frozen evidence-group split. Live RAG evaluation, not production traffic.",
                examples=[{
                    "input": {"question": case["question"]},
                    "output": {key: case[key] for key in ["reference_answer", "relevant_evidence_ids", "answerable"]},
                    "metadata": {"case_id": case["id"], "split": split, "section": case["section"],
                                 "question_type": case["question_type"], "labels": "agent-authored"},
                } for case in cases],
            )
            manifest["datasets"][split] = {"id": dataset.id, "version_id": dataset.version_id}
            save(manifest_path, manifest)
        ref = manifest["datasets"][split]
        retrieved = pc.datasets.get_dataset(dataset=ref["id"], version_id=ref["version_id"])
        assert len(retrieved.examples) == len(cases)
        reference = {case["id"]: case for case in cases}
        mapping = {}
        for example in retrieved.examples:
            case_id = example["metadata"]["case_id"]
            assert example["input"]["question"] == reference[case_id]["question"]
            assert example["output"]["relevant_evidence_ids"] == reference[case_id]["relevant_evidence_ids"]
            mapping[case_id] = example["node_id"]
        ref["examples"] = mapping
        save(manifest_path, manifest)
        print(f"Phoenix {split}: {len(cases)} examples verified", flush=True)
    print(f"Published {manifest['fragment_count']} source fragments (55 evidence rows + source preamble)", flush=True)


if __name__ == "__main__":
    main()
