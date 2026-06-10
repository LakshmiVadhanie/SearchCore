package store

import (
	"database/sql"
	"fmt"

	_ "github.com/lib/pq"
)

// Document represents a row from the documents table.
type Document struct {
	ID      int
	Content string
}

// Posting represents a row from the postings table.
type Posting struct {
	Term     string
	DocID    int
	TermFreq int
}

// Store wraps a PostgreSQL connection and provides the SearchCore data layer.
type Store struct {
	db *sql.DB
}

// NewStore opens a connection pool to PostgreSQL and verifies connectivity.
func NewStore(dsn string) (*Store, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}
	return &Store{db: db}, nil
}

// Close closes the underlying connection pool.
func (s *Store) Close() error {
	return s.db.Close()
}

// RunMigrations executes the SQL in the provided migration string.
// In production use, call this once at startup with the embedded SQL.
func (s *Store) RunMigrations(sql string) error {
	_, err := s.db.Exec(sql)
	return err
}

// InsertDocument inserts a new document and returns its assigned ID.
func (s *Store) InsertDocument(content string) (int, error) {
	var id int
	err := s.db.QueryRow(
		`INSERT INTO documents (content) VALUES ($1) RETURNING id`,
		content,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert document: %w", err)
	}
	return id, nil
}

// UpsertPosting inserts or updates a posting entry for (term, docID).
func (s *Store) UpsertPosting(term string, docID, termFreq int) error {
	_, err := s.db.Exec(
		`INSERT INTO postings (term, doc_id, term_freq) VALUES ($1, $2, $3)
		 ON CONFLICT (term, doc_id) DO UPDATE SET term_freq = EXCLUDED.term_freq`,
		term, docID, termFreq,
	)
	if err != nil {
		return fmt.Errorf("upsert posting term=%q doc=%d: %w", term, docID, err)
	}
	return nil
}

// IncrementTermStat increments the document frequency for a term by 1.
// If the term does not yet exist in term_stats, it is inserted with doc_freq=1.
func (s *Store) IncrementTermStat(term string) error {
	_, err := s.db.Exec(
		`INSERT INTO term_stats (term, doc_freq) VALUES ($1, 1)
		 ON CONFLICT (term) DO UPDATE SET doc_freq = term_stats.doc_freq + 1`,
		term,
	)
	if err != nil {
		return fmt.Errorf("increment term stat %q: %w", term, err)
	}
	return nil
}

// DecrementTermStats decrements doc_freq for each term by 1, removing the row
// if doc_freq reaches 0.
func (s *Store) DecrementTermStats(terms []string) error {
	for _, term := range terms {
		_, err := s.db.Exec(
			`UPDATE term_stats SET doc_freq = doc_freq - 1 WHERE term = $1`,
			term,
		)
		if err != nil {
			return fmt.Errorf("decrement term stat %q: %w", term, err)
		}
		_, err = s.db.Exec(
			`DELETE FROM term_stats WHERE term = $1 AND doc_freq <= 0`,
			term,
		)
		if err != nil {
			return fmt.Errorf("cleanup term stat %q: %w", term, err)
		}
	}
	return nil
}

// UpdateCorpusStats upserts total_docs and avg_doc_length in corpus_stats.
func (s *Store) UpdateCorpusStats(totalDocs int, avgDocLen float64) error {
	_, err := s.db.Exec(
		`INSERT INTO corpus_stats (key, value) VALUES ('total_docs', $1), ('avg_doc_length', $2)
		 ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`,
		totalDocs, avgDocLen,
	)
	if err != nil {
		return fmt.Errorf("update corpus stats: %w", err)
	}
	return nil
}

// LoadAllDocuments returns all documents from the store.
func (s *Store) LoadAllDocuments() ([]Document, error) {
	rows, err := s.db.Query(`SELECT id, content FROM documents ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("load documents: %w", err)
	}
	defer rows.Close()

	var docs []Document
	for rows.Next() {
		var d Document
		if err := rows.Scan(&d.ID, &d.Content); err != nil {
			return nil, fmt.Errorf("scan document: %w", err)
		}
		docs = append(docs, d)
	}
	return docs, rows.Err()
}

// LoadAllPostings returns all posting entries from the store.
func (s *Store) LoadAllPostings() ([]Posting, error) {
	rows, err := s.db.Query(`SELECT term, doc_id, term_freq FROM postings`)
	if err != nil {
		return nil, fmt.Errorf("load postings: %w", err)
	}
	defer rows.Close()

	var postings []Posting
	for rows.Next() {
		var p Posting
		if err := rows.Scan(&p.Term, &p.DocID, &p.TermFreq); err != nil {
			return nil, fmt.Errorf("scan posting: %w", err)
		}
		postings = append(postings, p)
	}
	return postings, rows.Err()
}

// LoadCorpusStats returns the corpus_stats table as a key→value map.
func (s *Store) LoadCorpusStats() (map[string]float64, error) {
	rows, err := s.db.Query(`SELECT key, value FROM corpus_stats`)
	if err != nil {
		return nil, fmt.Errorf("load corpus stats: %w", err)
	}
	defer rows.Close()

	stats := make(map[string]float64)
	for rows.Next() {
		var key string
		var value float64
		if err := rows.Scan(&key, &value); err != nil {
			return nil, fmt.Errorf("scan corpus stat: %w", err)
		}
		stats[key] = value
	}
	return stats, rows.Err()
}

// LoadTermStats returns the term_stats table as a term→doc_freq map.
func (s *Store) LoadTermStats() (map[string]int, error) {
	rows, err := s.db.Query(`SELECT term, doc_freq FROM term_stats`)
	if err != nil {
		return nil, fmt.Errorf("load term stats: %w", err)
	}
	defer rows.Close()

	stats := make(map[string]int)
	for rows.Next() {
		var term string
		var df int
		if err := rows.Scan(&term, &df); err != nil {
			return nil, fmt.Errorf("scan term stat: %w", err)
		}
		stats[term] = df
	}
	return stats, rows.Err()
}

// DeleteDocument deletes a document by ID. The ON DELETE CASCADE on postings
// automatically removes all associated posting rows.
func (s *Store) DeleteDocument(docID int) error {
	res, err := s.db.Exec(`DELETE FROM documents WHERE id = $1`, docID)
	if err != nil {
		return fmt.Errorf("delete document %d: %w", docID, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("document %d not found", docID)
	}
	return nil
}

// LoadAllDocumentSnippets returns a map of doc_id -> first 200 chars of content
// for all documents. Used to pre-warm the in-memory snippet cache at startup.
func (s *Store) LoadAllDocumentSnippets() (map[int]string, error) {
	rows, err := s.db.Query(`SELECT id, LEFT(content, 200) FROM documents ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("load snippets: %w", err)
	}
	defer rows.Close()

	snippets := make(map[int]string)
	for rows.Next() {
		var id int
		var snippet string
		if err := rows.Scan(&id, &snippet); err != nil {
			return nil, fmt.Errorf("scan snippet: %w", err)
		}
		snippets[id] = snippet
	}
	return snippets, rows.Err()
}

// GetDocumentContent returns the content of a single document.
func (s *Store) GetDocumentContent(docID int) (string, error) {
	var content string
	err := s.db.QueryRow(`SELECT content FROM documents WHERE id = $1`, docID).Scan(&content)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("document %d not found", docID)
	}
	if err != nil {
		return "", fmt.Errorf("get document %d: %w", docID, err)
	}
	return content, nil
}

// GetDocumentTerms returns all terms stored for a given document (from postings).
func (s *Store) GetDocumentTerms(docID int) ([]string, error) {
	rows, err := s.db.Query(`SELECT term FROM postings WHERE doc_id = $1`, docID)
	if err != nil {
		return nil, fmt.Errorf("get terms for doc %d: %w", docID, err)
	}
	defer rows.Close()

	var terms []string
	for rows.Next() {
		var term string
		if err := rows.Scan(&term); err != nil {
			return nil, err
		}
		terms = append(terms, term)
	}
	return terms, rows.Err()
}
