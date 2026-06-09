# SearchCore — Product Requirements Document

**Version:** 1.0  
**Author:** Dhruv Gorasiya  
**Status:** Draft  
**Last Updated:** May 2026

---

## 1. Overview

SearchCore is a high-performance, full-text search engine built in Go. It replaces naive SQL `LIKE` queries with a BM25-ranked inverted index over a document corpus, exposed via a gRPC API. The system is designed to demonstrate production-grade search infrastructure: fast indexing, relevance-ranked retrieval, and high-concurrency throughput validated under load.

---

## 2. Problem Statement

Most applications start with `LIKE '%query%'` for search. This approach has two fundamental problems:

- **No ranking.** Every matching document is treated as equally relevant.
- **No scalability.** Full table scans grow linearly with corpus size and collapse under concurrent load.

SearchCore solves both: it uses an inverted index for O(1) term lookups and BM25 for relevance scoring, achieving sub-10ms p99 latency at 500 concurrent queries on a 10K-document corpus.

---

## 3. Goals

- Index a corpus of plain-text documents and make them full-text searchable.
- Rank results by BM25 relevance score (not recency or insertion order).
- Expose all functionality via a gRPC API.
- Sustain 500 RPS at p99 < 10ms under load-test conditions.
- Keep the system stateless at the service layer (all state in PostgreSQL).

---

## 4. Non-Goals

- No authentication or multi-tenancy (single corpus, single namespace).
- No fuzzy/typo-tolerant matching.
- No multi-field boosting (title vs body weighting).
- No query highlighting or snippets.
- No distributed/sharded index (single node only).
- No real-time streaming ingestion (batch ingest only in v1).

---

## 5. System Architecture

```
Client
  |
  | gRPC (protobuf)
  v
SearchCore Service (Go)
  |          |
  |          v
  |     In-Memory Inverted Index (BM25)
  |          |
  v          v
PostgreSQL (document store + persisted index metadata)
```

### Component Breakdown

**SearchCore Service (Go)**
The core application. Handles document ingestion, index construction, and query processing. Stateless at the process level — all documents and index data are persisted in PostgreSQL.

**Inverted Index (In-Memory)**
Built at startup by loading documents from PostgreSQL. For each term, stores a posting list: `{ doc_id, term_frequency }`. BM25 scores are computed at query time using corpus-level statistics (IDF) stored alongside the index.

**PostgreSQL**
Serves two roles:

- Document store: raw document text and metadata.
- Index persistence: serialized posting lists and corpus statistics, enabling index reconstruction without re-processing documents on restart.

**gRPC API**
All client interactions go through gRPC. Protobuf schemas define the contract for indexing and querying.

---

## 6. Data Model

### PostgreSQL Schema

```sql
-- Raw document storage
CREATE TABLE documents (
    id          SERIAL PRIMARY KEY,
    content     TEXT        NOT NULL,
    created_at  TIMESTAMPTZ DEFAULT NOW()
);

-- Per-term, per-document posting entries
CREATE TABLE postings (
    term        TEXT    NOT NULL,
    doc_id      INT     NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    term_freq   INT     NOT NULL,
    PRIMARY KEY (term, doc_id)
);

-- Corpus-level statistics for BM25 IDF computation
CREATE TABLE corpus_stats (
    key         TEXT PRIMARY KEY,   -- e.g. 'total_docs', 'avg_doc_length'
    value       FLOAT NOT NULL
);

-- Per-term document frequency (number of docs containing the term)
CREATE TABLE term_stats (
    term        TEXT PRIMARY KEY,
    doc_freq    INT NOT NULL
);
```

### In-Memory Index Structure (Go)

```go
type PostingEntry struct {
    DocID     int
    TermFreq  int
}

type Index struct {
    Postings    map[string][]PostingEntry  // term -> posting list
    DocLengths  map[int]int                // doc_id -> token count
    AvgDocLen   float64
    TotalDocs   int
}
```

---

## 7. BM25 Scoring

BM25 ranks documents by relevance to a query. The formula used:

```
score(D, Q) = sum over terms t in Q of:
    IDF(t) * (tf(t,D) * (k1 + 1)) / (tf(t,D) + k1 * (1 - b + b * |D| / avgdl))
```

**Parameters (defaults):**

- `k1 = 1.5` (term frequency saturation)
- `b = 0.75` (document length normalization)

**IDF formula:**

```
IDF(t) = ln((N - df(t) + 0.5) / (df(t) + 0.5) + 1)
```

Where:

- `N` = total number of documents
- `df(t)` = number of documents containing term `t`
- `tf(t, D)` = term frequency of `t` in document `D`
- `|D|` = length of document `D` in tokens
- `avgdl` = average document length across the corpus

---

## 8. gRPC API Contract

### Proto Definition

```protobuf
syntax = "proto3";

package searchcore;

service SearchService {
    rpc IndexDocument (IndexRequest)   returns (IndexResponse);
    rpc Search        (SearchRequest)  returns (SearchResponse);
    rpc DeleteDocument(DeleteRequest)  returns (DeleteResponse);
    rpc Stats         (StatsRequest)   returns (StatsResponse);
}

// --- Index ---

message IndexRequest {
    string content = 1;  // Raw document text
}

message IndexResponse {
    int32  doc_id  = 1;
    bool   success = 2;
    string message = 3;
}

// --- Search ---

message SearchRequest {
    string query   = 1;
    int32  top_k   = 2;  // Number of results to return (default: 10)
}

message SearchResult {
    int32  doc_id  = 1;
    float  score   = 2;
    string snippet = 3;  // First 200 chars of document content
}

message SearchResponse {
    repeated SearchResult results = 1;
    int32                 total   = 2;  // Total matching documents
}

// --- Delete ---

message DeleteRequest {
    int32 doc_id = 1;
}

message DeleteResponse {
    bool   success = 1;
    string message = 2;
}

// --- Stats ---

message StatsRequest {}

message StatsResponse {
    int32  total_docs      = 1;
    int32  total_terms     = 2;
    float  avg_doc_length  = 3;
}
```

### Endpoint Details

| RPC              | Description                                               | Notes                                              |
| ---------------- | --------------------------------------------------------- | -------------------------------------------------- |
| `IndexDocument`  | Tokenizes content, updates inverted index, persists to DB | Synchronous; index is updated immediately          |
| `Search`         | Returns top-k BM25-ranked results for a query             | Multi-term queries use sum of per-term BM25 scores |
| `DeleteDocument` | Removes document from store and index                     | Updates corpus stats (avg doc length, total docs)  |
| `Stats`          | Returns corpus-level metadata                             | Useful for debugging and health checks             |

---

## 9. Tokenization

Tokenization is applied consistently at index time and query time.

**Pipeline:**

1. Lowercase all text.
2. Strip punctuation (keep alphanumeric and whitespace).
3. Split on whitespace.
4. Remove stop words (standard English stop list).
5. (Optional, v1.1) Porter stemming for morphological normalization.

No external NLP libraries. Implement tokenizer in pure Go.

---

## 10. Index Lifecycle

### Startup

1. Connect to PostgreSQL.
2. Load all documents and postings from DB.
3. Reconstruct in-memory `Index` struct.
4. Start gRPC server.

### Indexing a New Document

1. Tokenize document content.
2. Compute per-term frequencies.
3. Insert document into `documents` table.
4. Upsert posting entries into `postings` table.
5. Update `term_stats` (increment `doc_freq` for each new term).
6. Update `corpus_stats` (increment `total_docs`, recalculate `avg_doc_length`).
7. Update in-memory index.

### Deleting a Document

1. Remove from `documents`, cascade deletes `postings`.
2. Decrement `doc_freq` for affected terms in `term_stats`.
3. Update `corpus_stats`.
4. Remove from in-memory index.

---

## 11. Project Structure

```
searchcore/
├── cmd/
│   └── server/
│       └── main.go            # Entry point
├── internal/
│   ├── index/
│   │   ├── index.go           # In-memory index struct and operations
│   │   ├── bm25.go            # BM25 scoring implementation
│   │   └── tokenizer.go       # Tokenization pipeline
│   ├── store/
│   │   └── postgres.go        # DB access layer (documents, postings, stats)
│   └── service/
│       └── search_service.go  # gRPC handler implementations
├── proto/
│   └── searchcore.proto       # Protobuf definition
├── pb/
│   └── searchcore.pb.go       # Generated protobuf code (do not edit)
├── scripts/
│   └── seed.go                # Load 10K documents for benchmarking
├── load_test/
│   └── vegeta_attack.sh       # 500 RPS load test script
├── docker-compose.yml         # PostgreSQL + app
├── Makefile
└── README.md
```

---

## 12. Implementation Phases

### Phase 1 — Core Engine (Week 1)

- [ ] Set up Go module, project structure, Makefile.
- [ ] Implement tokenizer (lowercase, strip punctuation, stop words).
- [ ] Implement in-memory inverted index (posting lists, doc lengths).
- [ ] Implement BM25 scoring function with unit tests.
- [ ] Write unit tests for tokenizer and BM25 (table-driven, Go testing package).

### Phase 2 — Persistence (Week 1-2)

- [ ] Set up PostgreSQL via Docker Compose.
- [ ] Implement `store` layer: insert document, upsert postings, update stats.
- [ ] Implement index load-from-DB at startup.
- [ ] Implement delete with cascading index cleanup.

### Phase 3 — gRPC API (Week 2)

- [ ] Write `searchcore.proto`.
- [ ] Generate Go stubs with `protoc`.
- [ ] Implement `SearchService`: `IndexDocument`, `Search`, `DeleteDocument`, `Stats`.
- [ ] Wire service to index and store layers.
- [ ] Manual smoke test with `grpcurl`.

### Phase 4 — Load Testing and Tuning (Week 2-3)

- [ ] Write seed script: load 10K plain-text documents.
- [ ] Write vegeta load test: 500 RPS for 30 seconds targeting `Search`.
- [ ] Measure p99 latency baseline.
- [ ] Profile with `pprof` if p99 exceeds 10ms.
- [ ] Tune as needed (index read locks, query parallelism).

### Phase 5 — Polish (Week 3)

- [ ] Write README with architecture overview, setup instructions, benchmark results.
- [ ] Add `docker-compose.yml` for one-command local setup.
- [ ] Add GitHub Actions CI (build + test).
- [ ] Record p99 and RPS numbers for resume bullets.

---

## 13. Performance Targets

| Metric                     | Target                           |
| -------------------------- | -------------------------------- |
| p99 search latency         | < 10ms at 500 concurrent queries |
| Throughput                 | 500 RPS sustained                |
| Corpus size                | 10,000 documents                 |
| Index load time at startup | < 2 seconds                      |

---

## 14. Load Testing

Using [vegeta](https://github.com/tsenart/vegeta) for HTTP load testing (via a thin HTTP shim over gRPC, or directly if using grpc-gateway).

```bash
# Sample vegeta attack
echo 'POST http://localhost:8080/search
Content-Type: application/json
{"query": "sample query", "top_k": 10}' \
| vegeta attack -rate=500 -duration=30s \
| vegeta report
```

Key metrics to capture: p50, p95, p99 latency, success rate, RPS.

---

## 15. Tech Stack

| Layer            | Choice          | Reason                                                    |
| ---------------- | --------------- | --------------------------------------------------------- |
| Language         | Go              | Performance, concurrency primitives, strong stdlib        |
| Transport        | gRPC (protobuf) | Binary framing, lower serialization overhead vs JSON/REST |
| Database         | PostgreSQL      | Reliable, ACID, easy local setup                          |
| Load testing     | vegeta          | Scriptable, outputs percentile histograms                 |
| Containerization | Docker Compose  | Reproducible local dev environment                        |

---

## 16. Out of Scope (Future Versions)

- Stemming / lemmatization (v1.1)
- Multi-field documents with field boosting (v1.2)
- Persistent index (skip PostgreSQL, use custom binary format) (v2.0)
- Distributed sharding across nodes (v3.0)
