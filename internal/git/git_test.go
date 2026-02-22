package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestSplitLines(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"", 0},
		{"one", 1},
		{"one\ntwo", 2},
		{"one\ntwo\nthree", 3},
	}

	for _, tt := range tests {
		got := splitLines(tt.input)
		if len(got) != tt.want {
			t.Errorf("splitLines(%q) returned %d lines, want %d", tt.input, len(got), tt.want)
		}
	}
}

func TestMainRemote(t *testing.T) {
	dir := initTestRepo(t)
	chdir(t, dir)

	remote, err := MainRemote()
	if err != nil {
		t.Fatalf("MainRemote() error: %v", err)
	}
	if remote != "origin" {
		t.Errorf("MainRemote() = %q, want %q", remote, "origin")
	}
}

func TestMainRemote_PrefersUpstream(t *testing.T) {
	dir := initTestRepo(t)
	chdir(t, dir)

	mustGit(t, dir, "remote", "add", "upstream", "https://example.com/upstream.git")

	remote, err := MainRemote()
	if err != nil {
		t.Fatalf("MainRemote() error: %v", err)
	}
	if remote != "upstream" {
		t.Errorf("MainRemote() = %q, want %q", remote, "upstream")
	}
}

func TestMainRemote_NoRemotes(t *testing.T) {
	dir := t.TempDir()
	mustGit(t, dir, "init", "-b", "main")
	chdir(t, dir)

	_, err := MainRemote()
	if err == nil {
		t.Fatal("expected error when no remotes exist")
	}
}

func TestDefaultBranch(t *testing.T) {
	dir := initTestRepo(t)
	chdir(t, dir)

	branch := DefaultBranch("origin")
	if branch != "main" {
		t.Errorf("DefaultBranch() = %q, want %q", branch, "main")
	}
}

func TestCurrentBranch(t *testing.T) {
	dir := initTestRepo(t)
	chdir(t, dir)

	branch, err := CurrentBranch()
	if err != nil {
		t.Fatalf("CurrentBranch() error: %v", err)
	}
	if branch != "main" {
		t.Errorf("CurrentBranch() = %q, want %q", branch, "main")
	}
}

func TestLocalBranches(t *testing.T) {
	dir := initTestRepo(t)
	chdir(t, dir)

	mustGit(t, dir, "checkout", "-b", "feature-a")
	mustGit(t, dir, "checkout", "main")
	mustGit(t, dir, "checkout", "-b", "feature-b")

	branches, err := LocalBranches()
	if err != nil {
		t.Fatalf("LocalBranches() error: %v", err)
	}

	want := map[string]bool{"main": true, "feature-a": true, "feature-b": true}
	got := map[string]bool{}
	for _, b := range branches {
		got[b] = true
	}

	for name := range want {
		if !got[name] {
			t.Errorf("LocalBranches() missing %q", name)
		}
	}
}

func TestBranchRemotes(t *testing.T) {
	dir := initTestRepo(t)
	chdir(t, dir)

	result := BranchRemotes()
	if result == nil {
		t.Fatal("BranchRemotes() returned nil")
	}
	if result["main"] != "origin" {
		t.Errorf("BranchRemotes()[main] = %q, want %q", result["main"], "origin")
	}
}

func TestHasRef(t *testing.T) {
	dir := initTestRepo(t)
	chdir(t, dir)

	if !HasRef("refs/heads/main") {
		t.Error("HasRef(refs/heads/main) = false, want true")
	}
	if HasRef("refs/heads/nonexistent") {
		t.Error("HasRef(refs/heads/nonexistent) = true, want false")
	}
}

func TestRevParse(t *testing.T) {
	dir := initTestRepo(t)
	chdir(t, dir)

	shas, err := RevParse("refs/heads/main")
	if err != nil {
		t.Fatalf("RevParse() error: %v", err)
	}
	if len(shas) != 1 {
		t.Fatalf("RevParse() returned %d values, want 1", len(shas))
	}
	if len(shas[0]) != 40 {
		t.Errorf("RevParse() returned %q, expected 40-char SHA", shas[0])
	}
}

// initTestRepo creates a temporary git repo with a remote and an initial commit.
func initTestRepo(t *testing.T) string {
	t.Helper()
	base := t.TempDir()

	remote := filepath.Join(base, "remote.git")
	local := filepath.Join(base, "local")
	setup := filepath.Join(base, "setup")

	// Build a working repo, then bare-clone it as the remote
	mustGit(t, "", "init", "-b", "main", setup)
	mustGit(t, setup, "config", "user.email", "test@test.com")
	mustGit(t, setup, "config", "user.name", "Test")
	writeFile(t, filepath.Join(setup, "README.md"), "# test\n")
	mustGit(t, setup, "add", ".")
	mustGit(t, setup, "commit", "-m", "initial")
	mustGit(t, "", "clone", "--bare", setup, remote)

	// Clone for local work
	mustGit(t, "", "clone", remote, local)
	mustGit(t, local, "config", "user.email", "test@test.com")
	mustGit(t, local, "config", "user.name", "Test")

	return local
}

func chdir(t *testing.T, dir string) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(orig) })

	Verbose = false
	Color = false
	Stderr = os.Stderr
}

func mustGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v (in %s) failed: %v\n%s", args, dir, err, out)
	}
	return string(out)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
