package main

import (
	"fmt"
	"os"

	flags "github.com/jessevdk/go-flags"
	"github.com/lczyk/gitgum/src/commands"
	"github.com/lczyk/gitgum/src/version"
)

// Options defines the global command structure
type Options struct {
	Switch     commands.SwitchCommand     `command:"switch" description:"Switch to a branch interactively"`
	CheckoutPR commands.CheckoutPRCommand `command:"checkout-pr" description:"Checkout a pull request from a remote repository"`
	Completion commands.CompletionCommand `command:"completion" description:"Output shell completion script"`
	Status     commands.StatusCommand     `command:"status" description:"Show the status of the current git repository"`
	Push       commands.PushCommand       `command:"push" description:"Push the current branch to a remote repository"`
}

func main() {
	// Check for version flag before parsing to avoid command requirement
	for _, arg := range os.Args[1:] {
		if arg == "--version" || arg == "-v" {
			fmt.Println(version.GetFullVersion())
			os.Exit(0)
		}
	}

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
