package index

/*
#cgo CFLAGS:  -I${SRCDIR}/cpp/include
#cgo LDFLAGS: ${SRCDIR}/../../lib/libsearchindex.a -lstdc++ -lm
#include "cpp/include/index_c_api.h"
#include <stdlib.h>
*/
import "C"
import (
	"runtime"
	"unsafe"
)

// Index is the in-memory inverted index backed by the C++ IndexEngine.
// All methods are safe for concurrent use.
type Index struct {
	handle C.IndexHandle
}

// New returns an empty, ready-to-use Index.
func New() *Index {
	idx := &Index{handle: C.index_new()}
	runtime.SetFinalizer(idx, func(i *Index) { i.Close() })
	return idx
}

// Close frees the underlying C++ IndexEngine. Safe to call multiple times.
func (idx *Index) Close() {
	if idx.handle != nil {
		C.index_free(idx.handle)
		idx.handle = nil
	}
}

// AddDocument indexes a document given its ID, pre-tokenized + pre-stemmed
// terms, and raw content.
func (idx *Index) AddDocument(docID int, tokens []string, content string) {
	if len(tokens) == 0 {
		return
	}

	// Build a C string array for the tokens.
	cTokens := make([]*C.char, len(tokens))
	for i, t := range tokens {
		cTokens[i] = C.CString(t)
	}
	defer func() {
		for _, ct := range cTokens {
			C.free(unsafe.Pointer(ct))
		}
	}()

	cContent := C.CString(content)
	defer C.free(unsafe.Pointer(cContent))

	C.index_add_document(
		idx.handle,
		C.int(docID),
		(**C.char)(unsafe.Pointer(&cTokens[0])),
		C.int(len(tokens)),
		cContent,
		C.int(len(content)),
	)
}

// RemoveDocument removes a document from the index. No-op if docID doesn't exist.
func (idx *Index) RemoveDocument(docID int) {
	C.index_remove_document(idx.handle, C.int(docID))
}

// Snapshot returns corpus-level statistics.
func (idx *Index) Snapshot() (totalDocs int, avgDocLen float64) {
	var ctd C.int
	var cal C.double
	C.index_snapshot(idx.handle, &ctd, &cal)
	return int(ctd), float64(cal)
}

// TermCount returns the number of unique indexed terms.
func (idx *Index) TermCount() int {
	return int(C.index_term_count(idx.handle))
}

// DocFreq returns the document frequency for a term.
func (idx *Index) DocFreq(term string) int {
	cs := C.CString(term)
	defer C.free(unsafe.Pointer(cs))
	return int(C.index_doc_freq(idx.handle, cs))
}

// DocLength returns the token count for a document.
func (idx *Index) DocLength(docID int) (int, bool) {
	var outLen C.int
	found := C.index_doc_length(idx.handle, C.int(docID), &outLen)
	return int(outLen), found == 1
}

// SetPosting appends a single posting entry (used only at startup loading).
func (idx *Index) SetPosting(term string, docID int, termFreq int) {
	cs := C.CString(term)
	defer C.free(unsafe.Pointer(cs))
	C.index_set_posting(idx.handle, cs, C.int(docID), C.int(termFreq))
}

// SetDocLength sets a document length (startup only).
func (idx *Index) SetDocLength(docID, length int) {
	C.index_set_doc_length(idx.handle, C.int(docID), C.int(length))
}

// SetSnippet sets a document snippet (startup only).
func (idx *Index) SetSnippet(docID int, snippet string) {
	cs := C.CString(snippet)
	defer C.free(unsafe.Pointer(cs))
	C.index_set_snippet(idx.handle, C.int(docID), cs, C.int(len(snippet)))
}

// SetCorpusStats sets TotalDocs and AvgDocLen (startup only).
func (idx *Index) SetCorpusStats(totalDocs int, avgDocLen float64) {
	C.index_set_corpus_stats(idx.handle, C.int(totalDocs), C.double(avgDocLen))
}

// Search returns the top-k documents ranked by BM25 score for the given query tokens.
func Search(idx *Index, query []string, topK int) []SearchResult {
	if len(query) == 0 || topK <= 0 || idx == nil {
		return nil
	}

	cTokens := make([]*C.char, len(query))
	for i, t := range query {
		cTokens[i] = C.CString(t)
	}
	defer func() {
		for _, ct := range cTokens {
			C.free(unsafe.Pointer(ct))
		}
	}()

	arr := C.index_search(
		idx.handle,
		(**C.char)(unsafe.Pointer(&cTokens[0])),
		C.int(len(query)),
		C.int(topK),
	)
	if arr == nil {
		return nil
	}
	defer C.index_free_results(arr)

	count := int(arr.count)
	if count == 0 {
		return nil
	}

	// Convert C results to Go structs before freeing.
	cSlice := (*[1 << 28]C.CSearchResult)(unsafe.Pointer(arr.results))[:count:count]
	out := make([]SearchResult, count)
	for i := 0; i < count; i++ {
		out[i] = SearchResult{
			DocID:   int(cSlice[i].doc_id),
			Score:   float64(cSlice[i].score),
			Snippet: C.GoString(cSlice[i].snippet),
		}
	}
	return out
}
