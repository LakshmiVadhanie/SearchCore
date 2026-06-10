-- Raw document storage
CREATE TABLE IF NOT EXISTS documents (
    id          SERIAL PRIMARY KEY,
    content     TEXT        NOT NULL,
    created_at  TIMESTAMPTZ DEFAULT NOW()
);

-- Per-term, per-document posting entries
CREATE TABLE IF NOT EXISTS postings (
    term        TEXT    NOT NULL,
    doc_id      INT     NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    term_freq   INT     NOT NULL,
    PRIMARY KEY (term, doc_id)
);

-- Corpus-level statistics for BM25 IDF computation
CREATE TABLE IF NOT EXISTS corpus_stats (
    key         TEXT PRIMARY KEY,
    value       FLOAT NOT NULL
);

-- Per-term document frequency (number of docs containing the term)
CREATE TABLE IF NOT EXISTS term_stats (
    term        TEXT PRIMARY KEY,
    doc_freq    INT NOT NULL
);

-- Seed corpus_stats with initial values
INSERT INTO corpus_stats (key, value) VALUES
    ('total_docs',    0),
    ('avg_doc_length', 0)
ON CONFLICT (key) DO NOTHING;
