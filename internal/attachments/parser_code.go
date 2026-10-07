package attachments

const parserCode = `import hashlib, io, json, math, os, sys, zipfile
from PIL import Image

SOURCE = next((os.path.join("/inputs", n) for n in os.listdir("/inputs")), "")
OUTPUT = "/work/output/result.zip"
MAX_DERIVED = 80_000_000
MAX_ENTRIES = 600
derived_bytes = 0
derived_entries = 0
gaps = []
pages = []
asset_data = {}

def sha(data):
    return hashlib.sha256(data).hexdigest()

def gap(code, text, page=0):
    value = {"code": code, "text": text}
    if page:
        value["page"] = page
    gaps.append(value)

def add_asset(path, data):
    global derived_bytes, derived_entries
    if derived_entries >= MAX_ENTRIES or derived_bytes + len(data) > MAX_DERIVED:
        return False
    asset_data[path] = data
    derived_entries += 1
    derived_bytes += len(data)
    return True

def image_info(data):
    with Image.open(io.BytesIO(data)) as image:
        image.verify()
    with Image.open(io.BytesIO(data)) as image:
        width, height = image.size
        fmt = image.format
    if width < 1 or height < 1 or width * height > 40_000_000:
        raise ValueError("image dimensions exceed parser policy")
    media = {"PNG":"image/png", "JPEG":"image/jpeg", "WEBP":"image/webp"}.get(fmt)
    if not media:
        raise ValueError("unsupported image encoding")
    return width, height, media

def base_manifest(kind, media, digest):
    return {
        "schema_version": 1, "kind": kind, "media_type": media,
        "original_sha256": digest, "status": "FAILED", "width": 0, "height": 0,
        "page_count": 0, "line_count": 0, "available_pages": [],
        "available_lines": [], "gaps": gaps, "text_chunks": [], "pages": pages,
    }

def parse_image(data, digest, media):
    manifest = base_manifest("IMAGE", media, digest)
    try:
        width, height, actual = image_info(data)
        if actual != media:
            raise ValueError("image signature mismatch")
        manifest.update({"status":"READY", "width":width, "height":height,
                         "page_count":1, "available_pages":[1]})
    except Exception:
        gap("IMAGE_DECODE_FAILED", "图片无法解码，或展开尺寸超过 40,000,000 像素。")
    return manifest

def parse_text(data, digest):
    manifest = base_manifest("TEXT", "text/plain; charset=utf-8", digest)
    try:
        text = data.decode("utf-8", "strict")
        if "\x00" in text:
            raise ValueError("nul")
        chunks = []
        start_offset = 0
        start_line = 1
        offset = 0
        line = 0
        chunk_lines = 0
        for raw_line in data.splitlines(keepends=True):
            line += 1
            chunk_lines += 1
            if len(raw_line) > 65_536:
                raise OverflowError("line")
            offset += len(raw_line)
            if chunk_lines == 200 or offset - start_offset >= 524_288:
                chunks.append({"page":len(chunks)+1, "start_line":start_line, "end_line":line,
                               "start_offset":start_offset, "end_offset":offset})
                start_offset, start_line, chunk_lines = offset, line + 1, 0
            if line > 2_000_000 or len(chunks) > 10_000:
                raise OverflowError("lines")
        if offset < len(data):
            line += 1
            if len(data) - offset > 65_536:
                raise OverflowError("line")
            offset = len(data)
            chunk_lines += 1
        if chunk_lines or not chunks:
            chunks.append({"page":len(chunks)+1, "start_line":start_line,
                           "end_line":max(line, start_line), "start_offset":start_offset,
                           "end_offset":offset})
        manifest.update({"status":"READY", "page_count":len(chunks), "line_count":max(line, 1),
                         "available_lines":[1, max(line, 1)], "text_chunks":chunks})
    except UnicodeDecodeError:
        gap("TEXT_NOT_UTF8", "日志不是严格 UTF-8 文本。")
    except OverflowError:
        gap("TEXT_EXPANSION_LIMIT", "日志行数、单行长度或索引展开量超过解析上限。")
    except Exception:
        gap("TEXT_DECODE_FAILED", "日志无法按文本安全解析。")
    return manifest

def preview(page, number):
    pix = page.get_pixmap(matrix=pymupdf.Matrix(1.5, 1.5), alpha=False)
    image = Image.frombytes("RGB", (pix.width, pix.height), pix.samples)
    image.thumbnail((1200, 1600))
    output = io.BytesIO()
    image.save(output, "JPEG", quality=72, optimize=True)
    data = output.getvalue()
    path = "assets/page-%03d-preview.jpg" % number
    if not add_asset(path, data):
        gap("DERIVED_OUTPUT_LIMIT", "页面预览因派生文件上限未保存。", number)
        return None
    return {"path":path, "kind":"PREVIEW", "name":"page-%d-preview.jpg" % number,
            "media_type":"image/jpeg", "sha256":sha(data), "bytes":len(data),
            "width":image.width, "height":image.height}

def embedded_images(document, page, number):
    found = []
    seen = set()
    images = page.get_images(full=True)
    if len(images) > 50:
        gap("PAGE_IMAGE_LIMIT", "本页内嵌图片超过 50 个，仅记录前 50 个。", number)
        images = images[:50]
    for index, item in enumerate(images, 1):
        xref = item[0]
        if xref in seen:
            continue
        seen.add(xref)
        try:
            extracted = document.extract_image(xref)
            data = extracted["image"]
            width, height, media = image_info(data)
            if len(data) > 10_000_000:
                raise OverflowError()
            ext = {"image/png":"png", "image/jpeg":"jpg", "image/webp":"webp"}[media]
            path = "assets/page-%03d-image-%03d.%s" % (number, index, ext)
            if not add_asset(path, data):
                gap("DERIVED_OUTPUT_LIMIT", "内嵌图片因派生文件上限未保存。", number)
                break
            found.append({"path":path, "kind":"EMBEDDED_IMAGE",
                          "name":"page-%d-image-%d.%s" % (number, index, ext),
                          "media_type":media, "sha256":sha(data), "bytes":len(data),
                          "width":width, "height":height})
        except Exception:
            gap("PDF_IMAGE_UNSUPPORTED", "本页有图片无法安全展开或超过图片上限。", number)
    return found

def page_tables(page, number):
    tables = []
    try:
        discovered = page.find_tables().tables
        if len(discovered) > 20:
            gap("PAGE_TABLE_LIMIT", "本页表格超过 20 个，仅记录前 20 个。", number)
            discovered = discovered[:20]
        for index, table in enumerate(discovered, 1):
            rows = table.extract()
            if len(rows) > 500 or any(len(row) > 50 for row in rows):
                gap("TABLE_EXPANSION_LIMIT", "本页有表格行列超过解析上限。", number)
                continue
            clean = [["" if cell is None else str(cell) for cell in row] for row in rows]
            raw = json.dumps(clean, ensure_ascii=False, separators=(",",":")).encode()
            if len(raw) > 2_000_000:
                gap("TABLE_EXPANSION_LIMIT", "本页有表格文本超过解析上限。", number)
                continue
            tables.append({"index":index, "rows":clean, "sha256":sha(raw)})
    except Exception:
        gap("PDF_TABLE_FAILED", "本页表格提取失败，页面文本仍可用。", number)
    return tables

def parse_pdf(data, digest):
    manifest = base_manifest("PDF", "application/pdf", digest)
    try:
        document = pymupdf.open(stream=data, filetype="pdf")
        if document.needs_pass:
            gap("PDF_ENCRYPTED", "加密 PDF 不受支持。")
            manifest["status"] = "UNSUPPORTED"
            return manifest
        count = document.page_count
        manifest["page_count"] = count
        if count < 1:
            raise ValueError("empty")
        if count > 200:
            gap("PDF_PAGE_LIMIT", "PDF 超过 200 页，未进行截断解析。")
            manifest["status"] = "UNSUPPORTED"
            return manifest
        ready = 0
        scanned = 0
        failed = 0
        text_bytes = 0
        for number in range(1, count + 1):
            item = {"page":number, "status":"FAILED", "text":"",
                    "text_sha256":"", "tables":[], "images":[]}
            try:
                page = document.load_page(number - 1)
                text = page.get_text("text", sort=True)
                raw_text = text.encode("utf-8")
                text_bytes += len(raw_text)
                if len(raw_text) > 2_000_000 or text_bytes > 40_000_000:
                    item["error"] = {"code":"PAGE_TEXT_LIMIT", "message":"页面文本展开量超过上限。"}
                    gap("PAGE_TEXT_LIMIT", "页面文本展开量超过上限。", number)
                    failed += 1
                elif not text.strip() and len(page.get_images(full=True)) > 0:
                    item["status"] = "UNSUPPORTED"
                    item["error"] = {"code":"SCANNED_PAGE", "message":"页面只有扫描图像，本期不执行整页 OCR。"}
                    gap("SCANNED_PAGE", "页面只有扫描图像，本期不执行整页 OCR。", number)
                    scanned += 1
                else:
                    item["status"] = "READY"
                    item["text"] = text
                    item["text_sha256"] = sha(raw_text)
                    item["tables"] = page_tables(page, number)
                    ready += 1
                    manifest["available_pages"].append(number)
                item["preview"] = preview(page, number)
                item["images"] = embedded_images(document, page, number)
            except Exception:
                item["error"] = {"code":"PDF_PAGE_FAILED", "message":"页面解析失败。"}
                gap("PDF_PAGE_FAILED", "页面解析失败。", number)
                failed += 1
            pages.append(item)
        if ready == count and not gaps:
            manifest["status"] = "READY"
        elif ready > 0:
            manifest["status"] = "PARTIAL"
        elif scanned == count:
            manifest["status"] = "UNSUPPORTED"
        else:
            manifest["status"] = "FAILED"
        document.close()
    except Exception:
        gap("PDF_DECODE_FAILED", "PDF 已损坏、加密或无法安全解码。")
        manifest["status"] = "FAILED"
    return manifest

os.makedirs("/work/output", exist_ok=True)
try:
    data = open(SOURCE, "rb").read()
    digest = sha(data)
    if data.startswith(b"\x89PNG\r\n\x1a\n"):
        manifest = parse_image(data, digest, "image/png")
    elif data.startswith(b"\xff\xd8\xff"):
        manifest = parse_image(data, digest, "image/jpeg")
    elif len(data) >= 12 and data[:4] == b"RIFF" and data[8:12] == b"WEBP":
        manifest = parse_image(data, digest, "image/webp")
    elif data.startswith(b"%PDF-"):
        import pymupdf
        manifest = parse_pdf(data, digest)
    else:
        manifest = parse_text(data, digest)
except Exception:
    manifest = base_manifest("UNKNOWN", "application/octet-stream", "")
    gap("PARSER_FAILED", "解析程序未能读取附件。")

raw_manifest = json.dumps(manifest, ensure_ascii=False, separators=(",",":")).encode()
if len(raw_manifest) > 50_000_000:
    manifest = base_manifest(manifest.get("kind","UNKNOWN"), manifest.get("media_type","application/octet-stream"), manifest.get("original_sha256",""))
    gap("MANIFEST_LIMIT", "解析结果超过输出上限。")
    raw_manifest = json.dumps(manifest, ensure_ascii=False, separators=(",",":")).encode()
with zipfile.ZipFile(OUTPUT, "w", compression=zipfile.ZIP_DEFLATED, compresslevel=6) as archive:
    archive.writestr("manifest.json", raw_manifest)
    for path, value in asset_data.items():
        archive.writestr(path, value)
`
