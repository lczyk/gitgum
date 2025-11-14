package main

import (
	"fmt"
	"os"

	flags "github.com/jessevdk/go-flags"
)

type Options struct {
}

func main() {
	var opts Options
	parser := flags.NewParser(&opts, flags.Default)
	
	_, err := parser.Parse()
	if err != nil {
		os.Exit(1)
	}

	fmt.Println(opts)
}