"""Live public-HTTP probe for spurious gaps, separate from frozen full evaluations."""

import argparse
import time

import httpx

from evaluate_manual import REPORT, STATE, call, load_cases, now, save, sha

parser = argparse.ArgumentParser()
parser.add_argument("--name", required=True)
parser.add_argument("--ids", nargs="+", default=[
    "cce-manual-016", "cce-manual-018", "cce-manual-022",
])
args = parser.parse_args()
if not args.name.replace("-", "").isalnum():
    raise SystemExit("invalid name")
cases, _ = load_cases()
cases = {c["id"]: c for c in cases if c["split"] == "development"}
path = REPORT / (args.name + ".json")
if path.exists():
    raise SystemExit("probe result exists; use a new name")
result = {"mode": "LIVE_DIAGNOSTIC_NOT_ACCURACY", "created_at": now(),
          "binary_sha256": sha((STATE / "hwopsd").read_bytes()), "rows": []}
failed = []
with httpx.Client(base_url="http://127.0.0.1:18080", timeout=90,
                  headers={"Authorization": "Bearer " + (STATE / "api-token").read_text().strip()}) as client:
    for id in args.ids:
        case = cases[id]
        conversation = call(client, "POST", "/v1/conversations", {})
        pending = call(client, "POST", f"/v1/conversations/{conversation['id']}/messages",
                       {"text": case["question"]})
        deadline = time.monotonic() + 90
        while True:
            response = call(client, "GET", "/v1/responses/" + pending["id"])
            if response["status"] not in {"RUNNING", "QUEUED"}:
                break
            if time.monotonic() > deadline:
                raise TimeoutError("answer deadline")
            time.sleep(.1)
        result["rows"].append({"id": id, "question": case["question"], "result": response})
        save(path, result)
        print(id, response["status"], response.get("gaps"), flush=True)
        if response.get("data_mode") != "LIVE" or response["status"] != "ANSWERED" or response.get("gaps"):
            failed.append(id)
if failed:
    raise SystemExit("UNANSWERED_OR_GAPS: " + ", ".join(failed))
