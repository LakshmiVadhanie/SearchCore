package index

/*
#cgo CFLAGS: -I${SRCDIR}/cpp/include
#include "cpp/include/index_c_api.h"
#include <stdlib.h>
*/
import "C"
import "unsafe"

// Tokenize applies the C++ tokenization pipeline:
// lowercase → strip non-alnum → split → remove stop words → Snowball Porter2 stem.
//
// Applied identically at index time and query time.
func Tokenize(text string) []string {
	cs := C.CString(text)
	defer C.free(unsafe.Pointer(cs))

	var ctokens **C.char
	count := int(C.index_tokenize(cs, &ctokens))
	if count == 0 || ctokens == nil {
		return nil
	}
	defer C.index_free_tokens(ctokens, C.int(count))

	// Convert to Go strings before the C memory is freed.
	cSlice := (*[1 << 20]*C.char)(unsafe.Pointer(ctokens))[:count:count]
	out := make([]string, count)
	for i, cp := range cSlice {
		out[i] = C.GoString(cp)
	}
	return out
}
