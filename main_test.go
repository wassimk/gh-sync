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
		verbose bool
		wantErr bool
		isHelp  bool
	}{
		{"no flags", nil, false, false, false},
		{"verbose long", []string{"--verbose"}, true, false, false},
		{"verbose short", []string{"-v"}, true, false, false},
		{"unknown flag", []string{"--unknown"}, false, true, false},
		{"help long", []string{"--help"}, false, true, true},
		{"help short", []string{"-h"}, false, true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			verbose, err := parseArgs(tt.args)
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
			if verbose != tt.verbose {
				t.Errorf("verbose = %v, want %v", verbose, tt.verbose)
			}
		})
	}
}

func TestPrintUsage(t *testing.T) {
	var buf bytes.Buffer
	printUsage(&buf)

	output := buf.String()
	for _, want := range []string{"--verbose", "--help", "gh sync"} {
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

	return &testEnv{base: base, remote: remote, local: local, t: t}
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
	tmpCounter++
	tmp := filepath.Join(e.base, fmt.Sprintf("tmp-push-%s-%d", branch, tmpCounter))
	mustExec(e.t, "", "git", "clone", e.remote, tmp)
	mustExec(e.t, tmp, "git", "config", "user.email", "test@test.com")
	mustExec(e.t, tmp, "git", "config", "user.name", "Test")
	mustExec(e.t, tmp, "git", "checkout", branch)
	writeTestFile(e.t, filepath.Join(tmp, filename), content)
	mustExec(e.t, tmp, "git", "add", ".")
	mustExec(e.t, tmp, "git", "commit", "-m", "remote update: "+filename)
	mustExec(e.t, tmp, "git", "push")
	os.RemoveAll(tmp)
}

// createBranch creates a branch locally with one commit and pushes it with tracking.
func (e *testEnv) createBranch(name, filename, content string) {
	e.t.Helper()
	mustExec(e.t, e.local, "git", "checkout", "-b", name)
	writeTestFile(e.t, filepath.Join(e.local, filename), content)
	mustExec(e.t, e.local, "git", "add", ".")
	mustExec(e.t, e.local, "git", "commit", "-m", "add "+filename)
	mustExec(e.t, e.local, "git", "push", "-u", "origin", name)
	mustExec(e.t, e.local, "git", "checkout", "main")
}

// deleteRemoteBranch removes a branch from the bare remote.
func (e *testEnv) deleteRemoteBranch(name string) {
	e.t.Helper()
	mustExec(e.t, e.local, "git", "push", "origin", "--delete", name)
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
	err = sync(&outBuf, &errBuf, false)
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

	// Verify the branch is actually gone
	out := mustExec(t, env.local, "git", "branch", "--list", "merged-feature")
	if strings.TrimSpace(out) != "" {
		t.Error("branch merged-feature should have been deleted")
	}
}

func TestSync_DeleteSquashMergedBranch(t *testing.T) {
	env := newTestEnv(t)
	env.createBranch("squash-me", "squash.txt", "squash content\n")

	env.squashMergeOnRemote("squash-me")
	env.deleteRemoteBranch("squash-me")

	env.chdir()

	stdout, stderr, err := runSync(t)
	if err != nil {
		t.Fatalf("sync error: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(stdout, "Deleted branch squash-me") {
		t.Errorf("expected delete message for squash-merged branch, got stdout: %s\nstderr: %s", stdout, stderr)
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
	err := sync(&stdout, &stderr, true)
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

	err := sync(&stdout, &stderr, false)
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
	err := sync(&stdout, &stderr, false)
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
