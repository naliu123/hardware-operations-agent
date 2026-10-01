"""Direct generation probes for branch selection; no evaluation labels supplied."""
import json
import re
import sys

sys.path.insert(0, "scripts/rag")
import httpx
import evaluate_manual as ev

rows = json.loads((ev.REPORT / "r04b-selection-probe.json").read_text())["rows"]
seed = next(r for r in rows if r["id"] == "cce-manual-097")
ing = json.loads((ev.ROOT / "reports/rag-cce-manual-20260920/ingestion.json").read_text())
paths = {f: "/v1/knowledge/revisions/" + p["revision_id"] + "/fragments/" + f
         for d in ing["documents"].values() for p in d["parts"] for f in p["fragment_ids"]}
docs = []
with httpx.Client(base_url="http://127.0.0.1:18080", headers={
    "Authorization": "Bearer " + (ev.STATE / "api-token").read_text().strip()}) as c:
    for fid in seed["result"]["retrieved_fragment_ids"]:
        d = c.get(paths[fid]).json()
        docs.append({"id": fid, "content": d["content"], "meta_data": {
            k: d[k] for k in ["title", "section", "source", "revision_id", "document_context"]
        }})
prompt = re.search(r"schema.SystemMessage\(`(.*?)`\)", (ev.ROOT / "internal/einoflow/qa.go").read_text(), re.S)[1]
extra = """
操作类问题先区分用户目的和原文各分支目的：保留/再次使用资源、解除绑定、永久销毁是不同目的。
摘录范围必须服从操作目的。对于同一段中混排的不同目的流程，只摘录当前目的对应的句子，其他分支从引文中省略，不能以“完整原文”为由附带。
用户要继续使用原资源/数据时，答案与引文都不得包含清除原数据或删除底层资源的步骤；保留相关对象状态、前置条件和重新关联方式。
"""
out = []
with httpx.Client(timeout=90) as c:
    for name, actual in [
        ("purpose-filter", prompt + extra),
        ("purpose-filter-extract", prompt.replace("完整摘录支撑结论的原文段落或相关列表", "摘录支撑结论的全部相关原文句子或列表项") + extra),
    ]:
        body = {"model": "deepseek-flash", "temperature": 0, "thinking": {"type": "disabled"},
                "messages": [{"role": "system", "content": actual},
                             {"role": "user", "content": json.dumps({
                                 "question": seed["question"], "documents": docs}, ensure_ascii=False)}]}
        response = c.post("https://api.deepseek.com/chat/completions",
                          headers={"Authorization": "Bearer " + ev.credential()}, json=body)
        response.raise_for_status()
        raw = response.json()
        out.append({"name": name, "mode": "LIVE_DIRECT_GENERATION_NOT_END_TO_END",
                    "request": body, "response": raw})
        ev.save(ev.REPORT / "operation-scope-probe.json", out)
        print(name, raw["choices"][0]["message"]["content"], flush=True)
