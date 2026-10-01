"""Small direct API experiment; never an end-to-end quality score."""
import json
import re
import sys
from pathlib import Path

sys.path.insert(0, "scripts/rag")
import httpx
import evaluate_manual as ev

seed = json.loads((ev.REPORT / "r04-selection-probe.json").read_text())["rows"][0]
ing = json.loads((ev.ROOT / "reports/rag-cce-manual-20260920/ingestion.json").read_text())
paths = {f: "/v1/knowledge/revisions/" + p["revision_id"] + "/fragments/" + f
         for d in ing["documents"].values() for p in d["parts"] for f in p["fragment_ids"]}
docs = []
with httpx.Client(base_url="http://127.0.0.1:18080", headers={
    "Authorization": "Bearer " + (ev.STATE / "api-token").read_text().strip()}) as c:
    for fid in seed["result"]["evidence_selection"]["candidate_fragment_ids"]:
        d = c.get(paths[fid]).json()
        text = d["content"]
        if len(text) > 1600:
            text = text[:1000] + "\n[中间省略，仅作选择预览]\n" + text[-600:]
        docs.append({"id": fid, "content": text, "meta_data": {
            k: d[k] for k in ["title", "section", "source", "revision_id", "document_context"]
        }})
current = re.search(r"schema.SystemMessage\(fmt.Sprintf\(`(.*?)`, limit\)\)",
                    (ev.ROOT / "internal/einoflow/selection.go").read_text(), re.S)[1].replace("%d", "5")
prior = json.loads((ev.REPORT / "rerank-probe.json").read_text())[0]["prompt"]
prior = prior.replace('{"selected":[候选序号],"reason":"简要选取依据"}',
                      '{"selected_fragment_ids":["实际候选ID"]}').replace("候选序号", "候选ID")
out = []
with httpx.Client(timeout=90) as c:
    for name, prompt, temp in [("current-default", current, None), ("current-zero", current, 0),
                               ("prior-zero", prior, 0)]:
        body = {"model": "deepseek-flash", "thinking": {"type": "disabled"},
                "messages": [{"role": "system", "content": prompt},
                             {"role": "user", "content": json.dumps({
                                 "question": seed["question"], "documents": docs}, ensure_ascii=False)}]}
        if temp is not None:
            body["temperature"] = temp
        response = c.post("https://api.deepseek.com/chat/completions",
                          headers={"Authorization": "Bearer " + ev.credential()}, json=body)
        response.raise_for_status()
        raw = response.json()
        out.append({"name": name, "mode": "LIVE_DIRECT_SELECTION_NOT_END_TO_END",
                    "request": body, "response": raw})
        ev.save(ev.REPORT / "selection-temperature-probe.json", out)
        ids = json.loads(raw["choices"][0]["message"]["content"])["selected_fragment_ids"]
        print(name, [(d["meta_data"]["title"], d["meta_data"]["section"])
                     for d in docs if d["id"] in ids], flush=True)
