package commands

import "fmt"

// localRef names a local branch the way gg names a remote one: with its "remote"
// in front, so 'local/main' next to 'origin/main' reads as two branches of the
// same name on two sides rather than one branch mentioned twice. "local" is a
// name a real remote could carry; doctor flags one that does.
func localRef(branch string) string {
	return "local/" + branch
}

// localCommits renders a count of commits for a message: "1 local commit",
// "3 local commits".
func localCommits(n int) string {
	return fmt.Sprintf("%d local %s", n, plural(n, "commit", "commits"))
}
