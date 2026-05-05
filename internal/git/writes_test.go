package git_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/gitgum/internal/git"
	"github.com/lczyk/gitgum/internal/testutil/temp_repo"
)

func TestAdd_StagesPath(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	temp_repo.WriteFile(t, dir, "foo.txt", "x")

	assert.NoError(t, git.Repo{Dir: dir}.Add("foo.txt"))

	out := temp_repo.RunGit(t, dir, "status", "--porcelain")
	assert.ContainsString(t, out, "A  foo.txt")
}

func TestAdd_NonexistentPath(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	err := git.Repo{Dir: dir}.Add("nope.txt")
	assert.Error(t, err, assert.AnyError, "missing path should error")
}

func TestCommit_WithStagedFile(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	temp_repo.WriteFile(t, dir, "foo.txt", "x")
	temp_repo.RunGit(t, dir, "add", "foo.txt")

	assert.NoError(t, git.Repo{Dir: dir}.Commit("chore: add foo"))

	subject := strings.TrimSpace(temp_repo.RunGit(t, dir, "log", "-1", "--format=%s"))
	assert.Equal(t, subject, "chore: add foo")
}

func TestCommitEmpty_NoChanges(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)

	assert.NoError(t, git.Repo{Dir: dir}.CommitEmpty("chore: empty bump"))

	subject := strings.TrimSpace(temp_repo.RunGit(t, dir, "log", "-1", "--format=%s"))
	assert.Equal(t, subject, "chore: empty bump")
}

func TestResetHard_ToParent(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	temp_repo.CreateCommit(t, dir, "second.txt", "x", "chore: second")
	firstSHA := strings.TrimSpace(temp_repo.RunGit(t, dir, "rev-parse", "HEAD~1"))

	assert.NoError(t, git.Repo{Dir: dir}.ResetHard("HEAD~1"))

	headSHA := strings.TrimSpace(temp_repo.RunGit(t, dir, "rev-parse", "HEAD"))
	assert.Equal(t, headSHA, firstSHA)
}

func TestResetHard_BadRef(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	err := git.Repo{Dir: dir}.ResetHard("nonexistent-ref")
	assert.Error(t, err, assert.AnyError, "bad ref should error")
}

func TestCheckoutNewBranch_FromHEAD(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)

	assert.NoError(t, git.Repo{Dir: dir}.CheckoutNewBranch("feat", "HEAD"))

	branch := strings.TrimSpace(temp_repo.RunGit(t, dir, "rev-parse", "--abbrev-ref", "HEAD"))
	assert.Equal(t, branch, "feat")
}

func TestCheckoutNewBranch_AlreadyExists(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	temp_repo.RunGit(t, dir, "branch", "feat")

	err := git.Repo{Dir: dir}.CheckoutNewBranch("feat", "HEAD")
	assert.Error(t, err, assert.AnyError, "duplicate branch should error")
}

func TestCheckout_ExistingBranch(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	temp_repo.RunGit(t, dir, "branch", "dev")

	assert.NoError(t, git.Repo{Dir: dir}.Checkout("dev"))

	branch := strings.TrimSpace(temp_repo.RunGit(t, dir, "rev-parse", "--abbrev-ref", "HEAD"))
	assert.Equal(t, branch, "dev")
}

func TestCheckout_MissingBranch(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	err := git.Repo{Dir: dir}.Checkout("nope")
	assert.Error(t, err, assert.AnyError, "missing branch should error")
}

func TestTagAnnotated_Creates(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)

	assert.NoError(t, git.Repo{Dir: dir}.TagAnnotated("v1.0.0", "release v1.0.0"))

	tags := temp_repo.RunGit(t, dir, "tag", "-l")
	assert.ContainsString(t, tags, "v1.0.0")
	objType := strings.TrimSpace(temp_repo.RunGit(t, dir, "cat-file", "-t", "v1.0.0"))
	assert.Equal(t, objType, "tag")
}

func TestTagExists_PresentAndAbsent(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	temp_repo.RunGit(t, dir, "tag", "v1")

	r := git.Repo{Dir: dir}
	assert.That(t, r.TagExists("v1"), "v1 should exist")
	assert.That(t, !r.TagExists("v2"), "v2 should be absent")
}

// Sanity: covers the path filepath.Join is needed for in case test runner cwd
// differs. Confirms Repo.Dir is honoured even when test cwd is elsewhere.
func TestAdd_RespectsRepoDir(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	temp_repo.WriteFile(t, dir, "bar.txt", "y")

	r := git.Repo{Dir: dir}
	assert.NoError(t, r.Add(filepath.Join("bar.txt")))

	out := temp_repo.RunGit(t, dir, "status", "--porcelain")
	assert.ContainsString(t, out, "A  bar.txt")
}
