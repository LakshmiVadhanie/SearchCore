package index

import (
	"math"
	"testing"
)

// bm25IDF replicates the IDF formula for test assertions.
// IDF(t) = ln((N - df + 0.5) / (df + 0.5) + 1)
func bm25IDF(N, df int) float64 {
	if df == 0 {
		return 0
	}
	return math.Log((float64(N-df)+0.5)/(float64(df)+0.5) + 1.0)
}

func TestBM25IDF_Formula(t *testing.T) {
	// Rare terms must score higher IDF than common terms.
	N := 1000
	rareIDF   := bm25IDF(N, 1)
	commonIDF := bm25IDF(N, 500)
	if rareIDF <= commonIDF {
		t.Errorf("rare IDF (%f) should exceed common IDF (%f)", rareIDF, commonIDF)
	}
	if bm25IDF(N, 0) != 0 {
		t.Errorf("IDF(df=0) should be 0")
	}
}

func TestSearch_Empty(t *testing.T) {
	idx := New()
	defer idx.Close()
	if hits := Search(idx, []string{"foo"}, 10); hits != nil {
		t.Errorf("expected nil hits on empty index, got %v", hits)
	}
	if hits := Search(idx, nil, 10); hits != nil {
		t.Errorf("expected nil hits for nil query, got %v", hits)
	}
}

func TestSearch_SingleDocument(t *testing.T) {
	idx := New()
	defer idx.Close()
	idx.AddDocument(1, []string{"search", "engine", "search", "fast"}, "search engine search fast")

	hits := Search(idx, []string{"search"}, 10)
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(hits))
	}
	if hits[0].DocID != 1 {
		t.Errorf("expected docID 1, got %d", hits[0].DocID)
	}
	if hits[0].Score <= 0 {
		t.Errorf("expected positive score, got %f", hits[0].Score)
	}
}

func TestSearch_Ranking(t *testing.T) {
	idx := New()
	defer idx.Close()
	idx.AddDocument(1, []string{"search", "engine"}, "search engine")
	idx.AddDocument(2, []string{"search", "search", "engine"}, "search search engine")

	hits := Search(idx, []string{"search"}, 10)
	if len(hits) != 2 {
		t.Fatalf("expected 2 hits, got %d", len(hits))
	}
	if hits[0].DocID != 2 {
		t.Errorf("expected doc 2 to rank first (higher tf), got doc %d", hits[0].DocID)
	}
}

func TestSearch_TopK(t *testing.T) {
	idx := New()
	defer idx.Close()
	for i := 1; i <= 10; i++ {
		idx.AddDocument(i, []string{"common", "term"}, "common term")
	}

	hits := Search(idx, []string{"common"}, 3)
	if len(hits) != 3 {
		t.Errorf("expected 3 hits (topK=3), got %d", len(hits))
	}
}

func TestSearch_MultiTermQuery(t *testing.T) {
	idx := New()
	defer idx.Close()
	idx.AddDocument(1, []string{"inverted", "index", "search"}, "inverted index search")
	idx.AddDocument(2, []string{"database", "index"}, "database index")

	hits := Search(idx, []string{"inverted", "index"}, 10)
	if len(hits) < 1 {
		t.Fatal("expected hits, got none")
	}
	if hits[0].DocID != 1 {
		t.Errorf("expected doc 1 to rank first, got doc %d", hits[0].DocID)
	}
}

func TestSearch_NoMatchingTerm(t *testing.T) {
	idx := New()
	defer idx.Close()
	idx.AddDocument(1, []string{"hello", "world"}, "hello world")

	hits := Search(idx, []string{"nosuchterm"}, 10)
	if len(hits) != 0 {
		t.Errorf("expected 0 hits, got %d", len(hits))
	}
}

func TestSearch_SnippetIncluded(t *testing.T) {
	idx := New()
	defer idx.Close()
	idx.AddDocument(1, []string{"search", "engine"}, "search engine document")

	hits := Search(idx, []string{"search"}, 10)
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(hits))
	}
	if hits[0].Snippet == "" {
		t.Error("expected non-empty snippet in search result")
	}
}

func TestIndex_AddRemove(t *testing.T) {
	idx := New()
	defer idx.Close()
	idx.AddDocument(1, []string{"foo", "bar", "foo"}, "foo bar foo")
	idx.AddDocument(2, []string{"bar", "baz"}, "bar baz")

	totalDocs, _ := idx.Snapshot()
	if totalDocs != 2 {
		t.Errorf("expected TotalDocs=2, got %d", totalDocs)
	}

	idx.RemoveDocument(1)
	totalDocs, _ = idx.Snapshot()
	if totalDocs != 1 {
		t.Errorf("expected TotalDocs=1 after remove, got %d", totalDocs)
	}

	if df := idx.DocFreq("foo"); df != 0 {
		t.Errorf("expected DocFreq('foo')=0 after remove, got %d", df)
	}
}

func TestIndex_RemoveNonExistent(t *testing.T) {
	idx := New()
	defer idx.Close()
	idx.RemoveDocument(999) // should not panic
}

func TestSearch_StemmedDocumentMatchesInflectedQuery(t *testing.T) {
	// Index a document using the full Tokenize pipeline (which stems).
	// Then search with a different inflection of the same word.
	// Both should hit the same stem, so the document must be found.
	idx := New()
	defer idx.Close()

	tokens := Tokenize("the dogs are running fast in the park")
	idx.AddDocument(1, tokens, "the dogs are running fast in the park")

	// Search for "run" — should match because "running" and "run" share the stem "run".
	queryTokens := Tokenize("run")
	hits := Search(idx, queryTokens, 10)
	if len(hits) == 0 {
		t.Error("expected a hit for stemmed query 'run' matching 'running' in document")
	}
}
