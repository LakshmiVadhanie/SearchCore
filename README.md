# SearchCore

A high-performance, full-text search engine built in Go. Replaces naive SQL `LIKE` queries with a BM25-ranked inverted index over a document corpus, exposed via a gRPC API.

**Target:** sub-10ms p99 latency at 500 RPS on a 10,000-document corpus.

---

## Architecture

```
Client
  |
  | gRPC (protobuf)
  v
SearchCore Service (Go)
  |           |
  |           v
  |     In-Memory Inverted Index (BM25)
  |           |
  v           v
PostgreSQL (document store + persisted index)
```

| Component | Role |
|-----------|------|
| **Inverted index** | In-memory `map[term][]PostingEntry` with `sync.RWMutex`. Rebuilt from PostgreSQL at startup. |
| **BM25 scorer** | Ranks documents by relevance using k1=1.5, b=0.75. IDF computed per-query from corpus stats. |
| **Tokenizer** | Lowercase → strip punctuation → split → remove English stop words. Applied identically at index and query time. |
| **PostgreSQL** | Stores raw documents, posting lists, term stats, and corpus stats. |
| **gRPC API** | Four RPCs: `IndexDocument`, `Search`, `DeleteDocument`, `Stats`. |

---

## Quick Start

### Prerequisites

- Go 1.22+
- Docker & Docker Compose
- `grpcurl` (for smoke testing): `brew install grpcurl`

### 1. Start PostgreSQL

```bash
make docker-up
```

### 2. Build and run the server

```bash
make build
make run
# or directly:
./bin/searchcore
```

The server starts on `:50051` and connects to `postgres://searchcore:searchcore@localhost:5432/searchcore`.

Override with environment variables:

```bash
DATABASE_URL=postgres://... GRPC_ADDR=:9090 ./bin/searchcore
```

### 3. Seed 10K documents

```bash
make seed
```

### 4. Smoke test with grpcurl

```bash
# Index a document
grpcurl -plaintext -d '{"content": "the quick brown fox jumps over the lazy dog"}' \
  localhost:50051 searchcore.SearchService/IndexDocument

# Search
grpcurl -plaintext -d '{"query": "quick fox", "top_k": 5}' \
  localhost:50051 searchcore.SearchService/Search

# Corpus stats
grpcurl -plaintext -d '{}' localhost:50051 searchcore.SearchService/Stats

# Delete a document
grpcurl -plaintext -d '{"doc_id": 1}' localhost:50051 searchcore.SearchService/DeleteDocument
```

---

## gRPC API

| RPC | Input | Output | Description |
|-----|-------|--------|-------------|
| `IndexDocument` | `content: string` | `doc_id`, `success`, `message` | Tokenizes and indexes a document |
| `Search` | `query: string`, `top_k: int32` | ranked `results[]`, `total` | BM25-ranked full-text search |
| `DeleteDocument` | `doc_id: int32` | `success`, `message` | Removes document and its postings |
| `Stats` | — | `total_docs`, `total_terms`, `avg_doc_length` | Corpus metadata |

---

## Running Tests

```bash
make test
# or:
go test -race ./...
```

---

## Load Testing

### Using ghz (gRPC native)

```bash
go install github.com/bojand/ghz/cmd/ghz@latest

ghz --insecure \
    --proto proto/searchcore.proto \
    --call searchcore.SearchService.Search \
    --data '{"query":"search engine ranking","top_k":10}' \
    --rps 500 \
    --duration 30s \
    localhost:50051
```

### Using vegeta (requires HTTP gateway)

```bash
bash load_test/vegeta_attack.sh 500 30s
```

---

## Performance Results

Measured with `ghz` at 500 RPS for 30s against a 10,000-document corpus (Apple M-series, single node).

| Metric | Result | Target |
|--------|--------|--------|
| p50 search latency | 1.39 ms | — |
| p95 search latency | 1.51 ms | — |
| p99 search latency | **1.72 ms** | < 10 ms ✅ |
| Sustained RPS | **499.95** | 500 ✅ |
| Success rate | 99.99% | — |
| Corpus size | 10,000 docs | 10,000 docs ✅ |
| Index load time at startup | < 2 s | < 2 s ✅ |

### Key optimizations that hit the target

- **Atomic pointer swap (lock-free reads):** The index uses `atomic.Pointer[indexData]` so 500 concurrent search goroutines never block each other. Writers copy the index data, mutate, then atomically swap the pointer.
- **In-memory snippet cache:** Document snippets are pre-loaded into the index at startup. The `Search` RPC makes zero DB queries.
- **Min-heap top-k:** Instead of sorting all N matches, a size-k min-heap yields O(N log k) selection.

---

## Project Structure

```
searchcore/
├── cmd/server/main.go             # Entry point
├── internal/
│   ├── index/
│   │   ├── index.go               # In-memory inverted index
│   │   ├── bm25.go                # BM25 scoring
│   │   ├── tokenizer.go           # Tokenization pipeline
│   │   ├── bm25_test.go
│   │   └── tokenizer_test.go
│   ├── store/
│   │   └── postgres.go            # PostgreSQL access layer
│   └── service/
│       └── search_service.go      # gRPC handler implementations
├── proto/searchcore.proto         # Protobuf API contract
├── pb/                            # Generated protobuf Go stubs
├── scripts/seed.go                # 10K document seeder
├── load_test/
│   ├── vegeta_attack.sh           # vegeta load test script
│   └── body.json                  # Request body for vegeta
├── migrations/001_init.sql        # PostgreSQL schema
├── docker-compose.yml             # Local PostgreSQL setup
├── Makefile
├── PHASES.md                      # Implementation phase tracker
└── README.md
```

---

## Tech Stack

| Layer | Choice | Reason |
|-------|--------|--------|
| Language | Go | Performance, concurrency, strong stdlib |
| Transport | gRPC (protobuf) | Binary framing, low serialization overhead |
| Database | PostgreSQL | ACID, reliable, easy local setup |
| Load testing | ghz / vegeta | Scriptable, percentile histograms |
| Containerization | Docker Compose | One-command local dev environment |
