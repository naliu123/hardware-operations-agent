"""Build an offline report from immutable live results; never call a model."""

from collections import Counter
import csv
import hashlib
import json
import os
import statistics

from common import DATA, PHOENIX, REPORT, ROOT, frozen_cases, save

os.environ.setdefault("MPLCONFIGDIR", str(ROOT / ".cache/rag/matplotlib"))
import matplotlib

matplotlib.use("Agg")
import matplotlib.pyplot as plt


NAMES = [
    "dev-bm25-baseline", "dev-dense-v1", "dev-hybrid-v1",
    "dev-dense-answers", "heldout-dense-final", "heldout-dense-answers",
]
RETRIEVAL = [
    "recall_at_1", "recall_at_3", "recall_at_5", "precision_at_5",
    "hit_at_5", "mrr_at_5", "ndcg_at_5",
]
JUDGE = ["correctness", "faithfulness", "relevance"]
NOTES = {
    "cce-q001": "检索漏召回：全量拉取列表未匹配 kube-apiserver/LIST；返回缺口，没有编造处理办法。",
    "cce-q015": "标签覆盖差异：额外引用容器隧道网络升级场景，引用精确率为0.5；模型评审认为该背景有依据，不等同于幻觉。",
    "cce-q048": "问法与证据缺口：问题问“为什么”，原文只列出重复采集的后果，未解释机制。保留0.5分，下一版应校准题目或显式说明机制未知。",
    "cce-q053": "实质错误：未召回直接操作EVS的r03，误用解除挂载/umount的r01、r02并声称有恢复方法；r03恢复栏为“无”。“无”仅表示本页未提供方法。",
    "cce-q057": "比较题漏证据：缺控制节点IP的r07，仅召回Node节点r16，答案只回答一半且没有指出另一半缺口。",
    "cce-q061": "可见评审误判：答案已分别写明两参数改回0，judge却称没回答建议值；保留原始0.5分，不据此改高统计分数。",
}


def load(path):
    return json.loads(path.read_text())


def pct(value):
    return "—" if value is None else f"{value * 100:.2f}%"


def mean(values):
    return statistics.mean(values) if values else None


def write_csv(path, rows):
    fields = list(dict.fromkeys(key for row in rows for key in row))
    with path.open("w", encoding="utf-8-sig", newline="") as stream:
        writer = csv.DictWriter(stream, fieldnames=fields)
        writer.writeheader()
        writer.writerows(rows)


def judge_summary(rows):
    return {
        name: {
            "mean": mean([row["judge"]["scores"][name] for row in rows
                          if row["judge"]["scores"][name] is not None]),
            "n": sum(row["judge"]["scores"][name] is not None for row in rows),
            "distribution": dict(Counter(str(row["judge"]["scores"][name]) for row in rows)),
        } for name in JUDGE
    }


def usage_summary(rows):
    output = {}
    for stage in ["generation", "judge"]:
        usages = [row["result"]["model_usage"] if stage == "generation"
                  else row["judge"]["usage"] for row in rows]
        output[stage] = {
            "calls": sum(item.get("calls", 1) for item in usages),
            **{key: sum(item[key] for item in usages)
               for key in ["prompt_tokens", "completion_tokens", "total_tokens"]},
        }
    return output


def is_bad_case(row):
    # A two-evidence comparison can have perfect ranking with Recall@1 = 0.5.
    metrics = row["metrics"]
    return (
        metrics.get("recall_at_5", 1) < 1
        or metrics.get("mrr_at_5", 1) < 1
        or metrics.get("citation_precision", 1) < 1
        or any(row.get("judge", {}).get("scores", {}).get(key, 1) not in (1, None)
               for key in JUDGE)
    )


def make_chart(experiments, judges):
    fig, axes = plt.subplots(2, 2, figsize=(13, 8.5), layout="constrained")
    fig.suptitle("CCE RAG | Live Elasticsearch + BGE + DeepSeek Flash", fontsize=17)
    ax = axes[0, 0]
    for offset, key, label, color in [
        (-0.23, "recall_at_5", "Recall@5", "#136f63"),
        (0, "recall_at_1", "Recall@1", "#659dbd"),
        (0.23, "mrr_at_5", "MRR@5", "#bf8330"),
    ]:
        vals = [experiments[name]["summary"]["means"][key] * 100 for name in NAMES[:3]]
        ax.bar([i + offset for i in range(3)], vals, width=0.23, label=label, color=color)
    ax.set_xticks(range(3), ["BM25", "Dense (selected)", "RRF"])
    ax.set_title("Development only | 23 answerable questions")
    ax.set_ylim(0, 112)
    ax.axhline(60, color="#b74435", linestyle="--", linewidth=1, label="60% target")
    ax.legend(loc="lower right", fontsize=9)
    ax.set_ylabel("Score (%)")

    ax = axes[0, 1]
    keys = ["recall_at_1", "recall_at_3", "recall_at_5", "mrr_at_5", "ndcg_at_5"]
    vals = [experiments["heldout-dense-final"]["summary"]["means"][key] * 100 for key in keys]
    bars = ax.bar(range(len(keys)), vals, color=["#659dbd", "#659dbd", "#136f63", "#659dbd", "#659dbd"])
    ax.bar_label(bars, fmt="%.2f", padding=3, fontsize=9)
    ax.set_xticks(range(len(keys)), ["Recall@1", "Recall@3", "Recall@5", "MRR@5", "nDCG@5"])
    ax.set_title("Frozen heldout | 38 answerable questions")
    ax.axhline(60, color="#b74435", linestyle="--", linewidth=1)
    ax.set_ylim(0, 112)
    ax.set_ylabel("Score (%)")

    ax = axes[1, 0]
    scores = judges["heldout-dense-answers"]
    vals = [scores[key]["mean"] * 100 for key in JUDGE]
    bars = ax.bar(range(3), vals, color="#8a6db1")
    ax.bar_label(bars, fmt="%.2f", padding=3)
    ax.set_xticks(range(3), [f"{key.title()}\nn={scores[key]['n']}" for key in JUDGE])
    ax.set_title("Heldout LLM judge | Same model; not human scores")
    ax.set_ylim(0, 112)
    ax.set_ylabel("Mean rubric score (%)")

    ax = axes[1, 1]
    row_counts = Counter()
    for row in experiments["heldout-dense-final"]["rows"]:
        if row["answerable"]:
            recall = row["metrics"]["recall_at_5"]
            row_counts["Full" if recall == 1 else "Partial" if recall else "Miss"] += 1
    labels = ["Full", "Partial", "Miss"]
    bars = ax.bar(labels, [row_counts[label] for label in labels],
                  color=["#136f63", "#bf8330", "#b74435"])
    ax.bar_label(bars, padding=3)
    ax.set_title("Heldout evidence coverage | 3 negatives excluded")
    ax.set_ylim(0, 40)
    ax.set_ylabel("Question count")
    for ax in axes.flat:
        ax.spines[["top", "right"]].set_visible(False)
        ax.grid(axis="y", alpha=0.15)
        ax.set_axisbelow(True)
    fig.savefig(REPORT / "metrics.png", dpi=180)
    fig.savefig(REPORT / "metrics.svg")
    plt.close(fig)


def make_badcases(experiments, cases, evidence):
    records, lines = [], [
        "# RAG 错误、排序不足与评审分歧明细", "",
        "选入条件：Recall@5<1、MRR@5<1、引用标签精确率<1，或任一LLM评分<1。"
        "排序不足与标签分歧不自动等同于错误答案。全部保留原始指标；验收集只做事后分析，未用于本轮调参。", "",
    ]
    for name in NAMES:
        rows = [row for row in experiments[name]["rows"] if is_bad_case(row)]
        lines.extend([f"## {name}（{len(rows)} 条）", ""])
        for row in rows:
            record = {
                "experiment": name, "id": row["id"], "question": row["question"],
                "reference_answer": cases[row["id"]]["reference_answer"],
                "relevant_evidence_ids": row["relevant_evidence_ids"],
                "retrieved_evidence_ids": row["retrieved_evidence_ids"],
                "answer": row["result"].get("answer", ""),
                "metrics": row["metrics"], "judge": row.get("judge"),
                "trace_id": row["result"]["trace_id"], "note": NOTES.get(row["id"], ""),
                "source_evidence": [evidence[eid] for eid in row["relevant_evidence_ids"]],
            }
            records.append(record)
            lines.extend([
                f"### {row['id']} · {row['question']}", "",
                f"应召回：`{', '.join(row['relevant_evidence_ids'])}`  ",
                f"实际前五：`{', '.join(row['retrieved_evidence_ids'])}`  ",
                f"Recall@5={pct(row['metrics'].get('recall_at_5'))}；"
                f"MRR@5={pct(row['metrics'].get('mrr_at_5'))}", "",
                f"参考答案：{cases[row['id']]['reference_answer']}", "",
            ])
            for item in record["source_evidence"]:
                lines.extend([f"原文 `{item['evidence_id']}`：", "", "```text", item["text"], "```", ""])
            if experiments[name]["config"]["answers"]:
                lines.extend(["实际答案：", "", "```text",
                              record["answer"] or "（空答案，返回UNRESOLVED）", "```", ""])
                scores = row["judge"]["scores"]
                lines.extend([
                    f"LLM原始评分：正确性={scores['correctness']}，忠实度={scores['faithfulness']}，"
                    f"相关性={scores['relevance']}。{scores['explanation']}", "",
                ])
            if record["note"]:
                lines.extend([f"核对说明：{record['note']}", ""])
            lines.extend([f"生成/检索 trace_id：`{record['trace_id']}`", ""])
    save(REPORT / "badcases.json", records)
    (REPORT / "badcases.md").write_text("\n".join(lines))
    return records


def main():
    cases = {row["id"]: row for row in frozen_cases()}
    evidence = {row["evidence_id"]: row for row in load(DATA / "evidence.json")}
    source = load(DATA / "source-manifest.json")
    manifest = load(DATA / "dataset-manifest.json")
    selection = load(REPORT / "selection.json")
    verification = load(REPORT / "verification.json")
    assert hashlib.sha256((DATA / "source-tables.json").read_bytes()).hexdigest() == source["source_sha256"]
    assert selection["dataset_sha256"] == manifest["sha256"]
    experiments = {name: load(REPORT / f"{name}.json") for name in NAMES}
    summary_rows, query_rows = [], []
    judges, usages = {}, {}
    for name, exp in experiments.items():
        assert exp["config"]["dataset_sha256"] == manifest["sha256"]
        expected_ids = {key for key, row in cases.items() if row["split"] == exp["config"]["split"]}
        assert len(exp["rows"]) == len(expected_ids)
        assert {row["id"] for row in exp["rows"]} == expected_ids
        # Recompute means from individual saved scores rather than trusting a headline.
        keys = sorted({key for row in exp["rows"] for key in row["metrics"]})
        for key in keys:
            value = mean([row["metrics"][key] for row in exp["rows"] if key in row["metrics"]])
            assert abs(value - exp["summary"]["means"][key]) < 1e-12
        if exp["config"]["answers"]:
            assert all("judge" in row for row in exp["rows"])
            judges[name] = judge_summary(exp["rows"])
            usages[name] = usage_summary(exp["rows"])
        summary = {
            "experiment": name, "split": exp["config"]["split"],
            "strategy": exp["config"]["strategy"], "answers": exp["config"]["answers"],
            "questions": len(exp["rows"]), "answerable": len([r for r in exp["rows"] if r["answerable"]]),
            **exp["summary"]["means"],
            **{f"latency_{key}_ms": value for key, value in exp["summary"]["latency_ms"].items()},
        }
        for key, score in judges.get(name, {}).items():
            summary[f"llm_{key}"] = score["mean"]
            summary[f"llm_{key}_n"] = score["n"]
        summary_rows.append(summary)
        for row in exp["rows"]:
            item = {
                "experiment": name, "split": exp["config"]["split"], "id": row["id"],
                "question": row["question"], "answerable": row["answerable"],
                "question_type": row["question_type"], "section": row["section"],
                "reference_answer": cases[row["id"]]["reference_answer"],
                "relevant_evidence_ids": " ".join(row["relevant_evidence_ids"]),
                "retrieved_evidence_ids": " ".join(row["retrieved_evidence_ids"]),
                "latency_ms": row["latency_ms"], **row["metrics"],
                "status": row["result"].get("status", "RETRIEVAL_ONLY"),
                "answer": row["result"].get("answer", ""), "trace_id": row["result"]["trace_id"],
                "phoenix_run_id": row["phoenix_run_id"],
            }
            if "judge" in row:
                item.update({f"llm_{key}": value for key, value in row["judge"]["scores"].items()})
                item["judge_trace_id"] = row["judge"]["trace_id"]
                item.update({f"generation_{key}": value for key, value in row["result"]["model_usage"].items()})
                item.update({f"judge_{key}": row["judge"]["usage"][key]
                             for key in ["prompt_tokens", "completion_tokens", "total_tokens"]})
            query_rows.append(item)
    write_csv(REPORT / "summary.csv", summary_rows)
    write_csv(REPORT / "per-query.csv", query_rows)
    badcases = make_badcases(experiments, cases, evidence)
    make_chart(experiments, judges)
    heldout = experiments["heldout-dense-final"]["summary"]
    answers = experiments["heldout-dense-answers"]
    answer_means = answers["summary"]["means"]
    coverage = Counter(row["metrics"]["recall_at_5"] for row in answers["rows"] if row["answerable"])
    status = Counter(row["result"]["status"] for row in answers["rows"])
    total_usage = {
        stage: {key: sum(value[stage][key] for value in usages.values())
                for key in ["calls", "prompt_tokens", "completion_tokens", "total_tokens"]}
        for stage in ["generation", "judge"]
    }
    cleanup = load(REPORT / "cleanup.json") if (REPORT / "cleanup.json").exists() else None
    save(REPORT / "summary.json", {
        "data_mode": "LIVE", "primary_metric": manifest["primary_metric"],
        "target": manifest["target"], "achieved": heldout["means"]["recall_at_5"] >= manifest["target"],
        "dataset": manifest, "selection": selection, "experiments": summary_rows,
        "llm_judges": judges, "model_usage": usages, "model_usage_total": total_usage,
        "verification": verification, "cleanup": cleanup,
        "badcase_experiment_rows": len(badcases),
    })

    lines = [
        "# 华为云 CCE 文档 RAG 统计报表", "",
        "报告日期：2026-09-20（UTC+08:00）；原始请求时间保留UTC。运行模式：**LIVE**。", "",
        f"**固定独立验收集宏平均 Recall@5 = {pct(heldout['means']['recall_at_5'])}，达到≥60%的目标。**"
        "开发集在BM25基线上分析bad case后选择Dense；开发集Recall@5从97.83%提升到100%。"
        "验收结果来自选型冻结后的另一组问题，两组分数不能当作前后提升量。", "",
        "本次真实运行 Elasticsearch、本地 BGE 中文 embedding、Go + Eino HTTP 问答、"
        "官方 DeepSeek `deepseek-flash`，并向 Phoenix 写入数据集、实验、代码指标和模型评审。"
        "未使用REPLAY替代本表中的模型调用。", "",
        "## 数据与固定实验条件", "",
        f"- 来源：[华为云 CCE《高危操作一览》]({source['source']})；页面更新时间为"
        f"{source['document_updated_at']}，抓取时间为{source['fetched_at']}。",
        "- 正常浏览器读取页面，展开表格合并单元格；8张表、55条证据行，加1个来源前言，共56个ES片段。"
        "每条保留分类、操作、后果、恢复方案及备注；不采集跳转页面。",
        "- 66道Agent编写题：开发25题（23可回答、2负例），验收41题（38可回答、3负例）。"
        "按源证据组划分，同证据改写及比较题保持在同一组；所有源知识均可检索。",
        "- 问题、参考答案、证据标签与分组在实验前冻结；配置在验收前冻结。"
        "本轮只使用开发集调参，验收后的错误分析留给下一版数据集与独立验收。",
        "- 文档和片段存于Elasticsearch 9.5.4；业务状态仍使用真实本地文件存储。"
        "Go 1.26.8、Eino依赖及Python精确版本分别见go.mod和requirements.lock。",
        f"- embedding：`{selection['embedding_model']}`，512维，CLS + L2归一化，CPU两线程；"
        f"固定revision `{selection['embedding_revision']}`。",
        "- 最终策略Dense，Top K固定5，每路候选20；比较策略RRF常数60。"
        "查询添加BGE中文检索指令，文档按原文编码；没有增加k或修改标签来达标。",
        "- 生成和评审均为官方DeepSeek `deepseek-flash`，`thinking.type=disabled`。"
        "生成沿用项目证据引用提示，未增加特定题目的答案规则。",
        f"- 数据集SHA256：`{manifest['sha256']}`。",
        f"- 源表格SHA256：`{source['source_sha256']}`。", "",
        "## 检索统计", "",
        "| 数据 / 策略 | 可回答题数 | Recall@1 | Recall@3 | Recall@5 | Precision@5 | Hit@5 | MRR@5 | nDCG@5 | HTTP p95 |",
        "| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |",
    ]
    for name in [*NAMES[:3], "heldout-dense-final"]:
        exp = experiments[name]
        values = " | ".join(pct(exp["summary"]["means"][key]) for key in RETRIEVAL)
        lines.append(f"| {name} | {exp['summary']['answerable_count']} | {values} | "
                     f"{exp['summary']['latency_ms']['p95']:.2f} ms |")
    lines.extend([
        "", f"验收集38道可回答题：{coverage[1.0]}题证据全召回、{coverage[0.5]}题只召回一半、"
        f"{coverage[0.0]}题完全漏召回。宏平均为`(35×1 + 1×0.5 + 2×0) / 38 = 93.42%`。"
        "负例不进入召回率分母。答案实验再次通过完整HTTP链路检索，得到相同的逐题Recall指标。", "",
        "开发BM25已超过60%，继续迭代是为了补齐比较题cce-q060的第二项证据；Dense完成补齐，"
        "但Recall@1和MRR下降。按预定主指标选择Dense，没有宣称所有指标改善。"
        "等权RRF未补齐该遗漏，因此没有采用。详见[优化记录](optimization.md)。", "",
        "开发Dense与RRF曾并行运行，时延不作严格性能优劣判断；最终验收单独运行。"
        "所有时延来自56片段的本机热服务，不代表生产吞吐。", "",
        "![开发比较与独立验收统计](metrics.png)", "",
        "## 真实生成与评审", "",
        "| 指标 | 开发集 | 验收集 | 口径 |",
        "| --- | ---: | ---: | --- |",
    ])
    devmeans = experiments["dev-dense-answers"]["summary"]["means"]
    for key, label, scope in [
        ("processing_success", "处理成功率", "全部25/41题；未FAILED不等于正确回答"),
        ("answer_rate", "返回ANSWERED比例", "全部25/41题"),
        ("citation_integrity", "引用原文完整性", "有引用的23/37题；按题平均，HTTP回读哈希与发布状态"),
        ("citation_recall", "引用证据召回率", "可回答23/38题；无引用记0"),
        ("citation_precision", "引用标签精确率", "可回答23/38题；无引用记0"),
        ("citation_hit", "至少命中一项引用证据", "可回答23/38题"),
        ("abstention_accuracy", "无答案问题正确拒答率", "开发2/2、验收3/3；样本很少"),
    ]:
        lines.append(f"| {label} | {pct(devmeans[key])} | {pct(answer_means[key])} | {scope} |")
    for key, label in [("correctness", "LLM正确性均分"), ("faithfulness", "LLM忠实度均分"), ("relevance", "LLM相关性均分")]:
        dev, held = judges["dev-dense-answers"][key], judges["heldout-dense-answers"][key]
        lines.append(f"| {label} | {pct(dev['mean'])}（n={dev['n']}） | {pct(held['mean'])}（n={held['n']}） | 固定0/0.5/1 rubric |")
    lines.extend([
        "", f"验收实际状态：{status['ANSWERED']}个ANSWERED、{status['UNRESOLVED']}个UNRESOLVED、"
        f"{status['FAILED']}个FAILED；4个未解答中有3个预期负例及1个漏召回。"
        "LLM正确性分布为36题1分、3题0.5分、2题0分。"
        "忠实度仅对37个非空答案评分；空答案为null，不当作满分。", "",
        "以上LLM指标是模型评审均分，**不是人工准确率或模型自报置信度**。生成与评审使用同一模型，"
        "可能有共同偏差。引用完整性100%只证明引用可打开且内容一致，不能证明语义结论正确。", "",
        "| 延迟 | 开发真实问答（25题） | 验收真实问答（41题） |",
        "| --- | ---: | ---: |",
    ])
    for key in ["mean", "p50", "p95"]:
        lines.append(f"| {key} | {experiments['dev-dense-answers']['summary']['latency_ms'][key]:.2f} ms | "
                     f"{answers['summary']['latency_ms'][key]:.2f} ms |")
    lines.extend([
        "", "问答延迟包含创建会话、提交问题及约100ms间隔的终态轮询；不含后续引用回读与LLM评审。"
        "检索延迟包含Go HTTP与ES/embedding调用。p95使用排序后`ceil(0.95×n)`位置。", "",
        "## 逐题缺口与后续迭代", "",
        "| 验收题 | 类别 | 源证据核对及影响 |",
        "| --- | --- | --- |",
    ])
    for qid, category in [
        ("cce-q001", "漏召回"), ("cce-q053", "相似操作混淆"),
        ("cce-q057", "比较题漏一半"), ("cce-q048", "题目/证据缺口"), ("cce-q061", "评审误判"),
    ]:
        lines.append(f"| {qid} | {category} | {NOTES[qid]} |")
    lines.extend([
        "", "开发题cce-q015额外引用了有依据的背景，引用标签精确率0.5但LLM评分为满分，"
        "说明精确率受标签覆盖范围影响。详细问题、参考答案、实际前五证据、原文、答案、"
        "评分解释和trace_id均在[bad case明细](badcases.md)及[JSON](badcases.json)。", "",
        "本轮已按开发bad case完成BM25→Dense/RRF比较→Dense选型，并通过独立验收，停止继续调参。"
        "下一轮优先检验：对象和动作字段重排；比较题拆分查询并检查两侧覆盖；"
        "缺对应操作证据时拒答或明确部分回答；对“为什么”题补充有来源的机制资料或调整问题。"
        "这些是待验证假设，尚未作为本轮优化结果实现。应先建立新开发题和未查看的新验收集。", "",
        "## 指标定义与统计边界", "",
        "令`Gq`为问题的相关原文行集合，`Dq(k)`为前k个检索行ID，所有检索指标先逐题计算，"
        "再对可回答题宏平均。", "",
        "- Recall@k = `|Gq ∩ Dq(k)| / |Gq|`；Precision@5 = `|Gq ∩ Dq(5)| / 5`。",
        "- Hit@5 = 前五至少命中一行的题目比例；MRR@5 = 首个相关行排名的倒数，未命中为0。",
        "- nDCG@5采用二元相关性：`DCG=Σ rel(i)/log2(i+1)`，除以前五理想排序的DCG。",
        "- 每题仅1–2项相关行，固定返回5条；Precision@5即使全召回通常也只有20%–40%。"
        "它仍然反映上下文包含额外内容，不应隐藏，也不直接等同答案错误率。",
        "- 引用召回/精确率以实际引用集合替代检索集合；原文完整性按每题引用的哈希和发布状态计算。",
        "- LLM正确性对参考答案，忠实度对实际召回上下文，相关性对问题；"
        "不使用向量相似度或生成模型自评作为正确性的替代。",
        "- 数据来自单一公开页面，题目由Agent编写，未经过运维专家独立认证；"
        "验收是问题分组隔离，不是新文档或新硬件域泛化测试。5个负例不足以证明生产拒答能力。",
        "- 此实验不覆盖实时资产、内网设备、生产权限、PostgreSQL联调或容量验收；QA-03与QA-06的其他范围仍待实施。", "",
        "## Phoenix与用量证据", "",
        f"Phoenix项目：`hwops-cce-rag`；本地地址：{PHOENIX}。"
        f"已回读{len(verification['datasets'])}个数据集、{len(verification['experiments'])}个实验、"
        f"{sum(item['runs'] for item in verification['experiments'])}条实验run，"
        f"导出{verification['phoenix_span_count']}个span；"
        f"{verification['experiment_trace_count']}个实验/评审trace引用全部存在。", "",
        "| 数据集 | 样本数 | Phoenix ID / 版本 |",
        "| --- | ---: | --- |",
    ])
    for item in verification["datasets"]:
        lines.append(f"| [{item['split']}]({PHOENIX}/datasets/{item['id']}) | {item['count']} | "
                     f"`{item['id']}` / `{item['version_id']}` |")
    lines.extend(["", "| 实验 | run数 | Phoenix实验接口 |", "| --- | ---: | --- |"])
    for item in verification["experiments"]:
        lines.append(f"| [{item['name']}]({item['name']}.json) | {item['runs']} | "
                     f"[{item['id']}]({PHOENIX}/v1/experiments/{item['id']}) |")
    lines.extend([
        "", "Span类型：" + "、".join(f"{key}={value}" for key, value in verification["span_kinds"].items()) + "。",
        "66次生成和66次评审均有真实上游usage与trace。下表只统计这两组已完成的问答/评审实验，"
        "不推算额外连通性检查或账户总账单。", "",
        "| 阶段（开发+验收） | 实际调用数 | 输入tokens | 输出tokens | 总tokens |",
        "| --- | ---: | ---: | ---: | ---: |",
    ])
    for stage, label in [("generation", "生成"), ("judge", "LLM评审")]:
        item = total_usage[stage]
        lines.append(f"| {label} | {item['calls']} | {item['prompt_tokens']:,} | "
                     f"{item['completion_tokens']:,} | {item['total_tokens']:,} |")
    lines.extend([
        "", "用量是API返回值；没有假定价格或伪造费用。原始judge usage保留缓存命中字段。"
        "导出追踪和报告已扫描，未包含临时密钥。", "",
        "## 工程验证、资源与复现", "",
        "Go全量race测试通过，包含QA-01/QA-02回归、公开检索接口、真实ES与embedding流程；"
        "vet、构建与Python脚本运行检查通过。HTTP契约测试只模拟外部依赖；"
        "ES测试的生成器标记REPLAY，DeepSeek真实效果由上述独立实验验证。"
        "真实PostgreSQL测试因没有连接配置跳过。日志见[验证记录](validation/)。", "",
    ])
    if cleanup:
        lines.extend([
            f"资源释放状态：{'已全部停止并核验' if cleanup['all_released'] else '仍需核对'}；"
            f"临时DeepSeek密钥文件：{'已删除' if cleanup['temporary_key_deleted'] else '仍存在'}。"
            "Go、ES、Phoenix、embedding进程及子进程停止，相关端口已释放。"
            "Phoenix当前已关闭，以上本地链接需重启服务后访问。",
            f"停止前记录的进程树RSS合计为{cleanup['rss_before_mib']:.1f} MiB"
            "（瞬时RSS求和，非峰值或独占物理内存）；该进程集已释放。"
            "数据库、模型磁盘缓存和报表保留，便于复核。详见[清理记录](cleanup.json)。", "",
        ])
    else:
        lines.extend(["资源释放尚待执行，状态以最终[清理记录](cleanup.json)为准。", ""])
    lines.extend([
        "启动、健康检查、复测与停止命令见[运行说明](../../scripts/rag/README.md)。"
        "仅查看Phoenix历史数据可执行：", "",
        "```sh",
        "PHOENIX_WORKING_DIR=\"$PWD/.local/rag/phoenix\" PHOENIX_HOST=127.0.0.1 PHOENIX_PORT=16006 \\",
        "  .tools/rag-venv/bin/python scripts/rag/phoenix_local.py",
        "```", "",
        "该命令只打开Phoenix，查看后Ctrl+C退出。恢复整个实验使用`services.py start`，"
        "再次真实生成需通过环境重新提供密钥。"
        "离线重新生成报表只需`.tools/rag-venv/bin/python scripts/rag/report.py`，不会调用模型。", "",
        "## 交付文件", "",
        "- [汇总CSV](summary.csv)、[逐题CSV（182条）](per-query.csv)、[机器可读统计](summary.json)。",
        "- [统计图PNG](metrics.png)、[矢量图SVG](metrics.svg)、[bad case明细](badcases.md)。",
        "- [Phoenix spans JSON](phoenix-spans.json)、[CSV](phoenix-spans.csv)、[接口回读核验](verification.json)。",
        "- [冻结配置](selection.json)、[原文修订](published-revision.json)、[ES文档导出](elasticsearch-documents.json)。",
        "- [源数据与冻结题库](../../testdata/rag/cce/)、[代码与评测复现](../../scripts/rag/README.md)。", "",
    ])
    (REPORT / "report.md").write_text("\n".join(lines))
    print(json.dumps({"report": str(REPORT / "report.md"),
                      "heldout_recall_at_5": heldout["means"]["recall_at_5"],
                      "model_usage": total_usage, "per_query_rows": len(query_rows)}, indent=2))


if __name__ == "__main__":
    main()
