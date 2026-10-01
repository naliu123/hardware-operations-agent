"""Register the executable deterministic RAG evaluator in Phoenix."""

import argparse
import hashlib
import json
from pathlib import Path

import httpx

ROOT = Path(__file__).resolve().parents[2]
DEFAULT_MANIFEST = ROOT / "reports/rag-phoenix-evaluations/manifest.json"
DEFAULT_REPORT = ROOT / "reports/rag-phoenix-evaluations/evaluators.json"
EVALUATOR_NAME = "hwops-rag-runtime-contract-v1"
EVALUATOR_DESCRIPTION = (
    "Deterministic RAG runtime checks for LIVE mode, terminal processing, and citation "
    "presence. This is not semantic answer accuracy."
)
SANDBOX_CONFIG_ID = "U2FuZGJveENvbmZpZzoy"
INPUT_MAPPING = {"literalMapping": {}, "pathMapping": {}}
EVALUATOR_SOURCE = '''def evaluate(output):
    if not isinstance(output, dict):
        output = {}
    status = output.get("status")
    if status is None:
        not_applicable = {
            "label": "not_applicable",
            "explanation": "Retrieval-only output has no answer status.",
        }
        return {
            "live_data_mode": not_applicable,
            "processing_success": not_applicable,
            "citation_presence": not_applicable,
        }

    live = "pass" if output.get("data_mode") == "LIVE" else "fail"
    processing = "fail" if status in ("FAILED", "QUEUED", "RUNNING") else "pass"
    if status in ("ANSWERED", "PARTIAL"):
        citations = "pass" if bool(output.get("citations")) else "fail"
        citation_explanation = "Answer-like responses must contain at least one citation."
    else:
        citations = "not_applicable"
        citation_explanation = "Citation presence does not apply to this response status."

    return {
        "live_data_mode": {
            "label": live,
            "explanation": "Only LIVE responses count as model evaluation.",
        },
        "processing_success": {
            "label": processing,
            "explanation": "Queued, running, and failed responses do not pass processing.",
        },
        "citation_presence": {
            "label": citations,
            "explanation": citation_explanation,
        },
    }
'''

OUTPUT_DESCRIPTIONS = {
    "live_data_mode": "Whether an answer response is explicitly LIVE.",
    "processing_success": "Whether processing reached a terminal non-failure state.",
    "citation_presence": "Whether ANSWERED/PARTIAL responses include citations.",
}

QUERY_EVALUATORS = """
query EvaluatorRegistry {
  evaluatorCount
  evaluators(first: 100) {
    edges {
      node {
        __typename
        id
        name
        description
        kind
        datasetEvaluators {
          id
          name
          dataset { id name }
          inputMapping { literalMapping pathMapping }
          outputConfigs {
            __typename
            ... on CategoricalAnnotationConfig {
              name
              optimizationDirection
              values { label score }
            }
          }
        }
        ... on CodeEvaluator {
          language
          sandboxConfig { id name }
          currentVersion { sourceCode }
          inputMapping { literalMapping pathMapping }
          outputConfigs {
            __typename
            ... on CategoricalAnnotationConfig {
              name
              optimizationDirection
              values { label score }
            }
          }
        }
      }
    }
  }
}
"""

PREVIEW_EVALUATOR = """
mutation PreviewEvaluator($input: EvaluatorPreviewsInput!) {
  evaluatorPreviews(input: $input) {
    results {
      evaluatorName
      error
      annotation { name label score explanation }
    }
  }
}
"""

CREATE_EVALUATOR = """
mutation CreateEvaluator($input: CreateCodeEvaluatorInput!) {
  createCodeEvaluator(input: $input) {
    evaluator { id name }
  }
}
"""

CREATE_DATASET_EVALUATOR = """
mutation AttachEvaluator($input: CreateDatasetCodeEvaluatorInput!) {
  createDatasetCodeEvaluator(input: $input) {
    evaluator {
      id
      name
      dataset { id name }
      evaluator { id name kind }
    }
  }
}
"""


def encoded(value):
    return (json.dumps(value, ensure_ascii=False, indent=2) + "\n").encode()


def sha256(value):
    raw = value.encode() if isinstance(value, str) else encoded(value)
    return hashlib.sha256(raw).hexdigest()


def save(path, value):
    path.parent.mkdir(parents=True, exist_ok=True)
    temporary = path.with_suffix(path.suffix + ".tmp")
    temporary.write_bytes(encoded(value))
    temporary.replace(path)


def output_configs():
    values = [
        {"label": "pass", "score": 1.0},
        {"label": "fail", "score": 0.0},
        {"label": "not_applicable", "score": None},
    ]
    return [
        {
            "categorical": {
                "name": name,
                "description": description,
                "optimizationDirection": "MAXIMIZE",
                "values": values,
            }
        }
        for name, description in OUTPUT_DESCRIPTIONS.items()
    ]


def graphql(client, query, variables=None):
    response = client.post(
        "/graphql",
        json={"query": query, "variables": variables or {}},
    )
    response.raise_for_status()
    payload = response.json()
    if payload.get("errors"):
        messages = "; ".join(error.get("message", "GraphQL error") for error in payload["errors"])
        raise ValueError(messages)
    return payload["data"]


def normalized_remote_configs(configs):
    return [
        {
            "categorical": {
                "name": config["name"],
                "optimizationDirection": config["optimizationDirection"],
                "values": config["values"],
            }
        }
        for config in configs
    ]


def expected_configs_without_descriptions():
    return [
        {
            "categorical": {
                "name": config["categorical"]["name"],
                "optimizationDirection": config["categorical"]["optimizationDirection"],
                "values": config["categorical"]["values"],
            }
        }
        for config in output_configs()
    ]


def validate_evaluator(reference):
    if reference["__typename"] != "CodeEvaluator" or reference["kind"] != "CODE":
        raise ValueError(f"{EVALUATOR_NAME} exists but is not a code evaluator")
    if reference["description"] != EVALUATOR_DESCRIPTION:
        raise ValueError(f"{EVALUATOR_NAME} description differs from the local definition")
    if reference["language"] != "PYTHON":
        raise ValueError(f"{EVALUATOR_NAME} language differs from the local definition")
    if not reference.get("sandboxConfig") or reference["sandboxConfig"]["id"] != SANDBOX_CONFIG_ID:
        raise ValueError(f"{EVALUATOR_NAME} sandbox differs from the local definition")
    if not reference.get("currentVersion") or reference["currentVersion"]["sourceCode"] != EVALUATOR_SOURCE:
        raise ValueError(f"{EVALUATOR_NAME} source differs from the local definition")
    if reference["inputMapping"] != INPUT_MAPPING:
        raise ValueError(f"{EVALUATOR_NAME} input mapping differs from the local definition")
    if (
        reference["outputConfigs"]
        and normalized_remote_configs(reference["outputConfigs"])
        != expected_configs_without_descriptions()
    ):
        raise ValueError(f"{EVALUATOR_NAME} output configs differ from the local definition")


def preview_payload():
    outputs = [
        {"status": "ANSWERED", "data_mode": "LIVE", "citations": [{"fragment_id": "fixture"}]},
        {"status": "FAILED", "data_mode": "LIVE", "citations": []},
        {"trace_id": "fixture", "retrieval_queries": ["fixture"]},
    ]
    evaluator = {
        "inlineCodeEvaluator": {
            "name": EVALUATOR_NAME,
            "description": EVALUATOR_DESCRIPTION,
            "language": "PYTHON",
            "sourceCode": EVALUATOR_SOURCE,
            "outputConfigs": output_configs(),
            "sandboxConfigId": SANDBOX_CONFIG_ID,
        }
    }
    return {
        "previews": [
            {
                "context": {"input": {}, "output": output, "reference": {}, "metadata": {}},
                "evaluator": evaluator,
                "inputMapping": INPUT_MAPPING,
            }
            for output in outputs
        ]
    }


def validate_preview(results):
    expected = [
        ("live_data_mode", "pass"),
        ("processing_success", "pass"),
        ("citation_presence", "pass"),
        ("live_data_mode", "pass"),
        ("processing_success", "fail"),
        ("citation_presence", "not_applicable"),
        ("live_data_mode", "not_applicable"),
        ("processing_success", "not_applicable"),
        ("citation_presence", "not_applicable"),
    ]
    observed = []
    for result in results:
        if result.get("error") or not result.get("annotation"):
            raise ValueError(f"Phoenix evaluator preview failed: {result.get('error')}")
        name = result["annotation"]["name"].removeprefix(EVALUATOR_NAME + ".")
        observed.append((name, result["annotation"]["label"]))
    if observed != expected:
        raise ValueError("Phoenix evaluator preview returned unexpected labels")


def create_evaluator(client):
    preview = graphql(client, PREVIEW_EVALUATOR, {"input": preview_payload()})
    validate_preview(preview["evaluatorPreviews"]["results"])
    data = graphql(client, CREATE_EVALUATOR, {
        "input": {
            "name": EVALUATOR_NAME,
            "description": EVALUATOR_DESCRIPTION,
            "sourceCode": EVALUATOR_SOURCE,
            "language": "PYTHON",
            "sandboxConfigId": SANDBOX_CONFIG_ID,
            "outputConfigs": output_configs(),
            "inputMapping": INPUT_MAPPING,
        }
    })
    return data["createCodeEvaluator"]["evaluator"]["id"]


def attach_evaluator(client, evaluator_id, dataset_id):
    data = graphql(client, CREATE_DATASET_EVALUATOR, {
        "input": {
            "datasetId": dataset_id,
            "evaluatorId": evaluator_id,
            "name": EVALUATOR_NAME,
            "description": EVALUATOR_DESCRIPTION,
            "inputMapping": INPUT_MAPPING,
            "outputConfigs": output_configs(),
        }
    })
    return data["createDatasetCodeEvaluator"]["evaluator"]["id"]


def register_runtime_contract(phoenix_url, manifest_path=DEFAULT_MANIFEST, report_path=None):
    manifest_path = Path(manifest_path).resolve()
    report_path = (
        Path(report_path).resolve()
        if report_path is not None
        else manifest_path.parent / "evaluators.json"
    )
    manifest = json.loads(manifest_path.read_text())
    if manifest.get("phoenix_url") != phoenix_url:
        raise ValueError("Phoenix evaluator registry belongs to another server")

    with httpx.Client(base_url=phoenix_url, timeout=60) as client:
        registry = graphql(client, QUERY_EVALUATORS)
        matches = [
            edge["node"]
            for edge in registry["evaluators"]["edges"]
            if edge["node"]["name"] == EVALUATOR_NAME
        ]
        if len(matches) > 1:
            raise ValueError(f"multiple Phoenix evaluators named {EVALUATOR_NAME}")
        if matches:
            evaluator = matches[0]
            validate_evaluator(evaluator)
            evaluator_id = evaluator["id"]
        else:
            evaluator_id = create_evaluator(client)
            registry = graphql(client, QUERY_EVALUATORS)
            evaluator = next(
                edge["node"]
                for edge in registry["evaluators"]["edges"]
                if edge["node"]["id"] == evaluator_id
            )
            validate_evaluator(evaluator)

        attached_by_dataset = {
            reference["dataset"]["id"]: reference
            for reference in evaluator["datasetEvaluators"]
        }
        dataset_links = {}
        for dataset_key, dataset in sorted(manifest["datasets"].items()):
            existing = attached_by_dataset.get(dataset["id"])
            if existing is not None:
                if existing["name"] != EVALUATOR_NAME:
                    raise ValueError(f"unexpected evaluator binding for dataset {dataset_key}")
                if existing["inputMapping"] != INPUT_MAPPING:
                    raise ValueError(f"evaluator input mapping differs for dataset {dataset_key}")
                if (
                    normalized_remote_configs(existing["outputConfigs"])
                    != expected_configs_without_descriptions()
                ):
                    raise ValueError(f"evaluator output configs differ for dataset {dataset_key}")
                dataset_evaluator_id = existing["id"]
            else:
                dataset_evaluator_id = attach_evaluator(client, evaluator_id, dataset["id"])
            dataset_links[dataset_key] = {
                "dataset_id": dataset["id"],
                "dataset_evaluator_id": dataset_evaluator_id,
            }

        verified = graphql(client, QUERY_EVALUATORS)
        evaluator = next(
            edge["node"]
            for edge in verified["evaluators"]["edges"]
            if edge["node"]["id"] == evaluator_id
        )
        validate_evaluator(evaluator)
        remote_datasets = {item["dataset"]["id"] for item in evaluator["datasetEvaluators"]}
        expected_datasets = {item["id"] for item in manifest["datasets"].values()}
        if remote_datasets != expected_datasets:
            raise ValueError("Phoenix evaluator dataset bindings differ from the global manifest")
        for binding in evaluator["datasetEvaluators"]:
            if binding["inputMapping"] != INPUT_MAPPING:
                raise ValueError("Phoenix evaluator input mapping differs from the local definition")
            if (
                normalized_remote_configs(binding["outputConfigs"])
                != expected_configs_without_descriptions()
            ):
                raise ValueError("Phoenix evaluator output configs differ from the local definition")

    reference = {
        "id": evaluator_id,
        "name": EVALUATOR_NAME,
        "kind": "CODE",
        "sandbox_config_id": SANDBOX_CONFIG_ID,
        "source_sha256": sha256(EVALUATOR_SOURCE),
        "output_configs_sha256": sha256(output_configs()),
        "datasets": dataset_links,
    }
    manifest.setdefault("evaluator_definitions", {})[EVALUATOR_NAME] = reference
    save(manifest_path, manifest)
    report = {
        "schema_version": 1,
        "phoenix_url": phoenix_url,
        "evaluator_count": verified["evaluatorCount"],
        "evaluator": reference,
    }
    save(report_path, report)
    return report


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--phoenix-url", default="http://127.0.0.1:16006")
    parser.add_argument("--manifest", type=Path, default=DEFAULT_MANIFEST)
    parser.add_argument("--report", type=Path, default=DEFAULT_REPORT)
    args = parser.parse_args()
    result = register_runtime_contract(args.phoenix_url, args.manifest, args.report)
    print(json.dumps(result, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    try:
        main()
    except (ValueError, RuntimeError, httpx.HTTPError) as exc:
        raise SystemExit(str(exc))
