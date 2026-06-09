#include "index_engine.hpp"
#include <algorithm>
#include <cmath>
#include <queue>

namespace searchcore {

/* BM25 constants — match the Go implementation */
static constexpr double k1 = 1.5;
static constexpr double b  = 0.75;

/* ---- Constructor ------------------------------------------------------- */

IndexEngine::IndexEngine() = default;

/* ---- BM25 helpers ------------------------------------------------------ */

double IndexEngine::bm25_idf(int N, int df)
{
    if (df <= 0) return 0.0;
    return std::log((static_cast<double>(N - df) + 0.5) /
                    (static_cast<double>(df) + 0.5) + 1.0);
}

double IndexEngine::bm25_term_score(int tf, int doc_len, double avg_doc_len, double idf_val)
{
    double tf_f    = static_cast<double>(tf);
    double len_norm = 1.0 - b + b * static_cast<double>(doc_len) / avg_doc_len;
    return idf_val * (tf_f * (k1 + 1.0)) / (tf_f + k1 * len_norm);
}

/* ---- add_document (exclusive write lock) -------------------------------- */

void IndexEngine::add_document(int doc_id,
                               const std::vector<std::string>& tokens,
                               const std::string& content)
{
    /* Build term-frequency map */
    std::unordered_map<std::string, int> tf;
    tf.reserve(tokens.size());
    for (const auto& t : tokens) tf[t]++;
    int doc_len = static_cast<int>(tokens.size());

    std::string snippet = content.substr(0, 200);

    std::unique_lock<std::shared_mutex> lock(rw_mutex_);

    for (const auto& [term, freq] : tf) {
        postings_[term].push_back({doc_id, freq});
    }
    doc_lengths_[doc_id] = doc_len;
    snippets_[doc_id]    = std::move(snippet);

    total_docs_++;
    avg_doc_len_ += (static_cast<double>(doc_len) - avg_doc_len_) /
                    static_cast<double>(total_docs_);
}

/* ---- remove_document (exclusive write lock) ----------------------------- */

void IndexEngine::remove_document(int doc_id)
{
    std::unique_lock<std::shared_mutex> lock(rw_mutex_);

    auto it = doc_lengths_.find(doc_id);
    if (it == doc_lengths_.end()) return; /* no-op */

    int doc_len = it->second;

    /* Remove postings for this doc */
    for (auto& [term, plist] : postings_) {
        plist.erase(
            std::remove_if(plist.begin(), plist.end(),
                           [doc_id](const PostingEntry& p) {
                               return p.doc_id == doc_id;
                           }),
            plist.end()
        );
    }
    /* Erase terms with empty posting lists */
    for (auto pit = postings_.begin(); pit != postings_.end(); ) {
        if (pit->second.empty()) {
            pit = postings_.erase(pit);
        } else {
            ++pit;
        }
    }

    doc_lengths_.erase(doc_id);
    snippets_.erase(doc_id);

    int prev_total = total_docs_;
    total_docs_--;
    if (total_docs_ == 0) {
        avg_doc_len_ = 0.0;
    } else {
        avg_doc_len_ = (avg_doc_len_ * static_cast<double>(prev_total) -
                        static_cast<double>(doc_len)) /
                       static_cast<double>(total_docs_);
    }
}

/* ---- search (shared read lock) ----------------------------------------- */

std::vector<SearchResult> IndexEngine::search(
    const std::vector<std::string>& query_tokens, int top_k) const
{
    if (query_tokens.empty() || top_k <= 0) return {};

    std::shared_lock<std::shared_mutex> lock(rw_mutex_);

    if (total_docs_ == 0 || avg_doc_len_ == 0.0) return {};

    std::unordered_map<int, double> scores;
    scores.reserve(1024);

    for (const auto& term : query_tokens) {
        auto pit = postings_.find(term);
        if (pit == postings_.end()) continue;

        const auto& plist = pit->second;
        double idf_val = bm25_idf(total_docs_, static_cast<int>(plist.size()));

        for (const auto& p : plist) {
            auto lit = doc_lengths_.find(p.doc_id);
            int dlen = (lit != doc_lengths_.end()) ? lit->second : 0;
            scores[p.doc_id] += bm25_term_score(p.term_freq, dlen, avg_doc_len_, idf_val);
        }
    }

    if (scores.empty()) return {};

    /* Min-heap of size top_k for O(N log k) selection */
    using Pair = std::pair<double, int>; /* score, doc_id */
    std::priority_queue<Pair, std::vector<Pair>, std::greater<Pair>> heap;

    for (const auto& [doc_id, score] : scores) {
        if (static_cast<int>(heap.size()) < top_k) {
            heap.push({score, doc_id});
        } else if (score > heap.top().first) {
            heap.pop();
            heap.push({score, doc_id});
        }
    }

    /* Pop into results in ascending order, then reverse */
    std::vector<SearchResult> results;
    results.reserve(heap.size());
    while (!heap.empty()) {
        auto [score, doc_id] = heap.top();
        heap.pop();
        std::string snip;
        auto sit = snippets_.find(doc_id);
        if (sit != snippets_.end()) snip = sit->second;
        results.push_back({doc_id, score, std::move(snip)});
    }
    std::reverse(results.begin(), results.end()); /* highest score first */
    return results;
}

/* ---- Statistics -------------------------------------------------------- */

void IndexEngine::snapshot(int* total_docs, double* avg_doc_len) const
{
    std::shared_lock<std::shared_mutex> lock(rw_mutex_);
    *total_docs  = total_docs_;
    *avg_doc_len = avg_doc_len_;
}

int IndexEngine::term_count() const
{
    std::shared_lock<std::shared_mutex> lock(rw_mutex_);
    return static_cast<int>(postings_.size());
}

int IndexEngine::doc_freq(const std::string& term) const
{
    std::shared_lock<std::shared_mutex> lock(rw_mutex_);
    auto it = postings_.find(term);
    if (it == postings_.end()) return 0;
    return static_cast<int>(it->second.size());
}

bool IndexEngine::doc_length(int doc_id, int* out_len) const
{
    std::shared_lock<std::shared_mutex> lock(rw_mutex_);
    auto it = doc_lengths_.find(doc_id);
    if (it == doc_lengths_.end()) return false;
    *out_len = it->second;
    return true;
}

/* ---- Startup loading (no lock — called single-threaded at startup) ------ */

void IndexEngine::set_posting(const std::string& term, int doc_id, int term_freq)
{
    postings_[term].push_back({doc_id, term_freq});
}

void IndexEngine::set_doc_length(int doc_id, int length)
{
    doc_lengths_[doc_id] = length;
}

void IndexEngine::set_snippet(int doc_id, const std::string& snippet)
{
    snippets_[doc_id] = snippet;
}

void IndexEngine::set_corpus_stats(int total_docs, double avg_doc_len)
{
    total_docs_  = total_docs;
    avg_doc_len_ = avg_doc_len;
}

} /* namespace searchcore */
