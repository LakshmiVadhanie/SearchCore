/* Minimal modules.h for English UTF-8 stemmer only.
 * Generated function names come from stem_UTF_8_english.h (create_env / close_env / stem). */
#ifndef MODULES_H
#define MODULES_H

#include "libstemmer.h"

typedef enum {
    ENC_UTF_8,
    ENC_ISO_8859_1,
    ENC_UNKNOWN
} stemmer_encoding_t;

struct stemmer_encoding {
    const char * name;
    stemmer_encoding_t enc;
};

struct stemmer_modules {
    const char * name;
    stemmer_encoding_t enc;
    struct SN_env * (*create)(void);
    void (*close)(struct SN_env *);
    int (*stem)(struct SN_env *);
};

/* Declared in stem_UTF_8_english.h */
extern struct SN_env * create_env(void);
extern void            close_env(struct SN_env *);
extern int             stem(struct SN_env *);

static const struct stemmer_encoding encodings[] = {
    { "UTF_8", ENC_UTF_8 },
    { "UTF-8", ENC_UTF_8 },
    { 0,       0         }
};

static const char * algorithm_name_list[] = { "english", "en", 0 };
static const char ** algorithm_names = algorithm_name_list;

static const struct stemmer_modules modules[] = {
    { "english", ENC_UTF_8, create_env, close_env, stem },
    { "en",      ENC_UTF_8, create_env, close_env, stem },
    { 0, 0, 0, 0, 0 }
};

#endif /* MODULES_H */
