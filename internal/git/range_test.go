package git

import (
	"path/filepath"
	"testing"
)

func TestRange_IsIdentical(t *testing.T) {
	r := &Range{A: "abc123def456", B: "abc123def456"}
	if !r.IsIdentical() {
		t.Error("expected identical range")
	}

	r2 := &Range{A: "abc123", B: "def456"}
	if r2.IsIdentical() {
		t.Error("expected non-identical range")
	}
}

func TestNewRange(t *testing.T) {
	dir := initTestRepo(t)
	chdir(t, dir)

	r, err := NewRange("refs/heads/main", "refs/remotes/origin/main")
	if err != nil {
		t.Fatalf("NewRange() error: %v", err)
	}
	if r.A != r.B {
		t.Errorf("expected identical SHAs after fresh clone, got A=%s B=%s", r.A, r.B)
	}
}

func TestRange_IsAncestor(t *testing.T) {
	dir := initTestRepo(t)
	chdir(t, dir)

	// Create a second commit on main
	writeFile(t, filepath.Join(dir, "second.txt"), "second\n")
	mustGit(t, dir, "add", ".")
	mustGit(t, dir, "commit", "-m", "second")

	// Now local main is ahead of origin/main
	r, err := NewRange("refs/remotes/origin/main", "refs/heads/main")
	if err != nil {
		t.Fatalf("NewRange() error: %v", err)
	}

	if r.IsIdentical() {
		t.Error("should not be identical after new commit")
	}
	if !r.IsAncestor() {
		t.Error("origin/main should be ancestor of local main")
	}

	// Check the reverse
	r2, err := NewRange("refs/heads/main", "refs/remotes/origin/main")
	if err != nil {
		t.Fatalf("NewRange() error: %v", err)
	}
	if r2.IsAncestor() {
		t.Error("local main should NOT be ancestor of origin/main")
	}
}
