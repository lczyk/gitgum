package fuzzyfinder_test

import (
	"context"
	"fmt"

	ff "github.com/lczyk/gitgum/src/fuzzyfinder"
)

func ExampleFind() {
	items := []string{"foo", "bar", "baz"}
	idxs, _ := ff.Find(context.Background(), items, ff.Opt{})
	fmt.Println(items[idxs[0]])
}

func ExampleFind_multi() {
	items := []string{"foo", "bar", "baz"}
	idxs, _ := ff.Find(context.Background(), items, ff.Opt{Multi: true})
	for _, idx := range idxs {
		fmt.Println(items[idx])
	}
}
