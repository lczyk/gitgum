package commands

import (
	"fmt"
	"path/filepath"

	"github.com/lczyk/gitgum/internal/doctor"
)

// cdWrapperEnv is set by the shell function `gg completion --cd` prints on
// every call it passes through, which is the only way a child process can
// learn the function exists.
const cdWrapperEnv = "GITGUM_CD_WRAPPER"

// checkCdWrapper reports the cd wrapper as missing when the marker is absent.
// It lives here rather than in the doctor package because it is about the
// shell gg was called from, not the repo. The fix names the user's login
// shell; an unrecognised one gets the generic form.
func checkCdWrapper(getenv func(string) string) []doctor.Finding {
	if getenv(cdWrapperEnv) != "" {
		return nil
	}
	fix := "gg completion --cd <shell>  # then source it, see README"
	switch shell := filepath.Base(getenv("SHELL")); shell {
	case "fish":
		fix = "gg completion --cd fish | source"
	case "bash", "zsh":
		fix = fmt.Sprintf(`eval "$(gg completion --cd %s)"`, shell)
	case "nu":
		fix = "gg completion --cd nu | save -f ~/.gitgum-cd.nu  # then `source ~/.gitgum-cd.nu` in config.nu"
	}
	return []doctor.Finding{{Check: "cd-wrapper", Severity: doctor.SevFixable,
		Message: "the shell function that lets `gg worktree-switch` / `gg w` change directory is not installed in this shell",
		Fix:     fix}}
}
