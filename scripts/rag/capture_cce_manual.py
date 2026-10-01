"""Capture every leaf document in the live CCE user-guide navigation."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import time
from urllib.parse import urljoin, urlsplit, urlunsplit

from bs4 import BeautifulSoup
from markdownify import markdownify

ROOT = Path(__file__).resolve().parents[2]
DEFAULT_OUTPUT = ROOT / "testdata/rag/cce-manual"
DEFAULT_PROFILE = ROOT / ".cache/rag/cce-manual-browser"
DEFAULT_PLAYWRIGHT = ROOT / ".tools/playwright"
NAVIGATION_URL = "https://support.huaweicloud.com/intl/zh-cn/usermanual-cce/cce_10_0091.html"
CANONICAL_PREFIX = "https://support.huaweicloud.com/usermanual-cce/"
REQUEST_PREFIX = "https://support.huaweicloud.com/intl/zh-cn/usermanual-cce/"


def atomic_json(path, value):
    path.parent.mkdir(parents=True, exist_ok=True)
    temporary = path.with_suffix(path.suffix + ".tmp")
    temporary.write_text(json.dumps(value, ensure_ascii=False, indent=2) + "\n")
    temporary.replace(path)


def canonical_url(url):
    parsed = urlsplit(url)
    path = parsed.path.replace("/intl/zh-cn/usermanual-cce/", "/usermanual-cce/", 1)
    return urlunsplit(("https", "support.huaweicloud.com", path, "", ""))


def request_url(url):
    return REQUEST_PREFIX + Path(urlsplit(canonical_url(url)).path).name


def direct_title(item):
    anchor = item.find("a", recursive=False)
    return anchor.get_text(" ", strip=True) if anchor else ""


def parse_navigation(html):
    soup = BeautifulSoup(html, "html.parser")
    navigation = soup.select_one("#support-nav ul.side-nav")
    if navigation is None:
        raise ValueError("CCE navigation was not loaded")
    roots = [item for item in navigation.find_all("li", recursive=False)
             if direct_title(item) == "用户指南"]
    if len(roots) != 1:
        raise ValueError(f"expected one 用户指南 root, found {len(roots)}")
    items = []
    for node in roots[0].select("li.nav-item"):
        anchor = node.find("a", recursive=False)
        if anchor is None:
            continue
        target = anchor.get("p-href") or anchor.get("href", "")
        if "/usermanual-cce/" not in target or not urlsplit(target).path.endswith(".html"):
            continue
        hierarchy = []
        for parent in reversed(node.find_parents("li", class_="nav-item")):
            title = direct_title(parent)
            if title and title != "用户指南":
                hierarchy.append(title)
        title = anchor.get_text(" ", strip=True)
        hierarchy.append(title)
        actual_href = anchor.get("href", "")
        items.append({
            "id": Path(urlsplit(target).path).stem,
            "title": title,
            "navigation_path": hierarchy,
            "kind": "category" if actual_href.startswith("javascript") else "document",
            "url": canonical_url(target),
            "request_url": request_url(target),
        })
    urls = [item["url"] for item in items]
    if len(items) < 100 or len(urls) != len(set(urls)):
        raise ValueError(f"invalid navigation: {len(items)} entries, {len(set(urls))} unique URLs")
    return items


def normalize_article(html, navigation_item):
    soup = BeautifulSoup(html, "html.parser")
    if soup.title and soup.title.get_text(strip=True) == "Security Verification":
        raise ValueError("security verification response")
    article = soup.select_one(".articleBoxWithoutHead")
    if article is None:
        raise ValueError("article container missing")
    heading = article.select_one("h1")
    title = heading.get_text(" ", strip=True) if heading else navigation_item["title"]
    updated = soup.select_one(".help-content > .updateTime .updateInfo, .articleBoxWithoutHead .updateInfo")
    body = article.select_one('div[id^="body"]')
    if body is None:
        raise ValueError("article body missing")
    for node in body.select("script, style, noscript, button"):
        node.decompose()
    for image in body.select("img"):
        source = image.get("data-original") or image.get("data-src") or image.get("src")
        if source:
            image["src"] = urljoin(navigation_item["request_url"], source)
    for anchor in body.select("a[href]"):
        href = urljoin(navigation_item["request_url"], anchor["href"])
        if "/intl/zh-cn/usermanual-cce/" in href:
            href = canonical_url(href)
        anchor["href"] = href
    content = markdownify(str(body), heading_style="ATX", bullets="-", strip=["script", "style"])
    content = re.sub(r"[ \t]+\n", "\n", content)
    content = re.sub(r"\n{3,}", "\n\n", content).strip()
    if len(re.sub(r"\W", "", content)) < 20:
        raise ValueError("article body has no substantive text")
    prefix = [
        f"# {title}", "",
        f"来源：{navigation_item['url']}",
        "目录：" + " > ".join(["用户指南", *navigation_item["navigation_path"]]),
    ]
    if updated:
        prefix.append("更新时间：" + updated.get_text(" ", strip=True))
    document = "\n".join([*prefix, "", content, ""])
    return {
        "title": title,
        "updated_at": updated.get_text(" ", strip=True) if updated else "",
        "content": document,
        "content_sha256": hashlib.sha256(document.encode()).hexdigest(),
        "body_html_sha256": hashlib.sha256(str(body).encode()).hexdigest(),
        "bytes": len(document.encode()),
    }


def valid_checkpoint(entry, output):
    if entry.get("status") != "captured":
        return False
    path = output / entry["file"]
    return path.exists() and hashlib.sha256(path.read_bytes()).hexdigest() == entry["content_sha256"]


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--output", type=Path, default=DEFAULT_OUTPUT)
    parser.add_argument("--profile", type=Path, default=DEFAULT_PROFILE)
    parser.add_argument("--navigation-html", type=Path)
    parser.add_argument("--limit", type=int)
    args = parser.parse_args()
    os.environ.setdefault("PLAYWRIGHT_BROWSERS_PATH", str(DEFAULT_PLAYWRIGHT))
    from playwright.sync_api import sync_playwright

    output = args.output.resolve()
    documents = output / "documents"
    documents.mkdir(parents=True, exist_ok=True)
    checkpoint_path = output / "capture-checkpoint.json"
    checkpoint = json.loads(checkpoint_path.read_text()) if checkpoint_path.exists() else {"documents": {}}

    with sync_playwright() as playwright:
        browser = playwright.chromium.launch(headless=False, args=["--disable-crash-reporter", "--disable-breakpad"])
        context = browser.new_context(locale="zh-CN")
        page = context.new_page()
        page.goto(NAVIGATION_URL, wait_until="domcontentloaded", timeout=60_000)
        page.wait_for_function("document.title !== 'Security Verification'", timeout=120_000)
        page.wait_for_function(
            "document.querySelectorAll('#support-nav ul.side-nav li.nav-item a[p-href*=\"/usermanual-cce/\"]').length > 700",
            timeout=60_000,
        )
        navigation_html = args.navigation_html.read_text() if args.navigation_html else page.content()
        items = parse_navigation(navigation_html)
        documents_to_capture = [item for item in items if item["kind"] == "document"]
        if args.limit:
            documents_to_capture = documents_to_capture[:args.limit]
        navigation_payload = {
            "captured_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
            "source_page": canonical_url(NAVIGATION_URL),
            "entry_count": len(items),
            "document_count": sum(item["kind"] == "document" for item in items),
            "category_count": sum(item["kind"] == "category" for item in items),
            "items": items,
        }
        navigation_payload["sha256"] = hashlib.sha256(
            json.dumps(items, ensure_ascii=False, sort_keys=True).encode()
        ).hexdigest()
        atomic_json(output / "navigation.json", navigation_payload)
        print(f"Navigation: {navigation_payload['document_count']} documents, "
              f"{navigation_payload['category_count']} categories", flush=True)

        failures = {}
        for index, item in enumerate(documents_to_capture, 1):
            existing = checkpoint["documents"].get(item["id"], {})
            if valid_checkpoint(existing, output):
                if index % 25 == 0:
                    print(f"Capture {index}/{len(documents_to_capture)} (checkpoint)", flush=True)
                continue
            error = None
            for _ in range(2):
                try:
                    response = context.request.get(item["request_url"], timeout=60_000)
                    if not response.ok:
                        raise ValueError(f"HTTP {response.status}")
                    normalized = normalize_article(response.text(), item)
                    relative = Path("documents") / f"{item['id']}.md"
                    (output / relative).write_text(normalized.pop("content"))
                    checkpoint["documents"][item["id"]] = {
                        **item, **normalized, "file": str(relative), "status": "captured",
                    }
                    error = None
                    break
                except Exception as exc:
                    error = str(exc)
            if error:
                failures[item["id"]] = error
                checkpoint["documents"][item["id"]] = {**item, "status": "failed", "error": error}
            if index % 10 == 0 or error:
                checkpoint["updated_at"] = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
                atomic_json(checkpoint_path, checkpoint)
                print(f"Capture {index}/{len(documents_to_capture)} failures={len(failures)}", flush=True)
        checkpoint["updated_at"] = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
        atomic_json(checkpoint_path, checkpoint)
        browser.close()

    captured = [item for item in checkpoint["documents"].values() if item["status"] == "captured"]
    manifest = {
        "schema_version": 1,
        "captured_at": checkpoint["updated_at"],
        "source_page": canonical_url(NAVIGATION_URL),
        "navigation_sha256": navigation_payload["sha256"],
        "navigation_entries": len(items),
        "category_entries": navigation_payload["category_count"],
        "expected_documents": len(documents_to_capture),
        "captured_documents": len(captured),
        "failed_documents": failures,
        "total_utf8_bytes": sum(item["bytes"] for item in captured),
        "documents": sorted(captured, key=lambda item: next(
            i for i, nav in enumerate(items) if nav["id"] == item["id"]
        )),
    }
    manifest["corpus_sha256"] = hashlib.sha256(
        "".join(item["content_sha256"] for item in manifest["documents"]).encode()
    ).hexdigest()
    atomic_json(output / "manifest.json", manifest)
    if failures or len(captured) != len(documents_to_capture):
        raise SystemExit(f"capture incomplete: {len(captured)}/{len(documents_to_capture)}, failures={len(failures)}")
    print(json.dumps({key: manifest[key] for key in [
        "navigation_entries", "category_entries", "captured_documents",
        "total_utf8_bytes", "corpus_sha256",
    ]}, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
