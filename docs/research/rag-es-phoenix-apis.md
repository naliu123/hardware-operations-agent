# Elasticsearch, Phoenix, DeepSeek, and Chinese Embedding APIs

Verified against primary documentation, release metadata, and tagged source on **2026-09-20**. Research only: no packages installed, services started, models downloaded, or credentials accessed. Snippets below are unexecuted integration examples, not completed implementation or acceptance results.

## Findings and Versions

- **`deepseek-flash` is supported**, not an unrecognized model ID. DeepSeek's official API maps it to **DeepSeek-V4.1-Flash**. Use `https://api.deepseek.com`, not a third-party gateway. [Official quick start](https://api-docs.deepseek.com/)
- **Elasticsearch 9.5.4** has a downloadable native macOS aarch64 archive and bundled OpenJDK; Docker and a separately installed Java are unnecessary. **Native RRF requires Enterprise** in this version; use client-side fusion for Basic.
- **BGE small Chinese outputs 512 dimensions**, not the English small model's 384. Its official repository contains no ONNX artifact at the verified revision; local export is viable but untested here.
- **Phoenix's HTTP host setting does not restrict its gRPC listener**. See the deployment caveat below before starting an unauthenticated local instance.

| Component | Verified version / release | Primary evidence |
| --- | --- | --- |
| `arize-phoenix` server | `20.14.0`, 2026-09-18; Python `>=3.10,<3.15` | [PyPI metadata](https://pypi.org/pypi/arize-phoenix/20.14.0/json) |
| `arize-phoenix-client` | `3.5.0`, 2026-09-08; Python `>=3.10,<3.15` | [PyPI metadata](https://pypi.org/pypi/arize-phoenix-client/3.5.0/json) |
| `arize-phoenix-evals` | `3.8.0`, 2026-09-11; Python `>=3.10,<3.15` | [PyPI metadata](https://pypi.org/pypi/arize-phoenix-evals/3.8.0/json) |
| Go OTel HTTP trace exporter | `v1.46.0`, 2026-08-25 | [Versioned API](https://pkg.go.dev/go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp@v1.46.0) |
| Elasticsearch | `9.5.4`, 2026-09-15 | [Official download page](https://www.elastic.co/downloads/elasticsearch) |
| `sentence-transformers` | `6.1.0`, 2026-09-18; Python `>=3.10` | [PyPI metadata](https://pypi.org/pypi/sentence-transformers/6.1.0/json) |
| `onnxruntime` | `1.30.0`, 2026-09-10; Python `>=3.11` | [PyPI metadata and wheels](https://pypi.org/pypi/onnxruntime/1.30.0/json) |

These are observed releases, not a tested dependency lock. An isolated **Python 3.12** is a reasonable candidate; ORT's published CPython 3.12 Apple Silicon wheel is `onnxruntime-1.30.0-cp312-cp312-macosx_14_0_arm64.whl`, requiring macOS 14+. The main agent owns uv/Python installation and actual OS compatibility checks.

## Phoenix Local Server

The supported pip distribution is `arize-phoenix`; the server command is **`phoenix serve`**. Both `phoenix` and `arize-phoenix` entry points invoke `phoenix.server.main:main`. Server 20.14.0 requires client `>=3.5.0`. [Terminal guide](https://github.com/Arize-ai/phoenix/blob/arize-phoenix-v20.14.0/docs/phoenix/self-hosting/deployment-options/terminal.mdx), [package metadata](https://github.com/Arize-ai/phoenix/blob/arize-phoenix-v20.14.0/pyproject.toml)

Configuration example for the isolated environment, **subject to the gRPC caveat below**:

```sh
PHOENIX_WORKING_DIR=/Users/bytedance/code/.local/phoenix \
PHOENIX_HOST=127.0.0.1 \
PHOENIX_PORT=6006 \
PHOENIX_GRPC_PORT=4317 \
PHOENIX_ENABLE_AUTH=false \
PHOENIX_ALLOW_EXTERNAL_RESOURCES=false \
PHOENIX_TELEMETRY_ENABLED=false \
PHOENIX_DISABLE_AGENT_ASSISTANT=true \
PHOENIX_ENABLE_MCP_SERVER=false \
phoenix serve
```

- Without a database URL, durable SQLite is `${PHOENIX_WORKING_DIR}/phoenix.db`; the working-directory default is `~/.phoenix`.
- `PHOENIX_SQL_DATABASE_URL` overrides that choice. Explicit absolute SQLite example: `sqlite:////Users/bytedance/code/.local/phoenix/phoenix.db`. PostgreSQL is also supported; Elasticsearch is **not** Phoenix's internal storage backend.
- UI and REST base: `http://127.0.0.1:6006`; OTLP/HTTP: **`POST http://127.0.0.1:6006/v1/traces`**; OTLP/gRPC target: **`127.0.0.1:4317`**, without an HTTP path. Phoenix does not use port 4318 for HTTP by default.
- **Network caveat:** `GrpcServer` calls `add_insecure_port("[::]:<port>")` independently of `PHOENIX_HOST`. The CLI has no verified gRPC host/disable flag; port `0` is passed to gRPC, not treated as disabled. `read_only` disables gRPC but also prevents the intended write workflow. Require an independently verified network restriction or another validated launch approach before calling this fully local-only.

Sources: [configuration/defaults](https://github.com/Arize-ai/phoenix/blob/arize-phoenix-v20.14.0/src/phoenix/config.py), [CLI arguments](https://github.com/Arize-ai/phoenix/blob/arize-phoenix-v20.14.0/src/phoenix/server/cli/commands/serve.py), [gRPC binding](https://github.com/Arize-ai/phoenix/blob/arize-phoenix-v20.14.0/src/phoenix/server/grpc_server.py#L55-L111), [OTLP endpoint documentation](https://github.com/Arize-ai/phoenix/blob/arize-phoenix-v20.14.0/docs/phoenix/tracing/concepts-tracing/otel-openinference/exporter.mdx).

### Go OTel and OpenInference

Use ordinary Go OTel; a Phoenix-specific Go SDK is unnecessary for OTLP. Example startup fragment, with `sdktrace` aliasing `go.opentelemetry.io/otel/sdk/trace`; other identifiers come from `otel`, `otel/attribute`, `otel/sdk/resource`, and the HTTP exporter package:

```go
exp, err := otlptracehttp.New(ctx,
    otlptracehttp.WithEndpointURL("http://127.0.0.1:6006/v1/traces"),
    otlptracehttp.WithEncoding(otlptracehttp.EncodingProtobuf),
)
if err != nil {
    return err
}
tp := sdktrace.NewTracerProvider(
    sdktrace.WithBatcher(exp),
    sdktrace.WithResource(resource.NewSchemaless(
        attribute.String("service.name", "hwopsd"),
        attribute.String("openinference.project.name", "hwops-local"),
    )),
)
otel.SetTracerProvider(tp)
```

`WithEndpointURL` uses the supplied path as-is; include `/v1/traces`. Alternatively use `WithEndpoint("127.0.0.1:6006")`, `WithURLPath("/v1/traces")`, and `WithInsecure()`. Flush/shut down the provider at process exit using a fresh bounded context, not an already-cancelled request context. [Exporter API](https://pkg.go.dev/go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp@v1.46.0), [SDK lifecycle](https://pkg.go.dev/go.opentelemetry.io/otel/sdk/trace@v1.46.0)

| Location | Attributes to emit |
| --- | --- |
| Resource | `service.name`; `openinference.project.name` for Phoenix routing |
| Every AI span | `openinference.span.kind`: `CHAIN`, `RETRIEVER`, `EMBEDDING`, `LLM`, `TOOL`, or `EVALUATOR` as appropriate |
| Input/output | `input.value`, `output.value` as strings; `input.mime_type`, `output.mime_type` as `text/plain` or `application/json` |
| Model call | `llm.model_name="deepseek-flash"`, `llm.provider="deepseek"`, `llm.system="deepseek"`; integer `llm.token_count.prompt`, `.completion`, `.total` from actual usage |
| Embeddings | `embedding.model_name="BAAI/bge-small-zh-v1.5"` |
| Retrieval | `retrieval.documents.0.document.id`, `.content`, `.score`, `.metadata`; repeat with zero-based positions |
| Correlation | `session.id`; JSON-string `metadata` containing response ID, device snapshot, revision IDs, and `data_mode` |

Lists of objects must be flattened into indexed attributes; `.metadata` is a JSON string. `openinference.span.kind` is distinct from OTel's transport-oriented `SpanKind`. Recommendation: create these spans around Eino callbacks/adapters and propagate the same `context.Context`; do not assume Go OTel automatically observes Eino or that setting `PHOENIX_PROJECT_NAME` configures a Go resource. Bound/redact text, never record credentials, and avoid logging embedding arrays unnecessarily.

Sources: [OpenInference conventions, revision `a719562e`](https://github.com/Arize-ai/openinference/blob/a719562e20437d433a2e4cfb599256e22ec04531/spec/semantic_conventions.md), [Phoenix resource routing](https://github.com/Arize-ai/phoenix/blob/arize-phoenix-v20.14.0/docs/phoenix/tracing/concepts-tracing/otel-openinference/resource.mdx).

### Python Datasets, Experiments, and Annotations

Use **`from phoenix.client import Client`**, not older examples based on `phoenix.Client().upload_dataset`. Client 3.5.0 accepts `Client(base_url=...)`; no key is needed for a deliberately unauthenticated local server.

```python
from phoenix.client import Client

pc = Client(base_url="http://127.0.0.1:6006")
dataset = pc.datasets.create_dataset(
    name="hwops-qa-replay-v1",
    examples=[{
        "input": {"question": "What does this device alarm mean?"},
        "output": {"status": "NEEDS_CLARIFICATION"},
        "metadata": {"data_mode": "REPLAY", "case_id": "missing-device"},
    }],
)

# This hook must call the Go public HTTP flow and poll its response.
# It is supplied by the implementation, not implemented in this report.
def task(input):
    return call_hwops_public_http(input)

def status_match(output, expected):
    return output["status"] == expected["status"]

experiment = pc.experiments.run_experiment(
    dataset=dataset, task=task,
    evaluators={"status_match": status_match},
    experiment_name="REPLAY-http-contract",
    experiment_metadata={"data_mode": "REPLAY"},
)
```

Verified call shapes; optional parameters are abbreviated:

| Operation | Client 3.5.0 shape / behavior |
| --- | --- |
| Upload tabular data | `pc.datasets.create_dataset(name=..., dataframe=df, input_keys=[...], output_keys=[...], metadata_keys=[...])`; `csv_file_path=...` is an alternative. Do not combine `examples`, tabular, and separate `inputs`/`outputs` modes. |
| Append/retrieve | `pc.datasets.add_examples_to_dataset(dataset=..., examples=...)`; `pc.datasets.get_dataset(dataset=..., version_id=...)`. Returned `Dataset` has `.id`, `.version_id`, and `.examples`. |
| Re-evaluate | `pc.experiments.evaluate_experiment(experiment=experiment, evaluators={name: callable})` |
| Create without executing | `pc.experiments.create(dataset_id=dataset.id, dataset_version_id=dataset.version_id, experiment_name=...)` returns a dict with `["id"]`. |
| Import a Go run | `pc.experiments.log_run(experiment_id=..., dataset_example_id=..., output=..., start_time=..., end_time=..., trace_id=...)`; times are `datetime`, repetition defaults to 1. |
| Import an evaluation | `pc.experiments.log_evaluation(experiment_run_id=..., name=..., annotator_kind="CODE", score=..., label=..., explanation=...)` |
| Annotate a trace span | `pc.spans.add_span_annotation(span_id=..., annotation_name=..., annotator_kind="CODE", score=..., explanation=..., sync=True)` |
| Annotate a retrieved document | `pc.spans.add_document_annotation(span_id=..., document_position=0, annotation_name="relevance", annotator_kind="HUMAN", label=..., sync=True)` |

Tasks bind `input`; named arguments may also bind `expected`/`reference`, `metadata`, and `example`. Evaluators bind `output` and may bind `input`, `expected`, and other documented arguments. Boolean evaluators become 0/1 scores. For actual semantic evaluation use reviewed labels or an explicit judge, not status equality.

REST paths underlying these methods:

```text
POST /v1/datasets/upload?sync=true
GET  /v1/datasets/{dataset_id}/examples?version_id={version_id}
POST /v1/datasets/{dataset_id}/experiments
POST /v1/experiments/{experiment_id}/runs
POST /v1/experiment_evaluations
POST /v1/span_annotations?sync=true
POST /v1/document_annotations?sync=true
```

`log_run` can correlate externally executed Go work using its real trace ID. Successful duplicate experiment runs return 409; evaluations upsert by `(experiment_run_id, name)`. Span annotations upsert by `(span_id, name, identifier)`. Span/document batch annotation payloads use `{"data": [{"span_id": "...", "name": "...", "annotator_kind": "CODE", "result": {"score": 1.0}}]}`; document entries additionally require `document_position`. The batch keyword is **`document_annotations=`**, despite one stale source docstring showing `span_document_annotations=`.

Sources: [client constructor](https://github.com/Arize-ai/phoenix/blob/arize-phoenix-client-v3.5.0/packages/phoenix-client/src/phoenix/client/client.py), [datasets](https://github.com/Arize-ai/phoenix/blob/arize-phoenix-client-v3.5.0/packages/phoenix-client/src/phoenix/client/resources/datasets/__init__.py), [experiments and log methods](https://github.com/Arize-ai/phoenix/blob/arize-phoenix-client-v3.5.0/packages/phoenix-client/src/phoenix/client/resources/experiments/__init__.py), [span/document annotations](https://github.com/Arize-ai/phoenix/blob/arize-phoenix-client-v3.5.0/packages/phoenix-client/src/phoenix/client/resources/spans/__init__.py).

Optional LLM evaluation package 3.8.0 exposes `LLM(provider=..., model=..., **client_kwargs)` and `ClassificationEvaluator(name=..., llm=..., prompt_template=..., choices={"supported": 1, "unsupported": 0})`. `evaluator.evaluate({...})` returns `list[Score]` with `.score`, `.label`, `.explanation`. Classification requires supported tool calling or structured output; DeepSeek judge integration was **not** executed. Log judge results with `annotator_kind="LLM"`, code checks with `"CODE"`, and reviewed labels with `"HUMAN"`. [Evaluator source](https://github.com/Arize-ai/phoenix/blob/arize-phoenix-evals-v3.8.0/packages/phoenix-evals/src/phoenix/evals/evaluators.py), [LLM wrapper](https://github.com/Arize-ai/phoenix/blob/arize-phoenix-evals-v3.8.0/packages/phoenix-evals/src/phoenix/evals/llm/wrapper.py)

## Elasticsearch Without Docker or Premium Features

**Native artifact:** [elasticsearch-9.5.4-darwin-aarch64.tar.gz](https://artifacts.elastic.co/downloads/elasticsearch/elasticsearch-9.5.4-darwin-aarch64.tar.gz). A read-only HEAD returned **HTTP 200**, `Content-Length: 670276511`, `Last-Modified: Tue, 15 Sep 2026 15:21:50 GMT`. The downloaded [official checksum text](https://artifacts.elastic.co/downloads/elasticsearch/elasticsearch-9.5.4-darwin-aarch64.tar.gz.sha512) is:

```text
9dc069e30316d8d62686cd35ec9e8c4668e66dd483e30176de83c73006b00e1a582d92d7c614440128a02032921869c8cfefd879dd28e890be4a6d838651c832  elasticsearch-9.5.4-darwin-aarch64.tar.gz
```

The archive includes OpenJDK and starts with `bin/elasticsearch`; it can be extracted under the workspace. Verify the downloaded archive with `shasum -a 512 -c` during setup. This research verified artifact availability, **not execution on this Mac**. Check the [support matrix](https://www.elastic.co/support/matrix) against the actual OS. Default first-start security enables TLS/authentication; do not assume unauthenticated `http://localhost:9200`. [Archive installation guide](https://www.elastic.co/docs/deploy-manage/deploy/self-managed/install-elasticsearch-from-archive-on-linux-macos)

Basic is the default `xpack.license.self_generated.type`; avoid a trial when validating no-premium operation. Document storage, ordinary BM25, and HNSW `dense_vector` kNN provide the required path. **Native `retriever.rrf` and `linear` are Enterprise-gated** in tagged 9.5.4 source. Explicit `int8_hnsw` avoids depending on version/license-sensitive defaults such as `bbq_disk`. Generate vectors outside ES; do not require ELSER or an ES-hosted inference model.

Sources: [license setting](https://www.elastic.co/docs/reference/elasticsearch/configuration-reference/license-settings), [subscription feature matrix](https://www.elastic.co/subscriptions), [RRF Enterprise declaration](https://github.com/elastic/elasticsearch/blob/v9.5.4/x-pack/plugin/rank-rrf/src/main/java/org/elasticsearch/xpack/rank/rrf/RRFRankPlugin.java#L25-L37), [RRF enforcement](https://github.com/elastic/elasticsearch/blob/v9.5.4/x-pack/plugin/rank-rrf/src/main/java/org/elasticsearch/xpack/rank/rrf/RRFRetrieverBuilder.java#L104-L108), [dense-vector source](https://github.com/elastic/elasticsearch/blob/v9.5.4/server/src/main/java/org/elasticsearch/index/mapper/vectors/DenseVectorFieldMapper.java).

Suggested minimal mapping for `PUT /hwops-fragments-v1`:

```json
{
  "settings": {"number_of_shards": 1, "number_of_replicas": 0},
  "mappings": {"properties": {
    "content": {"type": "text", "analyzer": "cjk"},
    "revision_id": {"type": "keyword"},
    "fragment_id": {"type": "keyword"},
    "content_hash": {"type": "keyword"},
    "published": {"type": "boolean"},
    "embedding": {
      "type": "dense_vector", "dims": 512,
      "index": true, "similarity": "cosine",
      "index_options": {"type": "int8_hnsw"}
    }
  }}
}
```

`cjk` is built in, avoiding a plugin installation; assess retrieval quality on the actual Chinese hardware corpus. Preserve exact error codes/model identifiers in additional keyword fields. [Built-in analyzer registration](https://github.com/elastic/elasticsearch/blob/v9.5.4/modules/analysis-common/src/main/java/org/elasticsearch/analysis/common/CommonAnalysisPlugin.java), [BM25 default](https://github.com/elastic/elasticsearch/blob/v9.5.4/server/src/main/java/org/elasticsearch/index/similarity/SimilarityService.java), [vector mapping](https://www.elastic.co/docs/reference/elasticsearch/mapping-reference/dense-vector)

Send two requests to **`POST /hwops-fragments-v1/_search`**. These Python dictionaries express the JSON bodies; `question`, `eligible_revision_ids`, and `query_vector` are supplied values, with a real 512-element float vector:

```python
filters = [{"term": {"published": True}},
           {"terms": {"revision_id": eligible_revision_ids}}]
bm25 = {"size": 20, "query": {"bool": {
    "must": [{"match": {"content": question}}], "filter": filters}}}
knn = {"size": 20, "knn": {
    "field": "embedding", "query_vector": query_vector,
    "k": 20, "num_candidates": 100, "filter": filters}}
```

Fuse by immutable fragment identity using `score(d) = sum(1 / (60 + rank_i(d)))`, ranks starting at 1; absent documents contribute zero. Use deterministic tie-breaking. This client computation does not invoke the licensed server ranker. A `bool.should` combination of `match` and `knn` is another supported option, but combines scores rather than doing RRF. Place vector eligibility filters **inside `knn.filter`** to prefilter candidates; outer postfilters can leave too few results. Recommendation: use a stable index generation/shared snapshot across branches and retain the project's final applicability/publication checks, including fail-closed behavior when no revisions are eligible.

Sources: [kNN request/filter semantics](https://www.elastic.co/docs/reference/query-languages/query-dsl/query-dsl-knn-query), [top-level kNN](https://www.elastic.co/docs/solutions/search/vector/knn), [RRF formula](https://www.elastic.co/docs/reference/elasticsearch/rest-apis/reciprocal-rank-fusion).

## DeepSeek Provider

Official OpenAI-compatible base: **`https://api.deepseek.com`**. Chat API: **`POST https://api.deepseek.com/chat/completions`**. Model: **`deepseek-flash`**, currently served by **DeepSeek-V4.1-Flash**. Legacy `deepseek-v4-flash` and `deepseek-v4-flash-vision-exp` names are accepted but their original models are retired; do not silently substitute `deepseek-chat`.

Example request body, deliberately without authentication material:

```json
{
  "model": "deepseek-flash",
  "messages": [{"role": "user", "content": "Return only the requested answer."}],
  "thinking": {"type": "disabled"},
  "stream": false
}
```

Thinking defaults to **enabled**; explicitly disable it for a non-thinking baseline. OpenAI Python clients pass this via `extra_body={"thinking": {"type": "disabled"}}`. If tool calling is later used with thinking enabled, preserve the documented `reasoning_content` round-trip requirements; generic OpenAI compatibility alone does not prove the Eino adapter handles them. Model availability here means **officially documented**, not authenticated access, account entitlement, latency, or output quality verification.

Sources: [quick start](https://api-docs.deepseek.com/), [model table](https://api-docs.deepseek.com/quick_start/pricing), [chat API](https://api-docs.deepseek.com/api/create-chat-completion), [thinking mode](https://api-docs.deepseek.com/guides/thinking_mode).

## Lightweight Chinese Embeddings

**`BAAI/bge-small-zh-v1.5`**, pinned model revision **`7999e1d3359715c523056ef9478215996d62a620`**:

- MIT license; Chinese; approximately 24M parameters; 4 BERT layers; **512 output dimensions**; **512-token** position limit.
- Use **CLS pooling**, not mean pooling, then L2 normalization. For short-query-to-passage retrieval the model card recommends the exact query prefix encoded below; do **not** prefix documents. Version 1.5 can run without the instruction, with quality to be evaluated on the task.
- Official repository files contain PyTorch/safetensors and tokenizer assets, **no ONNX model** at this revision. Sentence Transformers documents automatic ONNX export and CPU inference; this establishes a supported route, not tested performance here.

```python
from sentence_transformers import SentenceTransformer

model = SentenceTransformer(
    "BAAI/bge-small-zh-v1.5",
    revision="7999e1d3359715c523056ef9478215996d62a620",
    backend="onnx",
    model_kwargs={"provider": "CPUExecutionProvider"},
)
query_prefix = (
    "\u4e3a\u8fd9\u4e2a\u53e5\u5b50\u751f\u6210\u8868\u793a"
    "\u4ee5\u7528\u4e8e\u68c0\u7d22\u76f8\u5173\u6587\u7ae0:"
)
q = model.encode([query_prefix + question], normalize_embeddings=True)
d = model.encode(passages, normalize_embeddings=True)
```

Export tooling uses `sentence-transformers[onnx]`; cache/export once during installation. A direct ONNX Runtime service must reproduce tokenizer/truncation behavior, select `last_hidden_state[:, 0, :]`, and normalize: the ordinary ONNX export contains the Transformer, **not sentence pooling/normalization**. An ARM64 quantization preset exists, but start with FP32 parity and measure before adopting int8. A small local Python embedding process is a viable Go/Eino boundary; its HTTP contract, resident memory, latency, and Go-native runtime alternatives remain implementation work.

Sources: [BAAI model card](https://huggingface.co/BAAI/bge-small-zh-v1.5/blob/7999e1d3359715c523056ef9478215996d62a620/README.md), [model config](https://huggingface.co/BAAI/bge-small-zh-v1.5/blob/7999e1d3359715c523056ef9478215996d62a620/config.json), [pooling config](https://huggingface.co/BAAI/bge-small-zh-v1.5/blob/7999e1d3359715c523056ef9478215996d62a620/1_Pooling/config.json), [repository metadata/file list](https://huggingface.co/api/models/BAAI/bge-small-zh-v1.5), [Sentence Transformers ONNX documentation](https://sbert.net/docs/sentence_transformer/usage/efficiency.html#onnx).

## Verification Boundary

Pending implementation checks: isolated dependency resolution; actual macOS support; Phoenix listener isolation and restart persistence; Go trace ingestion and correlation; dataset/experiment round-trip; ES Basic-license BM25/kNN and restart tests; ONNX parity/Chinese retrieval quality; authenticated DeepSeek integration under separately authorized conditions. Keep deterministic external-model fixtures labeled **REPLAY**, exercise the public HTTP flow with a real temporary store, and report real model integration separately. The main agent owns source-page ingestion and the reported 55 table rows; this report does not claim they were indexed or evaluated.
