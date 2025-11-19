package main

import (
	"os"

	flags "github.com/jessevdk/go-flags"
	"github.com/lczyk/gitgum/src/commands"
)

// Options defines the global command structure
type Options struct {
	Switch     commands.SwitchCommand     `command:"switch" description:"Switch to a branch interactively"`
	Completion commands.CompletionCommand `command:"completion" description:"Output shell completion script"`
	Status     commands.StatusCommand     `command:"status" description:"Show the status of the current git repository"`
	Push       commands.PushCommand       `command:"push" description:"Push the current branch to a remote repository"`
}

func main() {
	var opts Options
	parser := flags.NewParser(&opts, flags.Default)
	parser.Name = "gitgum"
	parser.Usage = "[OPTIONS] COMMAND"

	_, err := parser.Parse()
	if err != nil {
		// go-flags already prints the error
		if flagsErr, ok := err.(*flags.Error); ok {
			if flagsErr.Type == flags.ErrHelp {
				os.Exit(0)
			}
		}
		os.Exit(1)
	}
}
