#pragma once
#include "tokenizer.hpp"
#include <shared_mutex>
#include <string>
#include <unordered_map>
#include <vector>

namespace searchcore {

struct PostingEntry {
    int doc_id;
    int term_freq;
};

struct SearchResult {
    int         doc_id;
    double      score;
    std::string snippet;
};

class IndexEngine {
public:
    IndexEngine();
    ~IndexEngine() = default;

    /* ---- Runtime operations (thread-safe) ---- */
    void add_document(int doc_id,
                      const std::vector<std::string>& tokens,
                      const std::string& content);
    void remove_document(int doc_id);

    std::vector<SearchResult> search(const std::vector<std::string>& query_tokens,
                                     int top_k) const;

    /* ---- Statistics (thread-safe) ---- */
    void   snapshot(int* total_docs, double* avg_doc_len) const;
    int    term_count() const;
    int    doc_freq(const std::string& term) const;
    bool   doc_length(int doc_id, int* out_len) const;

    /* ---- Startup loading (single-threaded, no lock needed) ---- */
    void set_posting(const std::string& term, int doc_id, int term_freq);
    void set_doc_length(int doc_id, int length);
    void set_snippet(int doc_id, const std::string& snippet);
    void set_corpus_stats(int total_docs, double avg_doc_len);

private:
    mutable std::shared_mutex rw_mutex_;

    std::unordered_map<std::string, std::vector<PostingEntry>> postings_;
    std::unordered_map<int, int>         doc_lengths_;
    std::unordered_map<int, std::string> snippets_;
    int    total_docs_  = 0;
    double avg_doc_len_ = 0.0;

    /* BM25 helpers */
    static double bm25_idf(int N, int df);
    static double bm25_term_score(int tf, int doc_len, double avg_doc_len, double idf_val);
};

} /* namespace searchcore */
