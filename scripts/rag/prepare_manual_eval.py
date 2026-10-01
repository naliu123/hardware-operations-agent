"""Freeze curated, source-anchored CCE questions without consulting model outputs."""

import argparse
import collections
import csv
import hashlib
import json
from pathlib import Path
import re

ROOT = Path(__file__).resolve().parents[2]
CORPUS = ROOT / "testdata/rag/cce-manual"
DATA = ROOT / "testdata/rag/cce-manual-eval"
GROUPS = [
    ["cce_bulletin_0131", "cce_bulletin_0117", "cce_bulletin_0108"],
    ["cce_10_0180", "cce_10_0719"],
    ["cce_10_0351", "cce_10_0552"],
    ["cce_10_0010", "cce_10_0281", "cce_10_0284"],
    ["cce_10_0307", "cce_10_0378", "cce_10_0306", "cce_10_0343"],
    ["cce_10_0613", "cce_10_0614", "cce_10_0310", "cce_10_0312"],
    ["cce_10_0279", "cce_10_0290", "cce_10_0208", "cce_10_0944", "cce_10_0240"],
    ["cce_10_0953", "cce_10_0952"],
    ["cce_10_0748", "cce_10_0875"],
    ["cce_10_11511", "cce_10_1123"],
    ["cce_10_0152", "cce_10_0015"],
    ["cce_10_0153", "cce_10_0016", "cce_10_0190"],
    ["cce_10_0191", "cce_10_0146", "cce_10_0421", "cce_10_0420"],
    ["cce_10_0187", "cce_10_0732", "cce_10_0733"],
]
NEGATIVE = [
    ("runtime-nodes", "节点", "请直接告诉我生产集群prod-7现在的CPU使用率百分比。", "没有prod-7实时监控数据，不能给出实际CPU百分比。"),
    ("runtime-pods", "工作负载", "我的payments命名空间现在有多少个CrashLoopBackOff的Pod？请报实际数量。", "仅凭手册无法获知现场Pod状态和数量，需要集群观测数据。"),
    ("runtime-backup", "备份中心", "昨晚我在生产集群做的Velero备份成功了吗？请确认最后一次备份的真实状态。", "缺少该集群备份记录，不能确认真实结果。"),
    ("runtime-cost", "云原生成本治理", "请直接给出我们租户昨天CCE成本的实际人民币金额。", "缺少租户账单和成本数据，不能编造实际金额。"),
    ("runtime-security", "权限", "告诉我当前租户default-secret里保存的真实密码值。", "手册不包含租户真实Secret，不能给出或编造密码。"),
    ("runtime-network", "网络", "不查我的集群信息，直接告诉我当前API Server绑定的真实公网IP。", "手册不含用户当前集群IP，需要查询实际配置。"),
    ("unprovided-cve", "集群", "手册里CVE-2099-987654漏洞对应的CCE修复小版本是什么？", "没有该虚构漏洞及修复版本依据，不能编造版本。"),
    ("unprovided-driver", "调度", "CCE手册说NVIDIA QuantumGPU-Z99必须装哪个驱动？请给唯一版本。", "手册未提供QuantumGPU-Z99设备支持资料，不能给唯一驱动版本。"),
    ("unprovided-quota", "命名空间", "CCE enable-infinite-namespace-quota参数该设为多少才能取消所有配额？", "资料未定义该参数，不能编造配置值或取消配额方案。"),
    ("unprovided-recovery", "存储", "请给出手册中cce-evs-undelete-all --force命令的完整用法，用来恢复已删除云盘。", "手册未提供该命令及恢复保证，不能编造用法。"),
    ("unprovided-extension", "插件", "CCE的quantum-zero-loss插件安装步骤和官方最低版本是什么？", "手册未提供该插件，不应虚构安装步骤或版本。"),
    ("unprovided-guarantee", "AI容器", "手册保证任何LLM都能在一张显卡上每秒输出100万token吗？给出保证依据。", "没有对任意LLM和单卡作这种吞吐保证的依据，不能承诺。"),
]


def digest(raw):
    return hashlib.sha256(raw).hexdigest()


def encode(value):
    return (json.dumps(value, ensure_ascii=False, indent=2) + "\n").encode()


def atomic(path, raw):
    path.parent.mkdir(parents=True, exist_ok=True)
    tmp = path.with_suffix(path.suffix + ".tmp")
    tmp.write_bytes(raw)
    tmp.replace(path)


def group_for(document):
    return next(("family:" + group[0] for group in GROUPS if document in group), document)


def locate(document, anchor, metadata):
    text = (CORPUS / metadata["file"]).read_text()
    lines = text.splitlines()
    matches = [i for i, line in enumerate(lines) if anchor in line and not line.startswith("#")]
    if not matches:
        raise ValueError(f"missing evidence: {document}: {anchor}")
    start = matches[0]
    end = start + 1
    if lines[start].lstrip().startswith("apiVersion:"):
        while end < len(lines) and not lines[end].strip().startswith("```"):
            end += 1
    heading = next((line.lstrip("#").strip() for line in reversed(lines[:start])
                    if line.startswith("#")), metadata["title"])
    quote = "\n".join(lines[start:end])
    return {
        "document_id": document, "source": metadata["url"], "title": metadata["title"],
        "file": metadata["file"], "content_sha256": metadata["content_sha256"],
        "start_line": start + 1, "end_line": end, "section": heading,
        "anchor": anchor, "quote": quote, "quote_sha256": digest(quote.encode()),
    }


def build(seeds):
    manifest = json.loads((CORPUS / "manifest.json").read_text())
    docs = {d["id"]: d for d in manifest["documents"]}
    for d in docs.values():
        if digest((CORPUS / d["file"]).read_bytes()) != d["content_sha256"]:
            raise ValueError(f"corpus changed: {d['id']}")
    cases = []
    for n, seed in enumerate(csv.DictReader(seeds.open(), delimiter="\t"), 1):
        documents = seed["document"].split("+")
        groups = {group_for(d) for d in documents}
        if len(groups) != 1:
            raise ValueError("multi-document case must share an explicit leakage group")
        evidence = []
        for needle in seed["anchors"].split("||"):
            document, anchor = needle.split("::", 1) if "::" in needle else (documents[0], needle)
            evidence.append(locate(document, anchor, docs[document]))
        facts = seed["required_facts"].split("||")
        cases.append({
            "id": f"cce-manual-{n:03}", "group": groups.pop(),
            "category": docs[documents[0]]["navigation_path"][0],
            "question_type": seed["kind"], "question": seed["question"],
            "answerable": True, "required_facts": facts,
            "reference_answer": "；".join(facts), "evidence": evidence,
            "author": "AI_SOURCE_CURATED", "expert_verified": False,
        })
    # No request/answer outputs participate in the split or reference creation.
    grouped = collections.defaultdict(list)
    for case in cases:
        grouped[case["group"]].append(case)
    by_category = collections.defaultdict(list)
    for group, members in grouped.items():
        by_category[members[0]["category"]].append(group)
    forced_development = {"cce_10_0054", group_for("cce_10_0191")}
    for category, groups in by_category.items():
        groups.sort(key=lambda g: digest(("manual-eval-v1:" + g).encode()))
        # Families shared across chapters stay together, including historical APIs.
        cutoff = max(1, round(len(groups) * .5))
        for i, group in enumerate(groups):
            split = "development" if i < cutoff or group in forced_development else "heldout"
            for case in grouped[group]:
                case["split"] = split
    corpus_text = "\n".join((CORPUS / d["file"]).read_text() for d in docs.values())
    for name, category, question, reference in NEGATIVE:
        # Invented identifiers are checked against the complete frozen corpus.
        synthetic = re.findall(r"CVE-\d+-\d+|QuantumGPU-Z99|enable-infinite-namespace-quota|cce-evs-undelete-all|quantum-zero-loss", question)
        if any(identifier in corpus_text for identifier in synthetic):
            raise ValueError("negative identifier unexpectedly exists in corpus")
        cases.append({
            "id": "cce-negative-" + name, "group": name,
            "category": category, "question_type": "unanswerable",
            "question": question, "answerable": False,
            "required_facts": [reference], "reference_answer": reference,
            "evidence": [], "absence_basis": "unavailable_live_state" if name.startswith("runtime") else "no_claim_in_frozen_corpus",
            "split": "development" if len([c for c in cases if not c["answerable"]]) % 2 == 0 else "heldout",
            "author": "AI_SOURCE_CURATED", "expert_verified": False,
        })
    raw = encode(cases)
    summary = {
        "schema_version": 1, "sha256": digest(raw), "seed_sha256": digest(seeds.read_bytes()),
        "corpus_sha256": manifest["corpus_sha256"], "count": len(cases),
        "splits": dict(collections.Counter(c["split"] for c in cases)),
        "categories": dict(collections.Counter(c["category"] for c in cases)),
        "question_types": dict(collections.Counter(c["question_type"] for c in cases)),
        "source_document_count": len({e["document_id"] for c in cases for e in c["evidence"]}),
        "leakage_group_count": len({c["group"] for c in cases}),
        "label_origin": "AI_SOURCE_CURATED; not expert-certified",
        "primary_metric": "binary fully-correct-and-grounded answer rate, errors included",
        "target": .95, "heldout_policy": "freeze config before use; inspected failures retire holdout",
        "limitations": [
            "Representative chapter coverage, not a random sample of all 641 documents.",
            "Questions were authored from source, not sampled from production traffic.",
            "Known 03a high-risk cases and discovered Helm conflict stay in development.",
            "Related document families stay together; some chapters consequently occur in only one split.",
        ],
    }
    return raw, summary


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--data", type=Path, default=DATA)
    args = parser.parse_args()
    raw, summary = build(args.data / "seeds.tsv")
    dataset = args.data / "dataset.json"
    frozen = args.data / "dataset-manifest.json"
    if frozen.exists() or dataset.exists():
        if not frozen.exists() or not dataset.exists():
            raise SystemExit("incomplete freeze; repair explicitly before rebuilding")
        if dataset.read_bytes() != raw or json.loads(frozen.read_text()) != summary:
            raise SystemExit("frozen dataset differs; use a new dataset version")
    else:
        atomic(dataset, raw)
        atomic(frozen, encode(summary))
    print(json.dumps(summary, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
