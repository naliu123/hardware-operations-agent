import sys
from pathlib import Path
import unittest

sys.path.insert(0, str(Path(__file__).parent))

from capture_cce_manual import normalize_article, parse_navigation
from enrich_cce_assets import (
    build_image_messages, clean_embedding_text, image_content_type, parse_table,
    split_asset_fragment, validate_table_enrichment, visual_context,
)
from enrich_cce_manual import (
    source_blocks, validate_chunk_plan, validate_enrichment,
)
from ingest_cce_manual import split_document
from prepare_asset_eval import exact_data_case, image_discriminators


class CCEDocumentCaptureTest(unittest.TestCase):
    def test_navigation_separates_categories_from_documents(self):
        repeated = "".join(
            f'<li class="nav-item level3"><a href="https://support.huaweicloud.com/'
            f'intl/zh-cn/usermanual-cce/doc_{index}.html">文档{index}</a></li>'
            for index in range(100)
        )
        html = f"""
        <div id="support-nav"><ul class="side-nav">
          <li class="nav-item level1"><a href="javascript:">用户指南</a><ul>
            <li class="nav-item level2"><a href="javascript:"
              p-href="https://support.huaweicloud.com/intl/zh-cn/usermanual-cce/category.html">分类</a>
              <ul>{repeated}</ul>
            </li>
          </ul></li>
        </ul></div>
        """
        items = parse_navigation(html)
        self.assertEqual(101, len(items))
        self.assertEqual("category", items[0]["kind"])
        self.assertEqual(["分类", "文档0"], items[1]["navigation_path"])
        self.assertEqual(
            "https://support.huaweicloud.com/usermanual-cce/doc_0.html",
            items[1]["url"],
        )

    def test_article_keeps_source_hierarchy_and_body(self):
        item = {
            "title": "示例", "navigation_path": ["集群", "示例"],
            "url": "https://support.huaweicloud.com/usermanual-cce/example.html",
            "request_url": "https://support.huaweicloud.com/intl/zh-cn/usermanual-cce/example.html",
        }
        html = """
        <div class="help-content">
          <div class="updateTime">更新时间：<span class="updateInfo">2026-09-20 GMT+08:00</span></div>
          <div class="articleBoxWithoutHead"><h1>示例文档</h1>
            <div id="body1"><h2>操作</h2><p>这是足够长的正文内容，用于验证采集与规范化。</p>
              <a href="/intl/zh-cn/usermanual-cce/next.html">下一步</a></div>
          </div>
        </div>
        """
        result = normalize_article(html, item)
        self.assertIn("目录：用户指南 > 集群 > 示例", result["content"])
        self.assertIn("更新时间：2026-09-20 GMT+08:00", result["content"])
        self.assertIn("https://support.huaweicloud.com/usermanual-cce/next.html", result["content"])

    def test_large_document_is_split_on_utf8_boundaries(self):
        text = "# 文档\n\n" + "中文段落。" * 200
        parts = split_document(text, limit=200)
        self.assertGreater(len(parts), 1)
        self.assertTrue(all(len(part.encode()) <= 200 for part in parts))
        self.assertEqual("".join(text.split()), "".join("".join(parts).split()))

    def test_semantic_chunk_plan_keeps_original_table_and_image_ranges(self):
        text = """# 示例

来源：https://example.test

## 参数
参数说明。

| 名称 | 说明 |
| --- | --- |
| timeout | 超时 |

## 界面
![](https://example.test/screen.png "设置页")
单击保存。
"""
        lines, blocks = source_blocks(text, "示例")
        self.assertEqual(["text", "table", "image", "text"], [block["kind"] for block in blocks])
        plans = {"chunks": [
            {"start_block": 1, "end_block": 2},
            {"start_block": 3, "end_block": 4},
        ]}
        chunks = validate_chunk_plan(plans, blocks, lines)
        self.assertIn("| timeout | 超时 |", chunks[0]["content"])
        self.assertIn("screen.png", chunks[1]["content"])
        self.assertEqual("参数", chunks[0]["section"])
        self.assertEqual("界面", chunks[1]["section"])

    def test_asset_enrichment_is_required_and_bounded(self):
        window = [{
            "id": 1,
            "content": "| 名称 | 说明 |\n| timeout | 超时 |\n![](https://example.test/screen.png)",
        }]
        valid = {"chunks": [{
            "id": 1,
            "questions": ["timeout是什么？", "如何配置timeout？", "超时参数有什么作用？"],
            "table_descriptions": ["参数表说明timeout代表超时。"],
            "image_descriptions": ["图片位于timeout配置步骤，像素内容未读取。"],
        }]}
        result = validate_enrichment(valid, window)
        self.assertEqual(3, len(result[1]["questions"]))
        invalid = {"chunks": [dict(valid["chunks"][0], image_descriptions=[])]}
        with self.assertRaisesRegex(ValueError, "asset descriptions"):
            validate_enrichment(invalid, window)

    def test_explanatory_table_is_rewritten_one_original_row_at_a_time(self):
        table = parse_table(
            [
                "| 参数 | 说明 |",
                "| --- | --- |",
                r"| mode | `fast\|safe`模式 |",
                "| timeout | 请求超时时间 |",
            ],
            start_line=10,
            asset_id="table-001",
            section="参数",
        )
        self.assertEqual(["参数", "说明"], table["headers"])
        self.assertEqual(["mode", "`fast|safe`模式"], table["rows"][0]["cells"])
        value = {"tables": [{
            "id": "table-001",
            "type": "EXPLANATORY",
            "rows": [
                {"row": 1, "text": "参数mode表示fast|safe模式。"},
                {"row": 2, "text": "参数timeout表示请求超时时间。"},
            ],
            "summary": "",
        }]}
        enriched = validate_table_enrichment(value, [table])
        self.assertEqual("TABLE_TEXT", enriched["table-001"]["representations"][0]["kind"])
        self.assertEqual(12, enriched["table-001"]["representations"][0]["start_line"])

    def test_data_table_uses_a_summary_bound_to_the_whole_table(self):
        table = parse_table(
            ["| 时间 | QPS |", "| --- | ---: |", "| 10:00 | 120 |", "| 10:01 | 180 |"],
            start_line=20,
            asset_id="table-002",
            section="监控数据",
        )
        value = {"tables": [{
            "id": "table-002",
            "type": "DATA",
            "rows": [],
            "summary": "10:00至10:01的QPS由120上升到180。",
        }]}
        enriched = validate_table_enrichment(value, [table])
        self.assertEqual([{
            "kind": "TABLE_SUMMARY",
            "text": "10:00至10:01的QPS由120上升到180。",
            "start_line": 20,
            "end_line": 23,
        }], enriched["table-002"]["representations"])

    def test_vision_request_contains_real_image_and_context(self):
        image = {
            "id": "image-001",
            "url": "https://example.test/screen.png",
            "section": "创建服务",
            "context_before": "设置基础配置参数。",
            "context_after": "单击创建。",
        }
        messages = build_image_messages([image])
        content = messages[1]["content"]
        self.assertEqual("image_url", content[1]["type"])
        self.assertEqual(image["url"], content[1]["image_url"]["url"])
        description = visual_context(image, "界面显示服务名称和命名空间字段。")
        self.assertIn("创建服务", description)
        self.assertIn("服务名称和命名空间", description)

    def test_embedding_text_removes_image_urls_and_markdown_noise(self):
        raw = (
            '操作步骤\\n![](https://example.test/screen.png "点击放大")\\n'
            '[参数说明](https://example.test/manual) | `timeout` | 30'
        )
        cleaned = clean_embedding_text(raw)
        self.assertNotIn("https://", cleaned)
        self.assertNotIn("![]", cleaned)
        self.assertNotIn("|", cleaned)
        self.assertIn("图片", cleaned)
        self.assertIn("参数说明", cleaned)
        self.assertIn("timeout", cleaned)

    def test_large_explanatory_table_fragment_splits_on_source_rows(self):
        lines = [f"| key-{index} | value-{index} |" for index in range(20)]
        fragment = {
            "section": "参数",
            "start_line": 40,
            "end_line": 59,
            "content": "\n".join(lines),
            "representations": [
                {"kind": "QUESTION", "text": "有哪些参数？"},
                {"kind": "QUESTION", "text": "参数如何配置？"},
                *[
                    {
                        "kind": "TABLE_TEXT",
                        "text": f"key-{index}的值是value-{index}。",
                        "start_line": 40 + index,
                        "end_line": 40 + index,
                    }
                    for index in range(20)
                ],
            ],
        }
        parts = split_asset_fragment(fragment, limit=8)
        self.assertEqual(3, len(parts))
        self.assertEqual((40, 47), (parts[0]["start_line"], parts[0]["end_line"]))
        self.assertEqual((48, 55), (parts[1]["start_line"], parts[1]["end_line"]))
        self.assertEqual((56, 59), (parts[2]["start_line"], parts[2]["end_line"]))
        self.assertEqual(lines, [
            line for part in parts for line in part["content"].splitlines()
        ])

    def test_asset_eval_uses_exact_data_cells_and_image_discriminators(self):
        case = exact_data_case({
            "id": "table-1",
            "document_title": "性能数据",
            "section": "测试结果",
            "source_row": {
                "headers": ["vCPU", "p99时延", "吞吐量"],
                "cells": ["8", "456us", "469"],
            },
        })
        self.assertIn("vCPU为“8”", case["question"])
        self.assertEqual(
            ["p99时延为“456us”", "吞吐量为“469”"],
            case["required_facts"],
        )
        values = image_discriminators(
            "红框标注参数--no-collector.arp，按钮文字为“保存配置”。",
            "Prometheus插件平滑迁移实践",
            "采集配置迁移",
        )
        self.assertIn("no-collector.arp", values)
        self.assertIn("保存配置", values)

    def test_cached_asset_must_have_a_supported_image_signature(self):
        self.assertEqual("image/png", image_content_type(b"\x89PNG\r\n\x1a\npayload"))
        with self.assertRaisesRegex(ValueError, "supported image"):
            image_content_type(b"<html>security verification</html>")


if __name__ == "__main__":
    unittest.main()
