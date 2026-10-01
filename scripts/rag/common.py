"""Shared live HTTP access and frozen experiment artifacts (no credentials exported)."""

import hashlib
import json
import os
from pathlib import Path

import httpx

ROOT = Path(__file__).resolve().parents[2]
DATA = ROOT / "testdata/rag/cce"
REPORT = ROOT / "reports/rag-cce-20260920"
STATE = ROOT / ".local/rag"
PHOENIX = "http://127.0.0.1:16006"
os.environ.setdefault("PHOENIX_WORKING_DIR", str(STATE / "phoenix"))
os.environ.setdefault("PHOENIX_TELEMETRY_ENABLED", "false")


def save(path, value):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + "\n")


def frozen_cases(split=None):
    raw = (DATA / "dataset.json").read_bytes()
    manifest = json.loads((DATA / "dataset-manifest.json").read_text())
    if hashlib.sha256(raw).hexdigest() != manifest["sha256"]:
        raise RuntimeError("Frozen evaluation dataset changed")
    cases = json.loads(raw)
    return [case for case in cases if split is None or case["split"] == split]


def api():
    return httpx.Client(base_url="http://127.0.0.1:18080", timeout=65,
                        headers={"Authorization": "Bearer " + (STATE / "api-token").read_text().strip()})


def call(client, method, path, body=None):
    response = client.request(method, path, json=body)
    response.raise_for_status()
    return response.json()
