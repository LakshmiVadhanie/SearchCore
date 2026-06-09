#pragma once
#include <string>
#include <vector>
#include <unordered_set>

struct sb_stemmer; /* forward-declare to avoid including libstemmer.h in user headers */

namespace searchcore {

class Tokenizer {
public:
    Tokenizer();
    ~Tokenizer();

    /* Pipeline: lowercase → strip non-alnum → split → remove stop-words → stem.
     * Thread-safety: NOT thread-safe (sb_stemmer is stateful). Use one
     * Tokenizer per thread, or protect with a mutex. */
    std::vector<std::string> tokenize(const std::string& text) const;

private:
    struct sb_stemmer* stemmer_;
    std::unordered_set<std::string> stop_words_;

    void init_stop_words();
    std::string stem(const std::string& word) const;
};

} /* namespace searchcore */
