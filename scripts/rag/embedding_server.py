"""Small local Chinese embedding endpoint; lifetime owned by the RAG runner."""

import json
import os
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from threading import Lock

os.environ.setdefault("TOKENIZERS_PARALLELISM", "false")
os.environ.setdefault("OMP_NUM_THREADS", "2")
os.environ.setdefault("MKL_NUM_THREADS", "2")

import torch
from sentence_transformers import SentenceTransformer

MODEL = "BAAI/bge-small-zh-v1.5"
REVISION = "7999e1d3359715c523056ef9478215996d62a620"
PREFIX = "为这个句子生成表示以用于检索相关文章:"
torch.set_num_threads(2)
torch.set_num_interop_threads(1)
model = SentenceTransformer(
    os.environ.get("HWOPS_EMBED_MODEL_PATH", MODEL),
    revision=REVISION,
    device="cpu",
)
model.max_seq_length = 512
lock = Lock()


class Handler(BaseHTTPRequestHandler):
    def log_message(self, format, *args):
        pass

    def reply(self, code, body):
        raw = json.dumps(body, ensure_ascii=False).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(raw)))
        self.end_headers()
        self.wfile.write(raw)

    def do_GET(self):
        if self.path != "/healthz":
            self.reply(404, {"error": "not found"})
            return
        self.reply(200, {"model": MODEL, "revision": REVISION, "dimensions": 512, "device": "cpu", "threads": 2})

    def do_POST(self):
        if self.path != "/v1/embeddings":
            self.reply(404, {"error": "not found"})
            return
        try:
            size = int(self.headers.get("Content-Length", 0))
            if not 0 < size <= 1024*1024:
                raise ValueError("request size")
            body = json.loads(self.rfile.read(size))
            texts = body["input"]
            if isinstance(texts, str):
                texts = [texts]
            if not isinstance(texts, list) or not 0 < len(texts) <= 64:
                raise ValueError("input must contain 1 to 64 strings")
            if any(not isinstance(text, str) or not text.strip() or len(text) > 32000 for text in texts):
                raise ValueError("invalid text")
            if body.get("input_type") == "query":
                texts = [PREFIX + text for text in texts]
            with lock, torch.inference_mode():
                vectors = model.encode(texts, batch_size=16, normalize_embeddings=True, show_progress_bar=False)
            self.reply(200, {
                "model": MODEL,
                "data": [{"index": i, "embedding": vector.tolist()} for i, vector in enumerate(vectors)],
            })
        except (ValueError, KeyError, TypeError, json.JSONDecodeError):
            self.reply(400, {"error": "invalid embedding input"})
        except Exception:
            self.reply(500, {"error": "embedding failed"})


if __name__ == "__main__":
    port = int(os.environ.get("HWOPS_EMBED_PORT", "8765"))
    print(f"Embedding ready: {MODEL} revision={REVISION} CPU threads=2 port={port}", flush=True)
    ThreadingHTTPServer(("127.0.0.1", port), Handler).serve_forever()
