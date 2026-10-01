"""Inventory and idempotently publish all historical RAG evaluations to Phoenix."""

import argparse
import json
from pathlib import Path
from types import SimpleNamespace

import evaluate_manual as evaluator

ROOT = Path(__file__).resolve().parents[2]
DEFAULT_REPORT = ROOT / "reports/rag-phoenix-evaluations"
LEGACY_REPORT = ROOT / "reports/rag-cce-20260920"


def load_json(path):
    return json.loads(path.read_text())


def dataset_catalog(testdata_dir):
    catalog = {}
    for manifest_path in sorted(testdata_dir.glob("*eval*/dataset-manifest.json")):
        data_dir = manifest_path.parent
        manifest = load_json(manifest_path)
        raw = (data_dir / "dataset.json").read_bytes()
        if evaluator.sha(raw) != manifest["sha256"]:
            raise ValueError(f"frozen dataset changed: {data_dir}")
        cases = json.loads(raw)
        splits = {}
        for case in cases:
            splits.setdefault(case["split"], []).append(case["id"])
        if manifest["sha256"] in catalog:
            raise ValueError(f"duplicate dataset SHA256: {manifest['sha256']}")
        catalog[manifest["sha256"]] = {
            "data_dir": data_dir.resolve(),
            "name": data_dir.name,
            "splits": splits,
        }
    return catalog


def discover_manual_experiments(reports_dir, catalog):
    experiments = []
    for path in sorted(reports_dir.rglob("*.json")):
        try:
            value = load_json(path)
        except (json.JSONDecodeError, UnicodeDecodeError):
            continue
        if not (
            isinstance(value, dict)
            and isinstance(value.get("config"), dict)
            and isinstance(value.get("rows"), list)
            and isinstance(value.get("expected"), int)
            and isinstance(value.get("name"), str)
            and "status" in value
        ):
            continue
        if value["status"] != "complete" or value["expected"] != len(value["rows"]):
            raise ValueError(f"incomplete evaluation artifact: {path}")
        if path.stem != value["name"]:
            raise ValueError(f"experiment name does not match its file: {path}")
        dataset_sha = value["config"].get("dataset_sha256")
        split = value["config"].get("split")
        if dataset_sha not in catalog:
            raise ValueError(f"no frozen dataset for experiment: {path}")
        expected_ids = set(catalog[dataset_sha]["splits"].get(split, []))
        row_ids = [row.get("id") for row in value["rows"]]
        if len(row_ids) != len(set(row_ids)) or set(row_ids) != expected_ids:
            raise ValueError(f"experiment rows differ from frozen split: {path}")
        experiments.append({
            "name": value["name"],
            "path": path.resolve(),
            "report_dir": path.parent.resolve(),
            "data_dir": catalog[dataset_sha]["data_dir"],
            "dataset_sha256": dataset_sha,
            "split": split,
            "runs": len(row_ids),
            "evaluations": expected_manual_evaluations(value),
        })
    return experiments


def expected_manual_evaluations(experiment):
    final = experiment.get("summary", {}).get("audit_status") == "completed"
    return sum(len(evaluator.phoenix_scores(row, final=final)) for row in experiment["rows"])


def expected_legacy_evaluations(experiment):
    total = 0
    for row in experiment["rows"]:
        total += len(row.get("metrics", {})) + 1
        if "judge" in row:
            total += sum(
                row["judge"]["scores"].get(name) is not None
                for name in ["correctness", "faithfulness", "relevance"]
            )
    return total


def new_registry(phoenix_url):
    return {
        "schema_version": 2,
        "phoenix_url": phoenix_url,
        "datasets": {},
        "experiments": {},
    }


def merge_reference(target, key, value, kind):
    existing = target.get(key)
    if existing is not None and existing["id"] != value["id"]:
        raise ValueError(f"conflicting Phoenix {kind} IDs for {key}")
    if existing is None:
        target[key] = value


def adopt_manual_publications(registry, reports_dir):
    for path in sorted(reports_dir.rglob("phoenix-publish.json")):
        old = load_json(path)
        if old.get("schema_version") != 1:
            continue
        if old.get("phoenix_url") != registry["phoenix_url"]:
            raise ValueError(f"publication belongs to another Phoenix server: {path}")
        for key, reference in old.get("datasets", {}).items():
            merge_reference(registry["datasets"], key, reference, "dataset")
        for name, reference in old.get("experiments", {}).items():
            report = (path.parent / f"{name}.json").resolve()
            key = evaluator.repo_relative(report)
            adopted = {
                **reference,
                "name": name,
                "local_report": key,
                "source": "adopted-schema-v1",
            }
            merge_reference(registry["experiments"], key, adopted, "experiment")


def legacy_experiments(legacy_report):
    result = []
    for path in sorted(legacy_report.glob("*.json")):
        value = load_json(path)
        if (
            isinstance(value, dict)
            and isinstance(value.get("id"), str)
            and isinstance(value.get("config"), dict)
            and isinstance(value.get("rows"), list)
            and all("phoenix_run_id" in row for row in value["rows"])
        ):
            result.append((path.resolve(), value))
    return result


def adopt_legacy(registry, legacy_report, legacy_data_dir):
    dataset_sha = load_json(legacy_data_dir / "dataset-manifest.json")["sha256"]
    ingestion = load_json(legacy_report / "ingestion.json")
    cases = load_json(legacy_data_dir / "dataset.json")
    case_ids = {}
    for case in cases:
        case_ids.setdefault(case["split"], set()).add(case["id"])
    for split, reference in ingestion["datasets"].items():
        key = f"{dataset_sha}:{split}"
        adopted = {
            **reference,
            "name": f"hwops-cce-v1-{split}",
            "dataset_sha256": dataset_sha,
            "source": "native-03a",
        }
        if set(adopted["examples"]) != case_ids[split]:
            raise ValueError(f"legacy Phoenix dataset differs from frozen {split} split")
        merge_reference(registry["datasets"], key, adopted, "dataset")
    for path, experiment in legacy_experiments(legacy_report):
        split = experiment["config"]["split"]
        key = evaluator.repo_relative(path)
        runs = {}
        for row in experiment["rows"]:
            evaluations = list(row.get("metrics", {})) + ["latency_ms"]
            if "judge" in row:
                evaluations += [
                    "llm_" + name
                    for name in ["correctness", "faithfulness", "relevance"]
                    if row["judge"]["scores"].get(name) is not None
                ]
            runs[row["id"]] = {
                "id": row["phoenix_run_id"],
                "evaluations": evaluations,
            }
        adopted = {
            "id": experiment["id"],
            "name": experiment["name"],
            "local_report": key,
            "dataset_key": f"{dataset_sha}:{split}",
            "config_sha256": evaluator.sha(evaluator.encoded(experiment["config"])),
            "runs": runs,
            "source": "native-03a",
        }
        merge_reference(registry["experiments"], key, adopted, "experiment")


def inventory_payload(candidates, registry):
    legacy = [
        (path, value)
        for path, value in legacy_experiments(LEGACY_REPORT)
        if evaluator.repo_relative(path) in registry["experiments"]
    ]
    rows = []
    for candidate in candidates:
        key = evaluator.repo_relative(candidate["path"])
        rows.append({
            **{name: value for name, value in candidate.items()
               if name not in {"path", "report_dir", "data_dir"}},
            "local_report": key,
            "data_dir": evaluator.repo_relative(candidate["data_dir"]),
            "state": "published" if key in registry["experiments"] else "pending",
        })
    legacy_rows = [{
        "name": value["name"],
        "local_report": evaluator.repo_relative(path),
        "split": value["config"]["split"],
        "runs": len(value["rows"]),
        "evaluations": expected_legacy_evaluations(value),
        "state": "published",
    } for path, value in legacy]
    all_rows = legacy_rows + rows
    dataset_keys = set(registry["datasets"])
    dataset_keys.update(
        f"{candidate['dataset_sha256']}:{candidate['split']}" for candidate in candidates
    )
    return {
        "schema_version": 1,
        "created_at": evaluator.now(),
        "datasets": len(dataset_keys),
        "experiments": len(all_rows),
        "runs": sum(row["runs"] for row in all_rows),
        "evaluations": sum(row["evaluations"] for row in all_rows),
        "published": sum(row["state"] == "published" for row in all_rows),
        "pending": sum(row["state"] == "pending" for row in all_rows),
        "items": all_rows,
    }


def verify_remote(registry):
    from phoenix.client import Client

    client = Client(base_url=registry["phoenix_url"])
    datasets = {}
    for key, reference in registry["datasets"].items():
        restored = client.datasets.get_dataset(
            dataset=reference["id"], version_id=reference["version_id"], timeout=60
        )
        if len(restored.examples) != len(reference["examples"]):
            raise ValueError(f"Phoenix dataset example count mismatch: {key}")
        datasets[key] = len(restored.examples)
    experiments = {}
    for key, reference in registry["experiments"].items():
        local = load_json(ROOT / key)
        expected_runs = len(local["rows"])
        expected_evaluations = (
            expected_legacy_evaluations(local)
            if reference.get("source") == "native-03a"
            else expected_manual_evaluations(local)
        )
        remote = client.experiments.get(experiment_id=reference["id"])
        actual_runs = remote["successful_run_count"] + remote["failed_run_count"]
        details = client.experiments.get_experiment(experiment_id=reference["id"])
        actual_evaluations = len(details["evaluation_runs"])
        if (
            actual_runs != expected_runs
            or remote["missing_run_count"] != 0
            or actual_evaluations != expected_evaluations
        ):
            raise ValueError(f"Phoenix experiment count mismatch: {key}")
        experiments[key] = {
            "id": reference["id"],
            "runs": actual_runs,
            "evaluations": actual_evaluations,
        }
    return {
        "schema_version": 1,
        "verified_at": evaluator.now(),
        "phoenix_url": registry["phoenix_url"],
        "datasets": len(datasets),
        "experiments": len(experiments),
        "runs": sum(item["runs"] for item in experiments.values()),
        "evaluations": sum(item["evaluations"] for item in experiments.values()),
        "dataset_examples": sum(datasets.values()),
        "items": experiments,
    }


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--phoenix-url", default="http://127.0.0.1:16006")
    parser.add_argument("--manifest", type=Path, default=evaluator.PHOENIX_MANIFEST)
    parser.add_argument("--inventory", type=Path, default=DEFAULT_REPORT / "inventory.json")
    parser.add_argument("--verification", type=Path, default=DEFAULT_REPORT / "verification.json")
    parser.add_argument("--dry-run", action="store_true")
    args = parser.parse_args()

    catalog = dataset_catalog(ROOT / "testdata/rag")
    candidates = discover_manual_experiments(ROOT / "reports", catalog)
    registry = load_json(args.manifest) if args.manifest.exists() else new_registry(args.phoenix_url)
    if registry.get("schema_version") != 2:
        raise ValueError("Phoenix manifest schema mismatch")
    if registry.get("phoenix_url") != args.phoenix_url:
        raise ValueError("Phoenix manifest belongs to another server")
    adopt_manual_publications(registry, ROOT / "reports")
    adopt_legacy(registry, LEGACY_REPORT, ROOT / "testdata/rag/cce")
    inventory = inventory_payload(candidates, registry)
    evaluator.save(args.inventory, inventory)
    if args.dry_run:
        print(json.dumps({key: inventory[key] for key in [
            "datasets", "experiments", "runs", "evaluations", "published", "pending"
        ]}, ensure_ascii=False, indent=2))
        return

    evaluator.phoenix_preflight(args)
    evaluator.save(args.manifest, registry)
    for index, candidate in enumerate(candidates, start=1):
        evaluator.REPORT = candidate["report_dir"]
        result = evaluator.publish(SimpleNamespace(
            name=candidate["name"],
            data_dir=candidate["data_dir"],
            phoenix_url=args.phoenix_url,
            phoenix_manifest=args.manifest,
            dataset_name=None,
            quiet=True,
            register_evaluators=False,
        ))
        print(
            f"publish {index}/{len(candidates)} {candidate['name']} "
            f"runs={result['experiment']['runs']} evaluations={result['experiment']['evaluations']}",
            flush=True,
        )
    registry = load_json(args.manifest)
    from register_phoenix_evaluators import register_runtime_contract

    register_runtime_contract(args.phoenix_url, args.manifest)
    registry = load_json(args.manifest)
    verification = verify_remote(registry)
    registry["verification"] = {
        key: verification[key]
        for key in ["verified_at", "datasets", "experiments", "runs", "evaluations", "dataset_examples"]
    }
    evaluator.save(args.manifest, registry)
    evaluator.save(args.verification, verification)
    print(json.dumps(registry["verification"], ensure_ascii=False, indent=2))


if __name__ == "__main__":
    try:
        main()
    except (ValueError, RuntimeError, evaluator.httpx.HTTPError) as exc:
        raise SystemExit(str(exc))
