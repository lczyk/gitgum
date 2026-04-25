package fuzzyfinder_test

import (
	"fmt"

	"github.com/lczyk/gitgum/src/fuzzyfinder"
)

func ExampleFind() {
	items := []string{"foo", "bar", "baz"}
	idx, _ := fuzzyfinder.Find(items)
	fmt.Println(items[idx])
}

func ExampleFindMulti() {
	items := []string{"foo", "bar", "baz"}
	idxs, _ := fuzzyfinder.FindMulti(items)
	for _, idx := range idxs {
		fmt.Println(items[idx])
	}
}
