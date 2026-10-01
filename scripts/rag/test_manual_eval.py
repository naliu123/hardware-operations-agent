"""Behavior checks for experiment scoring (no substitute for model evaluation)."""

import importlib.util
from datetime import datetime, timezone
import json
from pathlib import Path
import shutil
import sys
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import patch

SCRIPT = Path(__file__).with_name("evaluate_manual.py")
MIGRATION_SCRIPT = Path(__file__).with_name("migrate_phoenix_evaluations.py")
REGISTER_SCRIPT = Path(__file__).with_name("register_phoenix_evaluators.py")
ROOT = SCRIPT.parents[2]


class EvaluationTests(unittest.TestCase):
    def test_fragment_paths_follow_the_configured_index_manifest(self):
        spec = importlib.util.spec_from_file_location("manual_eval", SCRIPT)
        evaluator = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(evaluator)
        v1 = evaluator.load_fragment_paths("hwops-cce-manual-v1")
        v2 = evaluator.load_fragment_paths("hwops-cce-manual-v2")
        self.assertEqual(len(v1), 3539)
        self.assertEqual(len(v2), 5458)
        self.assertNotEqual(set(v1), set(v2))

    def test_load_cases_uses_explicit_frozen_data_directory(self):
        spec = importlib.util.spec_from_file_location("manual_eval", SCRIPT)
        evaluator = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(evaluator)
        source = ROOT / "testdata/rag/cce-manual-eval"
        with tempfile.TemporaryDirectory() as directory:
            data = Path(directory)
            shutil.copy(source / "dataset.json", data / "dataset.json")
            shutil.copy(source / "dataset-manifest.json", data / "dataset-manifest.json")
            cases, manifest = evaluator.load_cases(data)
        self.assertEqual(len(cases), 111)
        self.assertEqual(manifest["sha256"], "aa778e2c4a824c64f13c6fbe2b5609c99f170be4f951824136b8f455fd4d9c31")

    def test_v2_selection_binds_new_heldout_without_changing_runtime_config(self):
        spec = importlib.util.spec_from_file_location("manual_eval", SCRIPT)
        evaluator = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(evaluator)
        experiment = {
            "name": "development-run",
            "config": {
                "split": "development",
                "dataset_sha256": "development-dataset",
                "corpus_sha256": "corpus",
                "rrf": 1,
                "model": "deepseek-flash",
            },
        }
        selection = evaluator.selection_payload(
            experiment, {"sha256": "new-heldout-dataset", "corpus_sha256": "corpus"}
        )
        heldout_config = dict(
            experiment["config"], split="heldout", dataset_sha256="new-heldout-dataset"
        )
        evaluator.validate_heldout_selection(selection, heldout_config)
        with self.assertRaisesRegex(ValueError, "matching frozen"):
            evaluator.validate_heldout_selection(
                selection, dict(heldout_config, rrf=60)
            )
        with self.assertRaisesRegex(ValueError, "dataset"):
            evaluator.validate_heldout_selection(
                selection, dict(heldout_config, dataset_sha256="different")
            )

    def test_each_selection_file_has_an_independent_holdout_use_marker(self):
        spec = importlib.util.spec_from_file_location("manual_eval", SCRIPT)
        evaluator = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(evaluator)
        self.assertEqual(
            evaluator.holdout_use_path(evaluator.REPORT / "selection.json"),
            evaluator.REPORT / "heldout-use.json",
        )
        self.assertEqual(
            evaluator.holdout_use_path(evaluator.REPORT / "selection-v2.json"),
            evaluator.REPORT / "selection-v2-use.json",
        )

    def test_partial_or_ungrounded_answers_are_not_counted_as_correct(self):
        spec = importlib.util.spec_from_file_location("manual_eval", SCRIPT)
        evaluator = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(evaluator)
        good = {"facts_met": [True, True], "contradictions": [], "unsupported_claims": [],
                "direct_answer": True, "correct_abstention": False}
        case = {"answerable": True, "required_facts": ["保留云盘", "删除PV"]}
        row = {"result": {"status": "ANSWERED", "data_mode": "LIVE"},
               "citation_integrity": True}
        self.assertTrue(evaluator.answer_pass(case, row, good))
        self.assertFalse(evaluator.answer_pass(case, row, dict(good, facts_met=[True, False])))
        self.assertFalse(evaluator.answer_pass(case, row, dict(good, unsupported_claims=["自造命令"])))
        self.assertFalse(evaluator.answer_pass(case, dict(row, citation_integrity=False), good))
        self.assertFalse(evaluator.answer_pass(case, dict(row, result={"status": "FAILED", "data_mode": "LIVE"}), good))
        self.assertFalse(evaluator.answer_pass(case, dict(row, result={"status": "ANSWERED", "data_mode": "REPLAY"}), good))
        self.assertFalse(evaluator.answer_pass(case, dict(row, error="HTTPError"), good))

    def test_judge_scores_remain_provisional_until_required_audit(self):
        spec = importlib.util.spec_from_file_location("manual_eval", SCRIPT)
        evaluator = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(evaluator)
        exp = {"config": {"answers": True}, "expected": 20, "rows": []}
        for i in range(20):
            exp["rows"].append({
                "id": str(i), "answerable": True, "metrics": {}, "latency_ms": 1,
                "result": {"status": "ANSWERED", "data_mode": "LIVE", "answer": f"答案{i}"},
                "judge": {"passed": i != 19},
            })
        summary = evaluator.aggregate(exp)
        self.assertEqual(summary["llm_judged_answer_accuracy"], .95)
        self.assertIsNone(summary["answer_accuracy"])
        self.assertEqual(len(summary["audit_required_ids"]), 5)
        for row in exp["rows"]:
            if row["id"] in summary["audit_required_ids"]:
                row["audit"] = {"passed": row["judge"]["passed"],
                                "answer_sha256": evaluator.sha(evaluator.encoded(row["result"]))}
        self.assertEqual(evaluator.aggregate(exp)["answer_accuracy"], .95)
        exp["rows"][19]["result"]["answer"] = "改动后的答案不能复用旧审核"
        self.assertIsNone(evaluator.aggregate(exp)["answer_accuracy"])

    def test_retrieval_run_can_freeze_after_reaching_dataset_target(self):
        spec = importlib.util.spec_from_file_location("manual_eval", SCRIPT)
        evaluator = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(evaluator)
        data = ROOT / "testdata/rag/cce-manual-assets-eval"
        with tempfile.TemporaryDirectory() as directory:
            report = Path(directory)
            evaluator.REPORT = report
            meta = json.loads((data / "dataset-manifest.json").read_text())
            rows = [{
                "id": str(index),
                "answerable": True,
                "metrics": {
                    "evidence_recall_at_5": 1.0,
                    "all_evidence_at_5": 1.0,
                    "document_hit_at_5": 1.0,
                },
                "latency_ms": 1,
                "result": {},
            } for index in range(40)]
            experiment = {
                "name": "retrieval-development",
                "status": "complete",
                "expected": 40,
                "config": {
                    "split": "development",
                    "answers": False,
                    "corpus_sha256": meta["corpus_sha256"],
                    "dataset_sha256": meta["sha256"],
                },
                "rows": rows,
            }
            (report / "retrieval-development.json").write_text(json.dumps(experiment))
            selection = report / "selection.json"
            evaluator.freeze(SimpleNamespace(
                name="retrieval-development",
                data_dir=data,
                selection_file=selection,
            ))
            self.assertTrue(selection.exists())

    def test_phoenix_payload_binds_examples_and_final_scores(self):
        spec = importlib.util.spec_from_file_location("manual_eval", SCRIPT)
        evaluator = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(evaluator)
        case = {
            "id": "asset-001",
            "question": "截图中显示什么？",
            "reference_answer": "显示运行中。",
            "required_facts": ["状态为运行中"],
            "answerable": True,
            "category": "IMAGE",
            "question_type": "image",
            "split": "heldout",
            "evidence": [{"source": "fixture://manual", "quote_sha256": "abc"}],
        }
        example = evaluator.phoenix_dataset_example(case)
        self.assertEqual(example["metadata"]["case_id"], "asset-001")
        self.assertEqual(example["output"]["required_facts"], ["状态为运行中"])
        row = {
            "metrics": {"evidence_recall_at_5": 1.0},
            "latency_ms": 12.5,
            "result": {"status": "ANSWERED"},
            "judge": {"passed": True},
            "audit": {"passed": True, "answer_sha256": ""},
        }
        row["audit"]["answer_sha256"] = evaluator.sha(evaluator.encoded(row["result"]))
        scores = evaluator.phoenix_scores(row)
        self.assertEqual(scores["evidence_recall_at_5"], (1.0, "CODE"))
        self.assertEqual(scores["llm_judged_answer_pass"], (1.0, "LLM"))
        self.assertEqual(scores["final_answer_pass"], (1.0, "CODE"))

    def test_phoenix_preflight_fails_closed(self):
        spec = importlib.util.spec_from_file_location("manual_eval", SCRIPT)
        evaluator = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(evaluator)
        request = evaluator.httpx.Request("GET", "http://127.0.0.1:16006/healthz")
        with patch.object(
            evaluator.httpx,
            "get",
            side_effect=evaluator.httpx.ConnectError("offline", request=request),
        ):
            with self.assertRaisesRegex(RuntimeError, "PHOENIX_UNAVAILABLE"):
                evaluator.phoenix_preflight(
                    SimpleNamespace(phoenix_url="http://127.0.0.1:16006")
                )

    def test_phoenix_backfill_uses_original_experiment_time(self):
        spec = importlib.util.spec_from_file_location("manual_eval", SCRIPT)
        evaluator = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(evaluator)
        started, ended = evaluator.row_times(
            {"created_at": "2026-09-20T10:00:00+00:00"},
            {"latency_ms": 1250},
            2,
        )
        self.assertEqual(started, datetime(2026, 9, 20, 10, 0, 0, 2, tzinfo=timezone.utc))
        self.assertEqual((ended - started).total_seconds(), 1.25)

    def test_phoenix_sync_is_default_and_offline_mode_is_explicit(self):
        spec = importlib.util.spec_from_file_location("manual_eval", SCRIPT)
        evaluator = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(evaluator)
        with patch.object(evaluator, "publish", return_value={"published": True}) as publish:
            result = evaluator.sync_phoenix(SimpleNamespace(no_phoenix=False))
            self.assertEqual(result, {"published": True})
            publish.assert_called_once()
        with patch.object(evaluator, "publish") as publish:
            result = evaluator.sync_phoenix(SimpleNamespace(no_phoenix=True))
            self.assertIsNone(result)
            publish.assert_not_called()

    def test_migration_inventory_only_accepts_complete_frozen_experiments(self):
        spec = importlib.util.spec_from_file_location("manual_eval", SCRIPT)
        evaluator = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(evaluator)
        previous = sys.modules.get("evaluate_manual")
        sys.modules["evaluate_manual"] = evaluator
        try:
            migration_spec = importlib.util.spec_from_file_location(
                "migration", MIGRATION_SCRIPT
            )
            migration = importlib.util.module_from_spec(migration_spec)
            migration_spec.loader.exec_module(migration)
        finally:
            if previous is None:
                del sys.modules["evaluate_manual"]
            else:
                sys.modules["evaluate_manual"] = previous
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            data = root / "testdata" / "sample-eval"
            reports = root / "reports"
            data.mkdir(parents=True)
            reports.mkdir()
            cases = [
                {"id": "dev-1", "split": "development"},
                {"id": "hold-1", "split": "heldout"},
            ]
            raw = evaluator.encoded(cases)
            (data / "dataset.json").write_bytes(raw)
            (data / "dataset-manifest.json").write_text(json.dumps({
                "sha256": evaluator.sha(raw),
            }))
            experiment = {
                "name": "complete-run",
                "status": "complete",
                "expected": 1,
                "config": {
                    "dataset_sha256": evaluator.sha(raw),
                    "split": "development",
                },
                "rows": [{
                    "id": "dev-1",
                    "metrics": {"evidence_recall_at_5": 1.0},
                    "latency_ms": 1,
                }],
            }
            (reports / "complete-run.json").write_text(json.dumps(experiment))
            (reports / "complete-run-badcases.json").write_text("[]")
            catalog = migration.dataset_catalog(root / "testdata")
            found = migration.discover_manual_experiments(reports, catalog)
        self.assertEqual([item["name"] for item in found], ["complete-run"])
        self.assertEqual(found[0]["runs"], 1)
        self.assertEqual(found[0]["evaluations"], 2)

    def test_registered_runtime_evaluator_handles_answer_failure_and_retrieval(self):
        spec = importlib.util.spec_from_file_location("register_evaluators", REGISTER_SCRIPT)
        registry = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(registry)
        namespace = {}
        exec(registry.EVALUATOR_SOURCE, namespace)
        evaluate = namespace["evaluate"]

        answered = evaluate({
            "status": "ANSWERED",
            "data_mode": "LIVE",
            "citations": [{"fragment_id": "fragment-1"}],
        })
        self.assertEqual(answered["live_data_mode"]["label"], "pass")
        self.assertEqual(answered["processing_success"]["label"], "pass")
        self.assertEqual(answered["citation_presence"]["label"], "pass")

        failed = evaluate({"status": "FAILED", "data_mode": "LIVE", "citations": []})
        self.assertEqual(failed["processing_success"]["label"], "fail")
        self.assertEqual(failed["citation_presence"]["label"], "not_applicable")

        retrieval = evaluate({"trace_id": "trace-1", "retrieval_queries": ["query"]})
        self.assertTrue(all(
            result["label"] == "not_applicable" for result in retrieval.values()
        ))

    def test_registered_runtime_evaluator_has_three_bounded_outputs(self):
        spec = importlib.util.spec_from_file_location("register_evaluators", REGISTER_SCRIPT)
        registry = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(registry)
        configs = registry.output_configs()
        self.assertEqual(
            [config["categorical"]["name"] for config in configs],
            ["live_data_mode", "processing_success", "citation_presence"],
        )
        for config in configs:
            self.assertEqual(
                config["categorical"]["values"],
                [
                    {"label": "pass", "score": 1.0},
                    {"label": "fail", "score": 0.0},
                    {"label": "not_applicable", "score": None},
                ],
            )


if __name__ == "__main__":
    unittest.main()
