"""Normalize browser-captured CCE tables without losing row-spanned categories."""

import argparse
import hashlib
import json
from pathlib import Path


SECTIONS = {
    1: "集群/节点",
    2: "网络",
    3: "容器",
    4: "负载均衡",
    5: "日志",
    6: "监控",
    7: "云硬盘",
    8: "插件",
}


def expand_rows(rows):
    spans = {}
    expanded = []
    for row in rows:
        values = {}
        for column, (value, remaining) in list(spans.items()):
            values[column] = value
            if remaining == 1:
                del spans[column]
            else:
                spans[column] = (value, remaining - 1)
        column = 0
        for cell in row:
            while column in values:
                column += 1
            for _ in range(cell["colspan"]):
                values[column] = cell["text"]
                if cell["rowspan"] > 1:
                    spans[column] = (cell["text"], cell["rowspan"] - 1)
                column += 1
        expanded.append([values[c] for c in sorted(values)])
    return expanded


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--source", default=".cache/rag/source/cce-browser-source.json")
    parser.add_argument("--output", default="testdata/rag/cce")
    args = parser.parse_args()
    raw = Path(args.source).read_bytes()
    source = json.loads(raw)
    output = Path(args.output)
    output.mkdir(parents=True, exist_ok=True)
    evidence = []
    markdown = [
        "# 华为云 CCE 高危操作一览",
        "",
        f"来源：{source['url']}",
        "文档更新时间：2026-03-30 GMT+08:00",
        f"抓取时间：{source['fetched_at']}",
        "以下内容按原表格逐行整理；只包含指定页面，不包含链接指向的补充文档。",
        "",
    ]
    for table in source["tables"]:
        table_id = int(table["id"])
        rows = expand_rows(table["rows"])
        headers = rows[0]
        for row_number, row in enumerate(rows[1:], 1):
            if len(row) != len(headers):
                raise ValueError(f"incomplete table {table_id}, row {row_number}")
            fields = dict(zip(headers, row))
            evidence_id = f"cce-t{table_id}-r{row_number:02}"
            title = f"{SECTIONS[table_id]} / {fields.get('分类', SECTIONS[table_id])}"
            text = "\n".join(f"{key}：{value}" for key, value in fields.items())
            item = {
                "evidence_id": evidence_id,
                "section": SECTIONS[table_id],
                "category": fields.get("分类", SECTIONS[table_id]),
                "table": table_id,
                "row": row_number,
                "source": source["url"],
                "fields": fields,
                "text": text,
                "sha256": hashlib.sha256(text.encode()).hexdigest(),
            }
            evidence.append(item)
            markdown.extend([f"## {evidence_id} {title}", "", text, ""])
    (output / "source-tables.json").write_bytes(raw)
    (output / "evidence.json").write_text(
        json.dumps(evidence, ensure_ascii=False, indent=2) + "\n"
    )
    (output / "document.md").write_text("\n".join(markdown))
    manifest = {
        "source": source["url"],
        "title": source["title"],
        "fetched_at": source["fetched_at"],
        "document_updated_at": "2026-03-30 GMT+08:00",
        "source_sha256": hashlib.sha256(raw).hexdigest(),
        "evidence_count": len(evidence),
        "table_count": len(source["tables"]),
        "extraction": "Visible browser DOM table cells with rowspan/colspan expansion",
    }
    (output / "source-manifest.json").write_text(
        json.dumps(manifest, ensure_ascii=False, indent=2) + "\n"
    )
    print(json.dumps(manifest, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
