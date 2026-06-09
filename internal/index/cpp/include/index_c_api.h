/* index_c_api.h — Pure C API for the C++ inverted index.
 * Included by CGo; must not contain any C++ types. */
#ifndef INDEX_C_API_H
#define INDEX_C_API_H

#ifdef __cplusplus
extern "C" {
#endif

/* Opaque handle to the C++ IndexEngine */
typedef void* IndexHandle;

/* ---- Lifecycle ---- */
IndexHandle index_new(void);
void        index_free(IndexHandle h);

/* ---- Tokenize ----
 * Lowercase → strip non-alnum → split → remove stop-words → Snowball stem.
 * Returns the number of tokens; fills *tokens_out with a malloc'd array of
 * C strings. Caller must free with index_free_tokens(). */
int  index_tokenize(const char* text, char*** tokens_out);
void index_free_tokens(char** tokens, int count);

/* ---- Document operations ---- */
void index_add_document(IndexHandle h, int doc_id,
                        const char** tokens, int n_tokens,
                        const char* content, int content_len);
void index_remove_document(IndexHandle h, int doc_id);

/* ---- Search ----
 * Returns a heap-allocated SearchResultArray. Caller must free with
 * index_free_results(). Returns NULL if there are no results. */
typedef struct {
    int    doc_id;
    double score;
    char*  snippet; /* heap-allocated; freed by index_free_results */
} CSearchResult;

typedef struct {
    CSearchResult* results;
    int            count;
} SearchResultArray;

SearchResultArray* index_search(IndexHandle h,
                                const char** query_tokens, int n_tokens,
                                int top_k);
void index_free_results(SearchResultArray* r);

/* ---- Corpus statistics ---- */
void index_snapshot(IndexHandle h, int* total_docs_out, double* avg_doc_len_out);
int  index_term_count(IndexHandle h);
int  index_doc_freq(IndexHandle h, const char* term);
/* Returns 1 if found (sets *out_len), 0 if not found. */
int  index_doc_length(IndexHandle h, int doc_id, int* out_len);

/* ---- Startup loading (single-threaded, called before server starts) ---- */
void index_set_posting(IndexHandle h, const char* term, int doc_id, int term_freq);
void index_set_doc_length(IndexHandle h, int doc_id, int length);
void index_set_snippet(IndexHandle h, int doc_id, const char* snippet, int snippet_len);
void index_set_corpus_stats(IndexHandle h, int total_docs, double avg_doc_len);

#ifdef __cplusplus
}
#endif

#endif /* INDEX_C_API_H */
