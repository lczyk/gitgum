package commands

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/lczyk/assert"
	"github.com/lczyk/assert/require"
	"github.com/lczyk/gitgum/internal/git"
	"github.com/lczyk/gitgum/internal/testutil/temp_repo"
)

// `git branch -r` lists every remote whichever one you ask about, so a
// producer per remote used to mean a full listing per remote. The local half
// is one ref read too: the tracking remote used to be a subprocess per branch.
func TestStreamBranches_ReadsRefsOnce(t *testing.T) {
	local, remote := temp_repo.NewRepoWithRemote(t)
	temp_repo.RunGit(t, local, "remote", "add", "second", remote)
	temp_repo.RunGit(t, local, "fetch", "--all")
	temp_repo.RunGit(t, local, "branch", "feature")
	temp_repo.RunGit(t, local, "branch", "other")

	r := git.Repo{Dir: local}
	remotes, err := r.GetRemotes()
	require.NoError(t, err, "listing remotes")
	assert.Equal(t, len(remotes), 2)

	calls := gitShim(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var errBuf bytes.Buffer
	src := streamBranches(ctx, r, &errBuf, "main", "", remotes, branchStreamOpts{})

	var sawRemote bool
	for range 50 {
		time.Sleep(10 * time.Millisecond)
		for _, item := range src.Snapshot() {
			if strings.HasPrefix(item, "remote: second/") {
				sawRemote = true
			}
		}
		if sawRemote {
			break
		}
	}
	assert.That(t, sawRemote, "picker should list the second remote's branches, got %v", src.Snapshot())
	assert.Equal(t, errBuf.String(), "")

	assert.Equal(t, countExact(calls(), "branch", "-r"), 1)
	assert.Equal(t, countCalls(calls(), "for-each-ref"), 1)
}

// delete reads the local branches up front for its empty-repo guard; the
// picker takes those rather than asking again.
func TestStreamBranches_TakesCallerLocals(t *testing.T) {
	dir := temp_repo.NewRepo(t)
	temp_repo.RunGit(t, dir, "branch", "feature")

	locals, err := git.Repo{Dir: dir}.LocalBranches()
	require.NoError(t, err, "listing local branches")

	calls := gitShim(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var errBuf bytes.Buffer
	streamBranches(ctx, git.Repo{Dir: dir}, &errBuf, "main", "", nil,
		branchStreamOpts{locals: locals})

	assert.Equal(t, countCalls(calls(), "for-each-ref"), 0)
}

// Every doctor check reads from one set of answers: seven checks wanting the
// same three listings used to take nine.
func TestDoctorCommand_ReadsEachFactOnce(t *testing.T) {
	local, remote := temp_repo.NewRepoWithRemote(t)
	temp_repo.RunGit(t, local, "remote", "add", "second", remote)
	temp_repo.RunGit(t, local, "branch", "feature")

	calls := gitShim(t)
	var buf strings.Builder
	cmd := &DoctorCommand{cmdIO: cmdIO{Out: &buf, Repo: git.Repo{Dir: local}}}
	require.NoError(t, cmd.Execute(nil), "doctor should succeed")

	assert.Equal(t, countExact(calls(), "remote"), 1)
	assert.Equal(t, countExact(calls(), "remote", "get-url", "origin"), 1)
	assert.Equal(t, countExact(calls(), "remote", "get-url", "second"), 1)
	assert.Equal(t, countCalls(calls(), "for-each-ref"), 1)
	assert.Equal(t, countCalls(calls(), "worktree"), 1)
}
