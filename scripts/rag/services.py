"""Start/stop only experiment-owned local processes; retain databases and reports."""

import argparse
import hashlib
import json
import os
import secrets
import signal
import subprocess
from pathlib import Path
from urllib.parse import urlparse

import psutil

ROOT = Path(__file__).resolve().parents[2]
STATE = ROOT / ".local/rag"
PYTHON = ROOT / ".tools/rag-venv/bin/python"
MANIFEST = STATE / "processes.json"
DEFAULT_MODEL = "deepseek-v4.1-flash"
DEFAULT_MODEL_ENDPOINT = "https://opencode.ai/zen/go/v1/chat/completions"


def alive(item):
    try:
        process = psutil.Process(item["pid"])
        return abs(process.create_time() - item["created"]) < 0.1
    except psutil.NoSuchProcess:
        return False


def start():
    STATE.mkdir(parents=True, exist_ok=True)
    if MANIFEST.exists():
        previous = json.loads(MANIFEST.read_text())
        if any(alive(item) for item in previous):
            raise SystemExit("Experiment processes already running; use status/stop first.")
    config = STATE / "elasticsearch-config"
    config.mkdir(exist_ok=True)
    (config / "elasticsearch.yml").write_text(
        f"cluster.name: hwops-rag\nnode.name: local-rag\n"
        "network.host: 127.0.0.1\nhttp.port: 19200\ntransport.port: 19300\n"
        "discovery.type: single-node\nxpack.security.enabled: false\n"
        "xpack.ml.enabled: false\nxpack.license.self_generated.type: basic\n"
        f"path.data: {STATE / 'elasticsearch-data'}\npath.logs: {STATE / 'elasticsearch-logs'}\n"
    )
    es_home = ROOT / ".tools/elasticsearch-9.5.4"
    for filename in ["jvm.options", "log4j2.properties"]:
        (config / filename).write_bytes((es_home / "config" / filename).read_bytes())
    (config / "jvm.options.d").mkdir(exist_ok=True)
    token_file = STATE / "api-token"
    if not token_file.exists():
        token_file.write_text(secrets.token_urlsafe(24))
        token_file.chmod(0o600)
    env = os.environ.copy()
    env.update(
        ES_PATH_CONF=str(config),
        ES_JAVA_OPTS="-Xms512m -Xmx512m -XX:ActiveProcessorCount=2",
        PHOENIX_WORKING_DIR=str(STATE / "phoenix"),
        PHOENIX_HOST="127.0.0.1",
        PHOENIX_PORT="16006",
        PHOENIX_ENABLE_AUTH="false",
        PHOENIX_ALLOW_EXTERNAL_RESOURCES="false",
        PHOENIX_TELEMETRY_ENABLED="false",
        PHOENIX_DISABLE_AGENT_ASSISTANT="true",
        PHOENIX_ENABLE_MCP_SERVER="false",
        HF_HOME=str(ROOT / ".cache/rag/huggingface"),
        HF_HUB_DISABLE_TELEMETRY="1",
        HF_HUB_DISABLE_XET="1",
        HWOPS_EMBED_PORT="18765",
        OMP_NUM_THREADS="2",
        TOKENIZERS_PARALLELISM="false",
    )
    processes = []
    try:
        for name, command in [
            ("elasticsearch", [str(es_home / "bin/elasticsearch")]),
            ("phoenix", [str(PYTHON), "scripts/rag/phoenix_local.py"]),
            ("embedding", [str(PYTHON), "scripts/rag/embedding_server.py"]),
        ]:
            with (STATE / f"{name}.log").open("ab") as log:
                process = subprocess.Popen(command, cwd=ROOT, env=env, stdout=log, stderr=log, start_new_session=True)
            processes.append({"name": name, "pid": process.pid, "created": psutil.Process(process.pid).create_time()})
            MANIFEST.write_text(json.dumps(processes, indent=2))
        print("Started local experiment processes; verify health with status.")
    except BaseException:
        stop()
        raise


def stop():
    if not MANIFEST.exists():
        print("No experiment process manifest.")
        return
    items = json.loads(MANIFEST.read_text())
    owned = []
    for item in items:
        if alive(item):
            process = psutil.Process(item["pid"])
            owned.extend(process.children(recursive=True))
            owned.append(process)
            try:
                os.killpg(item["pid"], signal.SIGTERM)
            except ProcessLookupError:
                pass
    _, remaining = psutil.wait_procs(owned, timeout=15)
    for process in remaining:
        try:
            process.kill()
        except psutil.NoSuchProcess:
            pass
    psutil.wait_procs(remaining, timeout=5)
    key = ROOT / ".cache/rag/deepseek.key"
    if key.exists():
        key.unlink()
    print(json.dumps({"stopped": [item["name"] for item in items], "remaining": [item["name"] for item in items if alive(item)]}))


def status():
    items = json.loads(MANIFEST.read_text()) if MANIFEST.exists() else []
    output = []
    for item in items:
        entry = dict(item, alive=alive(item))
        if entry["alive"]:
            process = psutil.Process(item["pid"])
            family = [process] + process.children(recursive=True)
            entry["rss_mib"] = round(sum(p.memory_info().rss for p in family if p.is_running())/1024**2, 1)
            entry["listeners"] = [
                f"{c.laddr.ip}:{c.laddr.port}"
                for p in family for c in p.net_connections("inet") if c.status == "LISTEN"
            ]
        output.append(entry)
    print(json.dumps(output, indent=2))


def app(strategy, mode, index, state_path, rrf_constant=60, evidence_selection=False,
        query_rewrite=False, multi_vector=False, model_name=None, model_endpoint=None):
    if not 1 <= rrf_constant <= 1000:
        raise SystemExit("rrf constant must be 1..1000")
    key = None
    if mode == "live":
        model_name = model_name or os.environ.get("HWOPS_MODEL") or DEFAULT_MODEL
        model_endpoint = model_endpoint or os.environ.get("HWOPS_MODEL_ENDPOINT") or DEFAULT_MODEL_ENDPOINT
        hostname = urlparse(model_endpoint).hostname
        local_endpoint = hostname in {"127.0.0.1", "localhost", "::1"}
        if not local_endpoint:
            key = os.environ.get("HWOPS_MODEL_API_KEY")
            if hostname == "api.deepseek.com":
                key = key or os.environ.get("DEEPSEEK_API_KEY")
                key_files = [STATE / "deepseek.key", ROOT / ".cache/rag/deepseek.key"]
            else:
                key_files = [STATE / "model.key"]
            for key_file in key_files:
                if not key and key_file.is_file():
                    key = key_file.read_text().strip()
        if not key and not local_endpoint:
            raise SystemExit("MODEL_CREDENTIAL_MISSING: configure HWOPS_MODEL_API_KEY or .local/rag/model.key")
    items = json.loads(MANIFEST.read_text())
    for item in items:
        if item["name"] == "hwopsd" and alive(item):
            process = psutil.Process(item["pid"])
            process.terminate()
            process.wait(timeout=15)
    items = [item for item in items if item["name"] != "hwopsd"]
    env = os.environ.copy()
    env.update(
        HWOPS_API_TOKEN=(STATE / "api-token").read_text().strip(),
        HWOPS_MODEL_MODE=mode.upper(),
        HWOPS_LISTEN_ADDR="127.0.0.1:18080",
        HWOPS_STATE_PATH=str(ROOT / state_path),
        HWOPS_ES_URL="http://127.0.0.1:19200",
        HWOPS_EMBED_URL="http://127.0.0.1:18765",
        HWOPS_ES_INDEX=index,
        HWOPS_RETRIEVAL_STRATEGY=strategy,
        HWOPS_RRF_CONSTANT=str(rrf_constant),
        HWOPS_EVIDENCE_SELECTION=str(evidence_selection).lower(),
        HWOPS_QUERY_REWRITE=str(query_rewrite).lower(),
        HWOPS_MULTI_VECTOR=str(multi_vector).lower(),
        HWOPS_PHOENIX_ENDPOINT="http://127.0.0.1:16006/v1/traces",
        HWOPS_PHOENIX_PROJECT="hwops-cce-rag",
    )
    if mode == "live":
        env.update(
            HWOPS_MODEL=model_name,
            HWOPS_MODEL_ENDPOINT=model_endpoint,
            HWOPS_MODEL_API_KEY=key or "",
        )
    with (STATE / "hwopsd.log").open("ab") as log:
        process = subprocess.Popen([str(STATE / "hwopsd")], cwd=ROOT, env=env,
                                   stdout=log, stderr=log, start_new_session=True)
    items.append({"name": "hwopsd", "pid": process.pid, "created": psutil.Process(process.pid).create_time(),
                  "strategy": strategy, "mode": mode, "index": index, "state_path": state_path,
                  "rrf_constant": rrf_constant,
                  "evidence_selection": evidence_selection,
                  "query_rewrite": query_rewrite,
                  "multi_vector": multi_vector,
                  "model": model_name if mode == "live" else None,
                  "model_endpoint": model_endpoint if mode == "live" else None,
                  "binary_sha256": hashlib.sha256((STATE / "hwopsd").read_bytes()).hexdigest()})
    MANIFEST.write_text(json.dumps(items, indent=2))
    print(f"Started Go HTTP application with {strategy} retrieval.")


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("action", choices=["start", "stop", "status", "app"])
    parser.add_argument("--strategy", choices=["bm25", "dense", "hybrid"], default="bm25")
    parser.add_argument("--mode", choices=["live", "replay"], default="live")
    parser.add_argument("--index", default="hwops-cce-v1")
    parser.add_argument("--state-path", default=".local/rag/hwops-state.json")
    parser.add_argument("--rrf-constant", type=int, default=60)
    parser.add_argument("--evidence-selection", action="store_true")
    parser.add_argument("--query-rewrite", action="store_true")
    parser.add_argument("--multi-vector", action="store_true")
    parser.add_argument("--model")
    parser.add_argument("--model-endpoint")
    args = parser.parse_args()
    if args.action == "app":
        app(args.strategy, args.mode, args.index, args.state_path, args.rrf_constant,
            args.evidence_selection, args.query_rewrite, args.multi_vector,
            args.model, args.model_endpoint)
    else:
        {"start": start, "stop": stop, "status": status}[args.action]()
