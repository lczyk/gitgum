package filetree

import (
	"strconv"
	"unicode"
)

// QuoteName wraps s in double quotes when drawing it bare would be ambiguous:
// whitespace, a quote, a backslash, or anything unprintable. Everything else
// is returned untouched, so ordinary names stay unadorned.
//
// Exported because callers compose Prefix and Suffix themselves and may need a
// name inside them -- a rename's source, say -- quoted the same way.
//
// This is deliberately not git's C-quoting, which escapes anything non-ascii
// to octal: "é" comes back as "\303\251", which is unreadable in a listing
// whose purpose is recognising your own files. Go quoting keeps printable
// unicode as itself.
func QuoteName(s string) string {
	if !needsQuote(s) {
		return s
	}
	return strconv.Quote(s)
}

func needsQuote(s string) bool {
	for _, r := range s {
		if r == '"' || r == '\\' || unicode.IsSpace(r) || !unicode.IsPrint(r) {
			return true
		}
	}
	return false
}
