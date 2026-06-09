#include "tokenizer.hpp"
#include "../vendor/libstemmer/include/libstemmer.h"
#include <cctype>
#include <cstring>
#include <sstream>

namespace searchcore {

/* ---- Constructor / Destructor ----------------------------------------- */

Tokenizer::Tokenizer()
    : stemmer_(sb_stemmer_new("english", "UTF_8"))
{
    init_stop_words();
}

Tokenizer::~Tokenizer()
{
    if (stemmer_) {
        sb_stemmer_delete(stemmer_);
        stemmer_ = nullptr;
    }
}

/* ---- Public ------------------------------------------------------------ */

std::vector<std::string> Tokenizer::tokenize(const std::string& text) const
{
    /* Step 1+2: lowercase and replace non-alphanumeric bytes with spaces.
     * Multi-byte UTF-8 continuation bytes (0x80–0xBF) and leading bytes
     * (0xC0–0xFF) are preserved as-is, matching the Go behaviour of
     * unicode.IsLetter for non-ASCII runes. */
    std::string buf;
    buf.reserve(text.size());
    for (unsigned char c : text) {
        if (c < 0x80) {
            /* ASCII: lowercase letters and digits pass through; rest → space */
            if (std::isalpha(c)) {
                buf += static_cast<char>(std::tolower(static_cast<unsigned char>(c)));
            } else if (std::isdigit(c)) {
                buf += static_cast<char>(c);
            } else {
                buf += ' ';
            }
        } else {
            /* Multi-byte UTF-8 byte: preserve (letter-like, matches Go) */
            buf += static_cast<char>(c);
        }
    }

    /* Step 3: split on whitespace */
    std::vector<std::string> tokens;
    std::istringstream ss(buf);
    std::string tok;
    while (ss >> tok) {
        /* Step 4: remove stop words and empty tokens */
        if (tok.empty() || stop_words_.count(tok)) continue;

        /* Step 5: Snowball Porter2 stem */
        tokens.push_back(stem(tok));
    }
    return tokens;
}

/* ---- Private ----------------------------------------------------------- */

std::string Tokenizer::stem(const std::string& word) const
{
    if (!stemmer_) return word;
    const sb_symbol* result = sb_stemmer_stem(
        stemmer_,
        reinterpret_cast<const sb_symbol*>(word.c_str()),
        static_cast<int>(word.size())
    );
    if (!result) return word;
    return std::string(reinterpret_cast<const char*>(result),
                       sb_stemmer_length(stemmer_));
}

void Tokenizer::init_stop_words()
{
    static const char* const words[] = {
        "a","about","above","after","again","against","all","am","an","and",
        "any","are","aren't","as","at","be","because","been","before","being",
        "below","between","both","but","by","can't","cannot","could","couldn't",
        "did","didn't","do","does","doesn't","doing","don't","down","during",
        "each","few","for","from","further","get","got","had","hadn't","has",
        "hasn't","have","haven't","having","he","he'd","he'll","he's","her",
        "here","here's","hers","herself","him","himself","his","how","how's",
        "i","i'd","i'll","i'm","i've","if","in","into","is","isn't","it",
        "it's","its","itself","let's","me","more","most","mustn't","my",
        "myself","no","nor","not","of","off","on","once","only","or","other",
        "ought","our","ours","ourselves","out","over","own","same","shan't",
        "she","she'd","she'll","she's","should","shouldn't","so","some",
        "such","than","that","that's","the","their","theirs","them",
        "themselves","then","there","there's","these","they","they'd",
        "they'll","they're","they've","this","those","through","to","too",
        "under","until","up","very","was","wasn't","we","we'd","we'll",
        "we're","we've","were","weren't","what","what's","when","when's",
        "where","where's","which","while","who","who's","whom","why","why's",
        "will","with","won't","would","wouldn't","you","you'd","you'll",
        "you're","you've","your","yours","yourself","yourselves",
        nullptr
    };
    for (int i = 0; words[i]; ++i) {
        stop_words_.insert(words[i]);
    }
}

} /* namespace searchcore */
