
- Don't add new features to the Bash version - only to the Go version.
- The new Go version does *not* necessarily implement all the features of the Bash version. This is intentional. Use the bash version as a vague reference only.
- Remember to update completions files when implementing / updating new commands.
- Never edit `generated.go` directly.  Edit templates and regenerate.
- Use `github.com/lczyk/assert` package (custom assertion library) for tests.
- Use table-driven tests with subtests for multiple cases
- gitgum uses external `git` and `fzf` commands. Assume they are in PATH. Don't try to reimplement their functionality.
- we use major.minor.patch versioning. any breaking changes should bump the minor version. any non-breaking changes should bump the patch version. every now and again the major version is bumped manually.
- the go version does not support --dry-run flag