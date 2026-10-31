package git

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

// Verbose controls whether git commands are logged to stderr.
var Verbose bool

// Color controls whether verbose output uses ANSI colors.
var Color bool

// Stderr is the writer for verbose output and fetch progress. Defaults to os.Stderr.
var Stderr io.Writer = os.Stderr

// exec runs a git command and returns trimmed stdout. Stderr is suppressed.
func execGit(args ...string) (string, error) {
	logCmd(args)
	cmd := exec.Command("git", args...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(out), "\n"), nil
}

// Spawn runs a git command with full I/O passthrough to the terminal.
func Spawn(args ...string) error {
	logCmd(args)
	cmd := exec.Command("git", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = Stderr
	return cmd.Run()
}

// Run runs a git command silently and returns whether it succeeded.
func Run(args ...string) bool {
	logCmd(args)
	return exec.Command("git", args...).Run() == nil
}

func logCmd(args []string) {
	if !Verbose {
		return
	}
	if Color {
		fmt.Fprintf(Stderr, "\033[35m$ git %s\033[0m\n", strings.Join(args, " "))
	} else {
		fmt.Fprintf(Stderr, "$ git %s\n", strings.Join(args, " "))
	}
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// MainRemote returns the primary remote, preferring upstream > github > origin.
func MainRemote() (string, error) {
	out, err := execGit("remote")
	if err != nil {
		return "", fmt.Errorf("failed to list remotes: %w", err)
	}

	remotes := splitLines(out)
	if len(remotes) == 0 {
		return "", fmt.Errorf("no git remotes found")
	}

	known := map[string]bool{}
	for _, name := range remotes {
		known[name] = true
	}

	for _, candidate := range []string{"upstream", "github", "origin"} {
		if known[candidate] {
			return candidate, nil
		}
	}

	return remotes[0], nil
}

// DefaultBranch resolves the default branch name for a remote.
// Checks symbolic-ref first, then probes for main and master on the remote.
func DefaultBranch(remote string) string {
	headRef := fmt.Sprintf("refs/remotes/%s/HEAD", remote)
	if out, err := execGit("symbolic-ref", "--quiet", headRef); err == nil {
		prefix := fmt.Sprintf("refs/remotes/%s/", remote)
		return strings.TrimPrefix(out, prefix)
	}

	if HasRef(fmt.Sprintf("refs/remotes/%s/main", remote)) {
		return "main"
	}
	if HasRef(fmt.Sprintf("refs/remotes/%s/master", remote)) {
		return "master"
	}

	return "main"
}

// CurrentBranch returns the name of the checked-out branch.
func CurrentBranch() (string, error) {
	out, err := execGit("symbolic-ref", "--short", "HEAD")
	if err != nil {
		return "", fmt.Errorf("not on any branch")
	}
	return out, nil
}

// LocalBranches lists all local branch names.
func LocalBranches() ([]string, error) {
	out, err := execGit("branch", "--format=%(refname:short)")
	if err != nil {
		return nil, fmt.Errorf("failed to list branches: %w", err)
	}
	return splitLines(out), nil
}

// Fetch fetches from a remote with pruning and progress output.
func Fetch(remote string) error {
	return Spawn("fetch", "--prune", "--quiet", "--progress", remote)
}

// BranchRemotes returns a mapping of local branch name to its configured
// remote, parsed from branch.*.remote git config entries.
func BranchRemotes() map[string]string {
	out, err := execGit("config", "--get-regexp", `^branch\..*\.remote$`)
	if err != nil {
		return nil
	}

	re := regexp.MustCompile(`^branch\.(.+)\.remote\s+(.+)$`)
	result := make(map[string]string)
	for _, line := range splitLines(out) {
		if m := re.FindStringSubmatch(line); m != nil {
			result[m[1]] = m[2]
		}
	}
	return result
}

// Upstreams maps each local branch with an upstream to its remote-tracking
// ref, such as refs/remotes/origin/feature. Git resolves it with the remote's
// real fetch refspec, and still reports it after the ref has been pruned.
func Upstreams() map[string]string {
	out, err := execGit("for-each-ref", "--format=%(refname:short) %(upstream)", "refs/heads/")
	if err != nil {
		return nil
	}
	result := make(map[string]string)
	for _, line := range splitLines(out) {
		if branch, upstream, ok := strings.Cut(line, " "); ok && upstream != "" {
			result[branch] = upstream
		}
	}
	return result
}

// RefSHAs maps each ref under prefix (such as refs/remotes/origin/) to the
// commit it points at.
func RefSHAs(prefix string) map[string]string {
	out, err := execGit("for-each-ref", "--format=%(refname) %(objectname)", prefix)
	if err != nil {
		return nil
	}
	result := make(map[string]string)
	for _, line := range splitLines(out) {
		if ref, sha, ok := strings.Cut(line, " "); ok {
			result[ref] = sha
		}
	}
	return result
}

// WorktreeBranches maps each branch checked out in a worktree of this
// repository (including the current one) to that worktree's path.
func WorktreeBranches() map[string]string {
	out, err := execGit("worktree", "list", "--porcelain")
	if err != nil {
		return nil
	}
	result := make(map[string]string)
	var path string
	for _, line := range splitLines(out) {
		if p, ok := strings.CutPrefix(line, "worktree "); ok {
			path = p
		} else if ref, ok := strings.CutPrefix(line, "branch refs/heads/"); ok {
			result[ref] = path
		}
	}
	return result
}

// RemoteURL returns the fetch URL of a remote.
func RemoteURL(remote string) (string, error) {
	return execGit("remote", "get-url", remote)
}

// IsAncestor reports whether commit a is an ancestor of (or equal to) commit b.
func IsAncestor(a, b string) bool {
	return Run("merge-base", "--is-ancestor", a, b)
}

// UpstreamRef resolves the full upstream tracking ref for a local branch.
func UpstreamRef(branch string) (string, error) {
	return execGit("rev-parse", "--symbolic-full-name", branch+"@{upstream}")
}

// HasRef checks whether a fully-qualified ref exists.
func HasRef(ref string) bool {
	return Run("show-ref", "--verify", "--quiet", ref)
}

// RevParse resolves refs to their SHA hashes.
func RevParse(refs ...string) ([]string, error) {
	args := make([]string, 0, 2+len(refs))
	args = append(args, "rev-parse", "--quiet")
	args = append(args, refs...)
	out, err := execGit(args...)
	if err != nil {
		return nil, err
	}
	return splitLines(out), nil
}

// MergeFFOnly fast-forwards the current branch to the given ref.
func MergeFFOnly(ref string) error {
	_, err := execGit("merge", "--ff-only", "--quiet", ref)
	return err
}

// UpdateRef points a ref at the commit identified by target.
func UpdateRef(ref, target string) error {
	_, err := execGit("update-ref", ref, target)
	return err
}

// DeleteBranch force-deletes a local branch.
func DeleteBranch(name string) error {
	_, err := execGit("branch", "-D", name)
	return err
}

// Switch switches to the named branch quietly. Unlike checkout, switch never
// treats its argument as a file path. The "--" keeps a branch name that comes
// from the remote, such as "-Cmain", from being parsed as an option.
func Switch(branch string) error {
	_, err := execGit("switch", "--quiet", "--", branch)
	return err
}

// MergeBase returns the best common ancestor commit of two refs.
func MergeBase(a, b string) (string, error) {
	return execGit("merge-base", a, b)
}

// TreeHash returns the tree object SHA for a commit ref.
func TreeHash(ref string) (string, error) {
	return execGit("rev-parse", ref+"^{tree}")
}

// CommitTree creates a commit object from a tree, parent, and message.
// The commit is only used for comparison and is never referenced, so it gets a
// fixed identity that works even when user.name and user.email aren't configured.
func CommitTree(tree, parent, message string) (string, error) {
	return execGit("-c", "user.name=gh-sync", "-c", "user.email=gh-sync@localhost",
		"commit-tree", tree, "-p", parent, "-m", message)
}

// Cherry checks whether a commit's patch exists in an upstream branch.
// The output line starts with "-" if already applied, "+" if not.
func Cherry(upstream, head string) (string, error) {
	return execGit("cherry", upstream, head)
}
