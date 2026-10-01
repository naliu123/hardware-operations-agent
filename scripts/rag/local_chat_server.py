"""Minimal local OpenAI-compatible chat endpoint for offline RAG preprocessing."""

import json
import os
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from threading import Lock
import time

import torch
from transformers import AutoModelForCausalLM, AutoTokenizer

MODEL = os.environ.get("HWOPS_LOCAL_CHAT_MODEL", "Qwen/Qwen2.5-1.5B-Instruct")
REVISION = os.environ.get(
    "HWOPS_LOCAL_CHAT_REVISION",
    "989aa7980e4cf806f80c7fef2b1adb7bc71aa306",
)
PORT = int(os.environ.get("HWOPS_LOCAL_CHAT_PORT", "18766"))
DEVICE = "mps" if torch.backends.mps.is_available() else "cpu"

tokenizer = AutoTokenizer.from_pretrained(MODEL, revision=REVISION)
model = AutoModelForCausalLM.from_pretrained(
    MODEL,
    revision=REVISION,
    torch_dtype="auto",
).to(DEVICE)
model.eval()
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
        self.reply(200, {"model": MODEL, "revision": REVISION, "device": DEVICE})

    def do_POST(self):
        if self.path != "/v1/chat/completions":
            self.reply(404, {"error": "not found"})
            return
        try:
            size = int(self.headers.get("Content-Length", 0))
            if not 0 < size <= 1024 * 1024:
                raise ValueError("request size")
            body = json.loads(self.rfile.read(size))
            messages = body["messages"]
            if not isinstance(messages, list) or not messages:
                raise ValueError("messages required")
            prompt = tokenizer.apply_chat_template(
                messages, tokenize=False, add_generation_prompt=True,
            )
            max_new_tokens = min(int(body.get("max_tokens", 4096)), 4096)
            started = time.perf_counter()
            with lock, torch.inference_mode():
                inputs = tokenizer([prompt], return_tensors="pt").to(DEVICE)
                generated = model.generate(
                    **inputs,
                    max_new_tokens=max_new_tokens,
                    do_sample=False,
                    repetition_penalty=1.02,
                    pad_token_id=tokenizer.eos_token_id,
                )
            completion = generated[0][inputs.input_ids.shape[1]:]
            content = tokenizer.decode(completion, skip_special_tokens=True).strip()
            prompt_tokens = int(inputs.input_ids.shape[1])
            completion_tokens = int(completion.shape[0])
            self.reply(200, {
                "id": "local-" + str(time.time_ns()),
                "model": MODEL,
                "created": int(time.time()),
                "choices": [{"index": 0, "message": {
                    "role": "assistant", "content": content,
                }, "finish_reason": "stop"}],
                "usage": {
                    "prompt_tokens": prompt_tokens,
                    "completion_tokens": completion_tokens,
                    "total_tokens": prompt_tokens + completion_tokens,
                },
                "latency_ms": round((time.perf_counter() - started) * 1000, 1),
            })
        except (KeyError, TypeError, ValueError, json.JSONDecodeError) as exc:
            self.reply(400, {"error": type(exc).__name__})
        except Exception as exc:
            self.reply(500, {"error": type(exc).__name__})


if __name__ == "__main__":
    print(
        f"Local chat ready: model={MODEL} revision={REVISION} "
        f"device={DEVICE} port={PORT}",
        flush=True,
    )
    ThreadingHTTPServer(("127.0.0.1", PORT), Handler).serve_forever()
