package index

import (
	"reflect"
	"strings"
	"testing"
)

func TestTokenize(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "empty string",
			input: "",
			want:  []string{},
		},
		{
			name:  "only stop words",
			input: "the a an is are",
			want:  []string{},
		},
		{
			name:  "lowercasing",
			input: "Hello World",
			// Porter2: hello→hello, world→world
			want: []string{"hello", "world"},
		},
		{
			name:  "punctuation stripped",
			input: "hello, world! foo.bar",
			// Porter2: all unchanged
			want: []string{"hello", "world", "foo", "bar"},
		},
		{
			name:  "mixed stop words and content",
			input: "the quick brown fox jumps over the lazy dog",
			// Porter2: jumps→jump, lazy→lazi
			want: []string{"quick", "brown", "fox", "jump", "lazi", "dog"},
		},
		{
			name:  "numbers preserved",
			input: "go1 version 123",
			// Porter2: go1→go1, version→version, 123→123
			want: []string{"go1", "version", "123"},
		},
		{
			name:  "hyphenated words split",
			input: "state-of-the-art design",
			// "of" and "the" are stop words; state→state, art→art, design→design
			want: []string{"state", "art", "design"},
		},
		{
			name:  "extra whitespace",
			input: "  hello   world  ",
			want:  []string{"hello", "world"},
		},
		{
			name:  "all punctuation",
			input: "!@#$%^&*()",
			want:  []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Tokenize(tt.input)
			if len(got) == 0 && len(tt.want) == 0 {
				return // both empty — pass
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Tokenize(%q)\n  got  %v\n  want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestTokenize_Unicode(t *testing.T) {
	// Non-ASCII letters should pass through the tokenizer without crashing.
	// We do not assert exact stems because Porter2 operates on ASCII sequences;
	// multi-byte bytes are preserved as-is by our tokenizer, matching Go behaviour.
	got := Tokenize("café naïve")
	if len(got) != 2 {
		t.Fatalf("expected 2 tokens, got %d: %v", len(got), got)
	}
	if !strings.HasPrefix(got[0], "caf") {
		t.Errorf("expected first token to start with 'caf', got %q", got[0])
	}
	if !strings.HasPrefix(got[1], "na") {
		t.Errorf("expected second token to start with 'na', got %q", got[1])
	}
}

func TestTokenize_StopWordConsistency(t *testing.T) {
	// Same query at index time and search time must produce identical tokens.
	query := "what is the best search engine for full text retrieval"
	a := Tokenize(query)
	b := Tokenize(query)
	if !reflect.DeepEqual(a, b) {
		t.Errorf("Tokenize is non-deterministic: %v vs %v", a, b)
	}
}

func TestTokenize_StemConsistency(t *testing.T) {
	// Words with the same stem should produce the same token, ensuring that
	// indexing "running" and querying "run" both map to the same posting list.
	stems := []struct{ a, b string }{
		{"running", "run"},
		{"searches", "search"},
		{"indexed", "index"},
	}
	for _, s := range stems {
		ta := Tokenize(s.a)
		tb := Tokenize(s.b)
		if len(ta) != 1 || len(tb) != 1 {
			t.Errorf("expected 1 token each for %q/%q, got %v/%v", s.a, s.b, ta, tb)
			continue
		}
		if ta[0] != tb[0] {
			t.Errorf("%q and %q should produce the same stem: got %q vs %q",
				s.a, s.b, ta[0], tb[0])
		}
	}
}
