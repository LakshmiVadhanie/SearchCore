/* c_api.cpp — extern "C" bridge between CGo and the C++ IndexEngine.
 *
 * Memory rules:
 *   - IndexHandle: allocated by index_new(), freed by index_free().
 *   - char** from index_tokenize(): allocated here, freed by index_free_tokens().
 *   - SearchResultArray from index_search(): allocated here, freed by index_free_results().
 *   - All strings passed in from Go (const char*) are read-only and must not be retained.
 */

#include "../include/index_c_api.h"
#include "index_engine.hpp"
#include "tokenizer.hpp"
#include <cstdlib>
#include <cstring>

/* Thread-local Tokenizer for index_tokenize() (standalone call path).
 * IndexEngine::add_document takes pre-tokenized tokens, so there is no
 * separate stemmer inside IndexEngine itself. */
static thread_local searchcore::Tokenizer* tl_tokenizer = nullptr;

static searchcore::Tokenizer* get_tokenizer() {
    if (!tl_tokenizer) {
        tl_tokenizer = new searchcore::Tokenizer();
    }
    return tl_tokenizer;
}

/* ---- Lifecycle ---- */

IndexHandle index_new(void)
{
    return new searchcore::IndexEngine();
}

void index_free(IndexHandle h)
{
    delete static_cast<searchcore::IndexEngine*>(h);
}

/* ---- Tokenize ---- */

int index_tokenize(const char* text, char*** tokens_out)
{
    auto* tok = get_tokenizer();
    std::vector<std::string> tv = tok->tokenize(text ? text : "");

    int count = static_cast<int>(tv.size());
    if (count == 0) {
        *tokens_out = nullptr;
        return 0;
    }

    char** arr = static_cast<char**>(std::malloc(sizeof(char*) * static_cast<size_t>(count)));
    if (!arr) { *tokens_out = nullptr; return 0; }

    for (int i = 0; i < count; ++i) {
        arr[i] = static_cast<char*>(std::malloc(tv[i].size() + 1));
        std::memcpy(arr[i], tv[i].c_str(), tv[i].size() + 1);
    }
    *tokens_out = arr;
    return count;
}

void index_free_tokens(char** tokens, int count)
{
    if (!tokens) return;
    for (int i = 0; i < count; ++i) std::free(tokens[i]);
    std::free(tokens);
}

/* ---- Document operations ---- */

void index_add_document(IndexHandle h, int doc_id,
                        const char** tokens, int n_tokens,
                        const char* content, int content_len)
{
    auto* eng = static_cast<searchcore::IndexEngine*>(h);

    std::vector<std::string> tv;
    tv.reserve(static_cast<size_t>(n_tokens));
    for (int i = 0; i < n_tokens; ++i) tv.emplace_back(tokens[i]);

    std::string cont(content, static_cast<size_t>(content_len));
    eng->add_document(doc_id, tv, cont);
}

void index_remove_document(IndexHandle h, int doc_id)
{
    static_cast<searchcore::IndexEngine*>(h)->remove_document(doc_id);
}

/* ---- Search ---- */

SearchResultArray* index_search(IndexHandle h,
                                const char** query_tokens, int n_tokens,
                                int top_k)
{
    auto* eng = static_cast<searchcore::IndexEngine*>(h);

    std::vector<std::string> qv;
    qv.reserve(static_cast<size_t>(n_tokens));
    for (int i = 0; i < n_tokens; ++i) qv.emplace_back(query_tokens[i]);

    std::vector<searchcore::SearchResult> results = eng->search(qv, top_k);
    if (results.empty()) return nullptr;

    auto* arr = static_cast<SearchResultArray*>(
        std::malloc(sizeof(SearchResultArray)));
    if (!arr) return nullptr;

    arr->count   = static_cast<int>(results.size());
    arr->results = static_cast<CSearchResult*>(
        std::malloc(sizeof(CSearchResult) * static_cast<size_t>(arr->count)));
    if (!arr->results) { std::free(arr); return nullptr; }

    for (int i = 0; i < arr->count; ++i) {
        arr->results[i].doc_id  = results[i].doc_id;
        arr->results[i].score   = results[i].score;
        const std::string& snip = results[i].snippet;
        arr->results[i].snippet = static_cast<char*>(
            std::malloc(snip.size() + 1));
        if (arr->results[i].snippet) {
            std::memcpy(arr->results[i].snippet, snip.c_str(), snip.size() + 1);
        }
    }
    return arr;
}

void index_free_results(SearchResultArray* r)
{
    if (!r) return;
    for (int i = 0; i < r->count; ++i) std::free(r->results[i].snippet);
    std::free(r->results);
    std::free(r);
}

/* ---- Statistics ---- */

void index_snapshot(IndexHandle h, int* total_docs_out, double* avg_doc_len_out)
{
    static_cast<searchcore::IndexEngine*>(h)->snapshot(total_docs_out, avg_doc_len_out);
}

int index_term_count(IndexHandle h)
{
    return static_cast<searchcore::IndexEngine*>(h)->term_count();
}

int index_doc_freq(IndexHandle h, const char* term)
{
    return static_cast<searchcore::IndexEngine*>(h)->doc_freq(term ? term : "");
}

int index_doc_length(IndexHandle h, int doc_id, int* out_len)
{
    return static_cast<searchcore::IndexEngine*>(h)->doc_length(doc_id, out_len) ? 1 : 0;
}

/* ---- Startup loading ---- */

void index_set_posting(IndexHandle h, const char* term, int doc_id, int term_freq)
{
    static_cast<searchcore::IndexEngine*>(h)->set_posting(term, doc_id, term_freq);
}

void index_set_doc_length(IndexHandle h, int doc_id, int length)
{
    static_cast<searchcore::IndexEngine*>(h)->set_doc_length(doc_id, length);
}

void index_set_snippet(IndexHandle h, int doc_id, const char* snippet, int snippet_len)
{
    static_cast<searchcore::IndexEngine*>(h)->set_snippet(
        doc_id, std::string(snippet, static_cast<size_t>(snippet_len)));
}

void index_set_corpus_stats(IndexHandle h, int total_docs, double avg_doc_len)
{
    static_cast<searchcore::IndexEngine*>(h)->set_corpus_stats(total_docs, avg_doc_len);
}
