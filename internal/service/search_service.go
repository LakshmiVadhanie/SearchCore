package service

import (
	"context"
	"fmt"

	"github.com/dhruvgorasiya/searchcore/internal/index"
	"github.com/dhruvgorasiya/searchcore/internal/store"
	pb "github.com/dhruvgorasiya/searchcore/pb"
)

// SearchService implements the gRPC SearchServiceServer interface.
type SearchService struct {
	pb.UnimplementedSearchServiceServer
	idx   *index.Index
	store *store.Store
}

// New creates a SearchService wired to the given index and store.
func New(idx *index.Index, st *store.Store) *SearchService {
	return &SearchService{idx: idx, store: st}
}

// IndexDocument tokenizes the document, persists it, and updates the in-memory index.
func (s *SearchService) IndexDocument(ctx context.Context, req *pb.IndexRequest) (*pb.IndexResponse, error) {
	if req.Content == "" {
		return &pb.IndexResponse{Success: false, Message: "content must not be empty"}, nil
	}

	tokens := index.Tokenize(req.Content)
	if len(tokens) == 0 {
		return &pb.IndexResponse{Success: false, Message: "document produced no indexable tokens"}, nil
	}

	// Persist document.
	docID, err := s.store.InsertDocument(req.Content)
	if err != nil {
		return nil, fmt.Errorf("insert document: %w", err)
	}

	// Compute per-term frequencies.
	tf := make(map[string]int, len(tokens))
	for _, t := range tokens {
		tf[t]++
	}

	// Upsert posting entries and term stats.
	for term, freq := range tf {
		if err := s.store.UpsertPosting(term, docID, freq); err != nil {
			return nil, fmt.Errorf("upsert posting: %w", err)
		}
		if err := s.store.IncrementTermStat(term); err != nil {
			return nil, fmt.Errorf("increment term stat: %w", err)
		}
	}

	// Update in-memory index (also caches snippet and updates corpus stats).
	s.idx.AddDocument(docID, tokens, req.Content)

	// Persist updated corpus stats.
	totalDocs, avgDocLen := s.idx.Snapshot()
	if err := s.store.UpdateCorpusStats(totalDocs, avgDocLen); err != nil {
		return nil, fmt.Errorf("update corpus stats: %w", err)
	}

	return &pb.IndexResponse{
		DocId:   int32(docID),
		Success: true,
		Message: fmt.Sprintf("indexed %d tokens", len(tokens)),
	}, nil
}

// Search returns the top-k BM25-ranked results for the given query.
func (s *SearchService) Search(ctx context.Context, req *pb.SearchRequest) (*pb.SearchResponse, error) {
	topK := int(req.TopK)
	if topK <= 0 {
		topK = 10
	}

	tokens := index.Tokenize(req.Query)
	hits := index.Search(s.idx, tokens, topK)

	results := make([]*pb.SearchResult, 0, len(hits))
	for _, hit := range hits {
		results = append(results, &pb.SearchResult{
			DocId:   int32(hit.DocID),
			Score:   float32(hit.Score),
			Snippet: hit.Snippet,
		})
	}

	return &pb.SearchResponse{
		Results: results,
		Total:   int32(len(results)),
	}, nil
}

// DeleteDocument removes a document from the store and the in-memory index.
func (s *SearchService) DeleteDocument(ctx context.Context, req *pb.DeleteRequest) (*pb.DeleteResponse, error) {
	docID := int(req.DocId)

	// Fetch the terms for this document before deletion (for stat cleanup).
	terms, err := s.store.GetDocumentTerms(docID)
	if err != nil {
		return &pb.DeleteResponse{Success: false, Message: err.Error()}, nil
	}

	// Remove from DB (cascades to postings).
	if err := s.store.DeleteDocument(docID); err != nil {
		return &pb.DeleteResponse{Success: false, Message: err.Error()}, nil
	}

	// Decrement term stats for affected terms.
	if err := s.store.DecrementTermStats(terms); err != nil {
		return nil, fmt.Errorf("decrement term stats: %w", err)
	}

	// Remove from in-memory index.
	s.idx.RemoveDocument(docID)

	// Persist updated corpus stats.
	totalDocs, avgDocLen := s.idx.Snapshot()
	if err := s.store.UpdateCorpusStats(totalDocs, avgDocLen); err != nil {
		return nil, fmt.Errorf("update corpus stats after delete: %w", err)
	}

	return &pb.DeleteResponse{
		Success: true,
		Message: fmt.Sprintf("document %d deleted", docID),
	}, nil
}

// Stats returns corpus-level metadata from the in-memory index.
func (s *SearchService) Stats(ctx context.Context, req *pb.StatsRequest) (*pb.StatsResponse, error) {
	totalDocs, avgDocLen := s.idx.Snapshot()
	return &pb.StatsResponse{
		TotalDocs:    int32(totalDocs),
		TotalTerms:   int32(s.idx.TermCount()),
		AvgDocLength: float32(avgDocLen),
	}, nil
}
