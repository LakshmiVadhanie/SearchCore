package index

// PostingEntry records how often a term appears in a document.
// Used by main.go loadIndex when iterating store postings at startup.
type PostingEntry struct {
	DocID    int
	TermFreq int
}

// SearchResult is a ranked result returned by Search.
type SearchResult struct {
	DocID   int
	Score   float64
	Snippet string
}
