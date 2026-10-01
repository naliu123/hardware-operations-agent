"""Bind explicit Agent review notes to retained answers; never creates verdicts."""

import argparse
import json
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "scripts/rag"))
from evaluate_manual import encoded, load_cases, save, sha


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--experiment", type=Path, required=True)
    parser.add_argument("--notes", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--data-dir", type=Path)
    args = parser.parse_args()
    experiment = json.loads(args.experiment.read_text())
    notes = json.loads(args.notes.read_text())
    rows = {r["id"]: r for r in experiment["rows"]}
    cases = {c["id"]: c for c in load_cases(args.data_dir)[0]} if args.data_dir else {
        c["id"]: c for c in load_cases()[0]
    }
    existing = {}
    if args.output.exists():
        existing = {r["id"]: r for r in json.loads(args.output.read_text())}
    reviews = []
    for case_id, (passed, reason) in notes["decisions"].items():
        row = rows[case_id]
        answer_hash = sha(encoded(row["result"]))
        if case_id in existing:
            old = existing[case_id]
            if (old["answer_sha256"] != answer_hash
                    or old["passed"] != passed or old["reason"] != reason
                    or old["reviewer"] != notes["reviewer"]):
                raise ValueError("Existing review changed; preserve it and use a new review version")
            reviews.append(old)
            continue
        reviews.append({
            "id": case_id,
            "passed": passed,
            "reviewer": notes["reviewer"],
            "reason": reason,
            "answer_sha256": answer_hash,
            "evidence_checked": {
                "reference": [e["quote_sha256"] for e in cases[case_id]["evidence"]],
                "retrieved": [
                    d["source"] + "#" + d["section"] + " sha256=" + sha(d["content"].encode())
                    for d in row["documents"]
                ],
                "answer_and_gaps": True,
            },
        })
    if set(existing) - set(notes["decisions"]):
        raise ValueError("Existing review missing from notes")
    save(args.output, reviews)
    print(f"Bound {len(reviews)} explicit reviews; import after judging completes.")


if __name__ == "__main__":
    main()
