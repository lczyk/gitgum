package fuzzyfinder_test

import (
	"testing"

	"github.com/lczyk/assert"
	fuzzyfinder "github.com/lczyk/gitgum/src/fuzzyfinder"
)

func TestSubstringMatcher(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		query string
		item  string
		want  bool
	}{
		"empty query matches anything":    {"", "anything", true},
		"single word substring":           {"foo", "foobar", true},
		"single word missing":             {"baz", "foobar", false},
		"case insensitive":                {"FOO", "foobar", true},
		"item case insensitive":           {"foo", "FOOBAR", true},
		"all words present in any order":  {"bar foo", "foobar", true},
		"some word missing":               {"foo qux", "foobar", false},
		"whitespace ignored":              {"  foo   bar  ", "foobar", true},
		"unicode case insensitive":        {"Ä", "fußÄnger", true},
		"empty item with non-empty query": {"foo", "", false},
		"empty item with empty query":     {"", "", true},
		"multibyte words":                 {"日本", "日本語の本", true},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, c.want, fuzzyfinder.SubstringMatcher(c.query, c.item))
		})
	}
}
