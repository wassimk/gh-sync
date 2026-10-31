package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wassimk/gh-sync/internal/git"
)

// ---------------------------------------------------------------------------
// Unit tests
// ---------------------------------------------------------------------------

func TestParseArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    options
		wantErr bool
		isHelp  bool
	}{
		{"no flags", nil, options{}, false, false},
		{"verbose long", []string{"--verbose"}, options{verbose: true}, false, false},
		{"verbose short", []string{"-v"}, options{verbose: true}, false, false},
		{"dry run long", []string{"--dry-run"}, options{dryRun: true}, false, false},
		{"dry run short", []string{"-n"}, options{dryRun: true}, false, false},
		{"both", []string{"-v", "-n"}, options{verbose: true, dryRun: true}, false, false},
		{"unknown flag", []string{"--unknown"}, options{}, true, false},
		{"help long", []string{"--help"}, options{}, true, true},
		{"help short", []string{"-h"}, options{}, true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseArgs(tt.args)
			if tt.isHelp {
				if err != errHelp {
					t.Fatalf("parseArgs(%v) error = %v, want errHelp", tt.args, err)
				}
				return
			}
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseArgs(%v) error = %v, wantErr %v", tt.args, err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if got != tt.want {
				t.Errorf("parseArgs(%v) = %+v, want %+v", tt.args, got, tt.want)
			}
		})
	}
}

func TestPrintUsage(t *testing.T) {
	var buf bytes.Buffer
	printUsage(&buf)

	output := buf.String()
	for _, want := range []string{"--verbose", "--dry-run", "--help", "gh sync"} {
		if !strings.Contains(output, want) {
			t.Errorf("usage output missing %q", want)
		}
	}
}

// ---------------------------------------------------------------------------
// Integration tests
// ---------------------------------------------------------------------------

type testEnv struct {
	base   string
	remote string
	local  string
	t      *testing.T

	// prHeads maps a head branch name to the head commits of its merged PRs.
	prHeads map[string][]string
}

// mergedPR records a merged PR for branch whose head commit is the
// branch's current local tip.
func (e *testEnv) mergedPR(branch string) {
	e.t.Helper()
	if e.prHeads == nil {
		e.prHeads = map[string][]string{}
	}
	sha := strings.TrimSpace(mustExec(e.t, e.local, "git", "rev-parse", branch))
	e.prHeads[branch] = append(e.prHeads[branch], sha)
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	base := t.TempDir()
	remote := filepath.Join(base, "remote.git")
	local := filepath.Join(base, "local")
	setup := filepath.Join(base, "setup")

	mustExec(t, "", "git", "init", "-b", "main", setup)
	mustExec(t, setup, "git", "config", "user.email", "test@test.com")
	mustExec(t, setup, "git", "config", "user.name", "Test")
	writeTestFile(t, filepath.Join(setup, "README.md"), "# test\n")
	mustExec(t, setup, "git", "add", ".")
	mustExec(t, setup, "git", "commit", "-m", "initial")
	mustExec(t, "", "git", "clone", "--bare", setup, remote)
	mustExec(t, "", "git", "clone", remote, local)
	mustExec(t, local, "git", "config", "user.email", "test@test.com")
	mustExec(t, local, "git", "config", "user.name", "Test")

	// Integration tests never call gh. Tests that need a merged PR set prHeads.
	env := &testEnv{base: base, remote: remote, local: local, t: t}
	orig := mergedPRHeads
	mergedPRHeads = func(remoteURL, branch string) []string { return env.prHeads[branch] }
	t.Cleanup(func() { mergedPRHeads = orig })

	return env
}

// chdir changes into the local repo and restores the original dir on cleanup.
func (e *testEnv) chdir() {
	e.t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		e.t.Fatal(err)
	}
	if err := os.Chdir(e.local); err != nil {
		e.t.Fatal(err)
	}
	e.t.Cleanup(func() { os.Chdir(orig) })

	git.Verbose = false
	git.Color = false
	git.Stderr = os.Stderr
}

var tmpCounter int

// addRemoteCommit pushes a new commit to the given branch on the bare remote.
func (e *testEnv) addRemoteCommit(branch, filename, content string) {
	e.t.Helper()
	e.commitOnRemote(branch, map[string]string{filename: content}, "remote update: "+filename)
}

// commitOnRemote pushes one commit with the given files to a branch on the bare remote.
func (e *testEnv) commitOnRemote(branch string, files map[string]string, message string) {
	e.t.Helper()
	tmpCounter++
	tmp := filepath.Join(e.base, fmt.Sprintf("tmp-push-%s-%d", branch, tmpCounter))
	mustExec(e.t, "", "git", "clone", e.remote, tmp)
	mustExec(e.t, tmp, "git", "config", "user.email", "test@test.com")
	mustExec(e.t, tmp, "git", "config", "user.name", "Test")
	mustExec(e.t, tmp, "git", "checkout", branch)
	for name, content := range files {
		writeTestFile(e.t, filepath.Join(tmp, name), content)
	}
	mustExec(e.t, tmp, "git", "add", ".")
	mustExec(e.t, tmp, "git", "commit", "-m", message)
	mustExec(e.t, tmp, "git", "push")
	os.RemoveAll(tmp)
}

// commitLocal adds one local commit to a branch without pushing it,
// then returns to main.
func (e *testEnv) commitLocal(branch, filename, content string) {
	e.t.Helper()
	mustExec(e.t, e.local, "git", "checkout", branch)
	writeTestFile(e.t, filepath.Join(e.local, filename), content)
	mustExec(e.t, e.local, "git", "add", ".")
	mustExec(e.t, e.local, "git", "commit", "-m", "add "+filename)
	mustExec(e.t, e.local, "git", "checkout", "main")
}

// createBranch creates a branch locally with one commit and pushes it with tracking.
func (e *testEnv) createBranch(name, filename, content string) {
	e.t.Helper()
	mustExec(e.t, e.local, "git", "branch", name)
	e.commitLocal(name, filename, content)
	mustExec(e.t, e.local, "git", "push", "-u", "origin", name)
}

// deleteRemoteBranch removes a branch from the bare remote.
func (e *testEnv) deleteRemoteBranch(name string) {
	e.t.Helper()
	mustExec(e.t, e.local, "git", "push", "origin", "--delete", name)
}

// deleteBranchOnHost removes a branch on the bare remote only, the way GitHub
// deletes a head branch after merge. The local remote-tracking ref stays until
// the next fetch --prune, unlike deleteRemoteBranch.
func (e *testEnv) deleteBranchOnHost(name string) {
	e.t.Helper()
	mustExec(e.t, "", "git", "--git-dir", e.remote, "branch", "-D", name)
}

// branchExists reports whether a local branch exists.
func (e *testEnv) branchExists(name string) bool {
	e.t.Helper()
	return strings.TrimSpace(mustExec(e.t, e.local, "git", "branch", "--list", name)) != ""
}

// mergeOnRemote merges a branch into main on the remote (regular merge).
func (e *testEnv) mergeOnRemote(branch string) {
	e.t.Helper()
	tmp := filepath.Join(e.base, "tmp-merge-"+branch)
	mustExec(e.t, "", "git", "clone", e.remote, tmp)
	mustExec(e.t, tmp, "git", "config", "user.email", "test@test.com")
	mustExec(e.t, tmp, "git", "config", "user.name", "Test")
	mustExec(e.t, tmp, "git", "merge", "origin/"+branch)
	mustExec(e.t, tmp, "git", "push")
	os.RemoveAll(tmp)
}

// squashMergeOnRemote squash-merges a branch into main on the remote.
func (e *testEnv) squashMergeOnRemote(branch string) {
	e.t.Helper()
	tmp := filepath.Join(e.base, "tmp-squash-"+branch)
	mustExec(e.t, "", "git", "clone", e.remote, tmp)
	mustExec(e.t, tmp, "git", "config", "user.email", "test@test.com")
	mustExec(e.t, tmp, "git", "config", "user.name", "Test")
	mustExec(e.t, tmp, "git", "merge", "--squash", "origin/"+branch)
	mustExec(e.t, tmp, "git", "commit", "-m", "squash merge "+branch)
	mustExec(e.t, tmp, "git", "push")
	os.RemoveAll(tmp)
}

func runSync(t *testing.T) (stdout, stderr string, err error) {
	t.Helper()
	var outBuf, errBuf bytes.Buffer
	err = sync(&outBuf, &errBuf, false, false)
	return outBuf.String(), errBuf.String(), err
}

// ---------------------------------------------------------------------------

func TestSync_UpToDate(t *testing.T) {
	env := newTestEnv(t)
	env.chdir()

	stdout, stderr, err := runSync(t)
	if err != nil {
		t.Fatalf("sync error: %v\nstderr: %s", err, stderr)
	}
	if stdout != "" {
		t.Errorf("expected no stdout for up-to-date repo, got: %s", stdout)
	}
}

func TestSync_FastForwardCurrentBranch(t *testing.T) {
	env := newTestEnv(t)
	env.addRemoteCommit("main", "new.txt", "new content\n")
	env.chdir()

	stdout, stderr, err := runSync(t)
	if err != nil {
		t.Fatalf("sync error: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(stdout, "Updated branch main") {
		t.Errorf("expected update message for main, got stdout: %s", stdout)
	}
}

func TestSync_FastForwardOtherBranch(t *testing.T) {
	env := newTestEnv(t)
	env.createBranch("feature", "feature.txt", "v1\n")
	env.addRemoteCommit("feature", "feature.txt", "v2\n")
	env.chdir()

	stdout, stderr, err := runSync(t)
	if err != nil {
		t.Fatalf("sync error: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(stdout, "Updated branch feature") {
		t.Errorf("expected update message for feature, got stdout: %s", stdout)
	}
}

func TestSync_DivergedBranch(t *testing.T) {
	env := newTestEnv(t)
	env.createBranch("diverged", "file.txt", "original\n")

	// Add a remote commit on the branch
	env.addRemoteCommit("diverged", "remote.txt", "from remote\n")

	// Add a local commit on the branch
	mustExec(t, env.local, "git", "checkout", "diverged")
	writeTestFile(t, filepath.Join(env.local, "local.txt"), "local change\n")
	mustExec(t, env.local, "git", "add", ".")
	mustExec(t, env.local, "git", "commit", "-m", "local change")
	mustExec(t, env.local, "git", "checkout", "main")

	env.chdir()

	stdout, stderr, err := runSync(t)
	if err != nil {
		t.Fatalf("sync error: %v\nstdout: %s", err, stdout)
	}
	if !strings.Contains(stderr, "unpushed commits") {
		t.Errorf("expected unpushed warning, got stderr: %s", stderr)
	}
}

func TestSync_DeleteMergedBranch(t *testing.T) {
	env := newTestEnv(t)
	env.createBranch("merged-feature", "merged.txt", "content\n")

	env.mergeOnRemote("merged-feature")
	env.deleteRemoteBranch("merged-feature")

	env.chdir()

	stdout, stderr, err := runSync(t)
	if err != nil {
		t.Fatalf("sync error: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(stdout, "Deleted branch merged-feature") {
		t.Errorf("expected delete message, got stdout: %s", stdout)
	}

	if env.branchExists("merged-feature") {
		t.Error("branch merged-feature should have been deleted")
	}
}

func TestSync_DeleteSquashMergedBranch(t *testing.T) {
	env := newTestEnv(t)
	env.createBranch("squash-me", "squash.txt", "squash content\n")

	env.squashMergeOnRemote("squash-me")
	env.deleteBranchOnHost("squash-me")

	env.chdir()

	stdout, stderr, err := runSync(t)
	if err != nil {
		t.Fatalf("sync error: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(stdout, "Deleted branch squash-me") {
		t.Errorf("expected delete message for squash-merged branch, got stdout: %s\nstderr: %s", stdout, stderr)
	}
}

// isolateGitConfig hides the global and system git config from the local repo,
// so a test controls the identity and signing settings the sync sees.
func (e *testEnv) isolateGitConfig() {
	e.t.Helper()
	e.t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(e.base, "empty-gitconfig"))
	e.t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
}

func TestSync_DeleteSquashMergedBranchWithoutIdentity(t *testing.T) {
	env := newTestEnv(t)
	env.createBranch("squash-me", "squash.txt", "squash content\n")

	env.squashMergeOnRemote("squash-me")
	env.deleteBranchOnHost("squash-me")

	// No user.name or user.email, and no guessing one from the hostname
	env.isolateGitConfig()
	mustExec(t, env.local, "git", "config", "--unset", "user.name")
	mustExec(t, env.local, "git", "config", "--unset", "user.email")
	mustExec(t, env.local, "git", "config", "user.useConfigOnly", "true")

	env.chdir()

	stdout, stderr, err := runSync(t)
	if err != nil {
		t.Fatalf("sync error: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(stdout, "Deleted branch squash-me") {
		t.Errorf("expected delete message for squash-merged branch, got stdout: %s\nstderr: %s", stdout, stderr)
	}
}

func TestSync_KeepSquashMatchedBranchWithUnpushedCommits(t *testing.T) {
	env := newTestEnv(t)
	env.createBranch("feature", "a.txt", "a\n")
	env.mergedPR("feature")

	// A local commit that was never pushed
	env.commitLocal("feature", "b.txt", "b\n")

	// main receives the same overall change, so the squash check matches,
	// and the remote branch is deleted
	env.commitOnRemote("main", map[string]string{"a.txt": "a\n", "b.txt": "b\n"}, "same change")
	env.deleteBranchOnHost("feature")

	env.chdir()

	_, stderr, err := runSync(t)
	if err != nil {
		t.Fatalf("sync error: %v\nstderr: %s", err, stderr)
	}
	if !env.branchExists("feature") {
		t.Fatal("branch with unpushed commits should not be deleted")
	}
	if !strings.Contains(stderr, "never pushed") {
		t.Errorf("expected never-pushed warning, got stderr: %s", stderr)
	}
}

func TestSync_KeepSquashMergedBranchWhenUpstreamAlreadyPruned(t *testing.T) {
	env := newTestEnv(t)
	env.createBranch("squash-me", "squash.txt", "squash content\n")

	env.squashMergeOnRemote("squash-me")
	// Deleting from the local clone also removes the remote-tracking ref,
	// so gh sync can't confirm what was last pushed.
	env.deleteRemoteBranch("squash-me")

	env.chdir()

	_, stderr, err := runSync(t)
	if err != nil {
		t.Fatalf("sync error: %v\nstderr: %s", err, stderr)
	}
	if !env.branchExists("squash-me") {
		t.Fatal("squash-matched branch should be kept when its pushed state is unknown")
	}
	if !strings.Contains(stderr, "never pushed") {
		t.Errorf("expected never-pushed warning, got stderr: %s", stderr)
	}
}

func TestSync_DeleteSquashMergedBranchConfirmedByMergedPR(t *testing.T) {
	env := newTestEnv(t)
	env.createBranch("squash-me", "squash.txt", "squash content\n")
	env.mergedPR("squash-me")

	env.squashMergeOnRemote("squash-me")
	// Pruned before gh sync runs, like a git pull with fetch.prune=true
	env.deleteRemoteBranch("squash-me")

	env.chdir()

	stdout, stderr, err := runSync(t)
	if err != nil {
		t.Fatalf("sync error: %v\nstderr: %s", err, stderr)
	}
	if env.branchExists("squash-me") {
		t.Errorf("branch confirmed by a merged PR should be deleted\nstdout: %s\nstderr: %s", stdout, stderr)
	}
}

func TestSync_DryRunThenSyncDeletesSquashMergedBranch(t *testing.T) {
	env := newTestEnv(t)
	env.createBranch("squash-me", "squash.txt", "squash content\n")
	env.mergedPR("squash-me")
	env.squashMergeOnRemote("squash-me")
	env.deleteBranchOnHost("squash-me")
	env.chdir()

	var stdout, stderr bytes.Buffer
	if err := sync(&stdout, &stderr, false, true); err != nil {
		t.Fatalf("dry run error: %v\nstderr: %s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Would delete branch squash-me") {
		t.Fatalf("expected would-delete message, got stdout: %s\nstderr: %s", stdout.String(), stderr.String())
	}

	// The dry run's fetch pruned the tracking ref; the real run must still agree.
	out, errOut, err := runSync(t)
	if err != nil {
		t.Fatalf("sync error: %v\nstderr: %s", err, errOut)
	}
	if !strings.Contains(out, "Deleted branch squash-me") {
		t.Errorf("expected delete after dry run, got stdout: %s\nstderr: %s", out, errOut)
	}
}

// addWorktree checks out branch in a new linked worktree outside the local repo.
func (e *testEnv) addWorktree(branch string) string {
	e.t.Helper()
	path := filepath.Join(e.base, "wt-"+strings.ReplaceAll(branch, "/", "-"))
	mustExec(e.t, e.local, "git", "worktree", "add", "-q", path, branch)
	return path
}

func TestSync_SkipDeletingBranchCheckedOutInWorktree(t *testing.T) {
	env := newTestEnv(t)
	env.createBranch("in-worktree", "a.txt", "a\n")
	env.createBranch("plain", "b.txt", "b\n")
	env.mergeOnRemote("in-worktree")
	env.mergeOnRemote("plain")
	env.deleteBranchOnHost("in-worktree")
	env.deleteBranchOnHost("plain")
	env.addWorktree("in-worktree")
	env.chdir()

	stdout, stderr, err := runSync(t)
	if err != nil {
		t.Fatalf("sync error: %v\nstderr: %s", err, stderr)
	}
	if !env.branchExists("in-worktree") {
		t.Error("branch checked out in a worktree should be kept")
	}
	if !strings.Contains(stderr, "'in-worktree' is checked out in") {
		t.Errorf("expected worktree warning, got stderr: %s", stderr)
	}
	// The sync keeps going after the skipped branch.
	if env.branchExists("plain") || !strings.Contains(stdout, "Deleted branch plain") {
		t.Errorf("expected plain to be deleted, got stdout: %s\nstderr: %s", stdout, stderr)
	}
}

func TestSync_SkipFastForwardingBranchCheckedOutInWorktree(t *testing.T) {
	env := newTestEnv(t)
	env.createBranch("in-worktree", "a.txt", "v1\n")
	env.addRemoteCommit("in-worktree", "a.txt", "v2\n")
	wt := env.addWorktree("in-worktree")
	before := strings.TrimSpace(mustExec(t, env.local, "git", "rev-parse", "in-worktree"))
	env.chdir()

	stdout, stderr, err := runSync(t)
	if err != nil {
		t.Fatalf("sync error: %v\nstderr: %s", err, stderr)
	}
	if after := strings.TrimSpace(mustExec(t, env.local, "git", "rev-parse", "in-worktree")); after != before {
		t.Errorf("branch checked out in a worktree moved from %s to %s", before, after)
	}
	if strings.Contains(stdout, "Updated branch in-worktree") {
		t.Errorf("did not expect an update, got stdout: %s", stdout)
	}
	if !strings.Contains(stderr, "'in-worktree' is checked out in") {
		t.Errorf("expected worktree warning, got stderr: %s", stderr)
	}
	// The worktree's files must still match its HEAD.
	if status := strings.TrimSpace(mustExec(t, wt, "git", "status", "--porcelain")); status != "" {
		t.Errorf("worktree has unexpected changes:\n%s", status)
	}
}

func TestSync_DryRunReportsWorktreeBranches(t *testing.T) {
	env := newTestEnv(t)
	env.createBranch("in-worktree", "a.txt", "a\n")
	env.mergeOnRemote("in-worktree")
	env.deleteBranchOnHost("in-worktree")
	env.addWorktree("in-worktree")
	env.chdir()

	var stdout, stderr bytes.Buffer
	if err := sync(&stdout, &stderr, false, true); err != nil {
		t.Fatalf("sync error: %v\nstderr: %s", err, stderr.String())
	}
	if strings.Contains(stdout.String(), "Would delete branch in-worktree") {
		t.Errorf("dry run should not promise to delete a worktree branch, got: %s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "'in-worktree' is checked out in") {
		t.Errorf("expected worktree warning, got stderr: %s", stderr.String())
	}
}

func TestSync_DryRun(t *testing.T) {
	env := newTestEnv(t)
	env.createBranch("merged-feature", "merged.txt", "content\n")
	env.mergeOnRemote("merged-feature")
	env.deleteBranchOnHost("merged-feature")
	env.addRemoteCommit("main", "new.txt", "new\n")
	env.chdir()

	before := strings.TrimSpace(mustExec(t, env.local, "git", "rev-parse", "main"))

	var stdout, stderr bytes.Buffer
	if err := sync(&stdout, &stderr, false, true); err != nil {
		t.Fatalf("sync error: %v\nstderr: %s", err, stderr.String())
	}

	if !strings.Contains(stdout.String(), "Would update branch main") {
		t.Errorf("expected would-update message, got stdout: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Would delete branch merged-feature") {
		t.Errorf("expected would-delete message, got stdout: %s", stdout.String())
	}
	if after := strings.TrimSpace(mustExec(t, env.local, "git", "rev-parse", "main")); after != before {
		t.Errorf("dry run moved main from %s to %s", before, after)
	}
	if !env.branchExists("merged-feature") {
		t.Error("dry run deleted merged-feature")
	}
}

func TestSync_NotMergedBranch(t *testing.T) {
	env := newTestEnv(t)
	env.createBranch("not-merged", "unique.txt", "unique\n")

	// Delete from remote without merging
	env.deleteRemoteBranch("not-merged")

	env.chdir()

	stdout, stderr, err := runSync(t)
	if err != nil {
		t.Fatalf("sync error: %v\nstdout: %s", err, stdout)
	}
	if !strings.Contains(stderr, "not merged") {
		t.Errorf("expected not-merged warning, got stderr: %s", stderr)
	}
}

func TestSync_DeleteCurrentBranch(t *testing.T) {
	env := newTestEnv(t)
	env.createBranch("current-gone", "gone.txt", "gone\n")

	env.mergeOnRemote("current-gone")
	env.deleteRemoteBranch("current-gone")

	// Switch to the branch that will be deleted
	mustExec(t, env.local, "git", "checkout", "current-gone")

	env.chdir()

	stdout, stderr, err := runSync(t)
	if err != nil {
		t.Fatalf("sync error: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(stdout, "Deleted branch current-gone") {
		t.Errorf("expected delete message, got stdout: %s", stdout)
	}

	// Verify we switched to the default branch
	branch, err := git.CurrentBranch()
	if err != nil {
		t.Fatalf("CurrentBranch() error: %v", err)
	}
	if branch != "main" {
		t.Errorf("expected to be on main after deletion, got %s", branch)
	}
}

func TestSync_ImplicitUpstream(t *testing.T) {
	env := newTestEnv(t)

	// Create a branch on the remote only
	env.addRemoteCommit("main", "setup.txt", "setup\n")

	// Create a local branch with no tracking config
	mustExec(t, env.local, "git", "checkout", "-b", "implicit")
	writeTestFile(t, filepath.Join(env.local, "implicit.txt"), "local\n")
	mustExec(t, env.local, "git", "add", ".")
	mustExec(t, env.local, "git", "commit", "-m", "local implicit")
	mustExec(t, env.local, "git", "push", "origin", "implicit") // push without -u (no tracking)
	mustExec(t, env.local, "git", "checkout", "main")

	// Advance the branch on the remote
	env.addRemoteCommit("implicit", "implicit.txt", "updated\n")

	env.chdir()

	stdout, _, err := runSync(t)
	if err != nil {
		t.Fatalf("sync error: %v", err)
	}
	if !strings.Contains(stdout, "Updated branch implicit") {
		t.Errorf("expected update message for implicit tracking, got stdout: %s", stdout)
	}
}

func TestSync_ColorOutput(t *testing.T) {
	env := newTestEnv(t)
	env.addRemoteCommit("main", "new.txt", "content\n")
	env.chdir()

	var stdout, stderr bytes.Buffer
	err := sync(&stdout, &stderr, true, false)
	if err != nil {
		t.Fatalf("sync error: %v", err)
	}

	if !strings.Contains(stdout.String(), "\033[") {
		t.Errorf("expected ANSI color codes in output, got: %q", stdout.String())
	}
}

func TestSync_VerboseOutput(t *testing.T) {
	env := newTestEnv(t)
	env.chdir()

	var stdout, stderr bytes.Buffer
	git.Verbose = true
	git.Color = false
	git.Stderr = &stderr

	err := sync(&stdout, &stderr, false, false)
	if err != nil {
		t.Fatalf("sync error: %v", err)
	}

	if !strings.Contains(stderr.String(), "$ git") {
		t.Errorf("expected verbose git commands on stderr, got: %q", stderr.String())
	}
}

func TestSync_NoRemotes(t *testing.T) {
	dir := t.TempDir()
	mustExec(t, "", "git", "init", "-b", "main", dir)

	orig, _ := os.Getwd()
	os.Chdir(dir)
	t.Cleanup(func() { os.Chdir(orig) })

	git.Verbose = false
	git.Color = false

	var stdout, stderr bytes.Buffer
	err := sync(&stdout, &stderr, false, false)
	if err == nil {
		t.Fatal("expected error when no remotes exist")
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func mustExec(t *testing.T, dir, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v (in %s) failed: %v\n%s", name, args, dir, err, out)
	}
	return string(out)
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
