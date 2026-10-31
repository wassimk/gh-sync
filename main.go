package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/wassimk/gh-sync/internal/gh"
	"github.com/wassimk/gh-sync/internal/git"
)

var errHelp = errors.New("help requested")

// mergedPRHeads looks up the head commits of merged pull requests. Tests
// replace it so they never call gh.
var mergedPRHeads = gh.MergedPRHeads

// options holds the parsed command-line flags.
type options struct {
	verbose bool
	dryRun  bool
}

func main() {
	opts, err := parseArgs(os.Args[1:])
	if err != nil {
		if errors.Is(err, errHelp) {
			printUsage(os.Stdout)
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}

	useColor := isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())
	git.Verbose = opts.verbose
	git.Color = useColor

	if err := syncWith(os.Stdout, os.Stderr, useColor, opts); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}
}

func parseArgs(args []string) (options, error) {
	var opts options
	for _, arg := range args {
		switch arg {
		case "--verbose", "-v":
			opts.verbose = true
		case "--dry-run", "-n":
			opts.dryRun = true
		case "--help", "-h":
			return options{}, errHelp
		default:
			return options{}, fmt.Errorf("unknown argument: %s", arg)
		}
	}

	return opts, nil
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, `Usage: gh sync [flags]

Fetch from the primary remote and update local branches.

If a local branch is outdated, fast-forward it.
If a local branch contains unpushed work, warn about it.
If a branch seems merged and its upstream was deleted, delete it.

Flags:
  --dry-run, -n       Show what would change without changing any branches
  --verbose, -v       Log each git command to stderr
  -h, --help          Show this help`)
}

func sync(stdout, stderr io.Writer, useColor, dryRun bool) error {
	return syncWith(stdout, stderr, useColor, options{dryRun: dryRun})
}

func syncWith(stdout, stderr io.Writer, useColor bool, opts options) error {
	dryRun := opts.dryRun
	var green, brightGreen, red, brightRed, reset string
	if useColor {
		green = "\033[32m"
		brightGreen = "\033[1;32m"
		red = "\033[31m"
		brightRed = "\033[1;31m"
		reset = "\033[0m"
	}

	// Find the main remote (upstream > github > origin)
	remote, err := git.MainRemote()
	if err != nil {
		return err
	}

	// Determine the default branch on that remote
	defaultBranch := git.DefaultBranch(remote)
	defaultRef := fmt.Sprintf("refs/remotes/%s/%s", remote, defaultBranch)

	// Note which branch we're on (empty string if detached HEAD)
	currentBranch, _ := git.CurrentBranch()

	// Remember each branch's upstream and where the remote's branches pointed
	// before fetch --prune removes the deleted ones.
	upstreams := git.Upstreams()
	remoteSHAs := git.RefSHAs("refs/remotes/" + remote + "/")

	// Fetch with pruning so deleted remote branches are cleaned up
	if err := git.Fetch(remote); err != nil {
		return fmt.Errorf("fetch failed: %w", err)
	}

	// Read branch.*.remote config to know which branches explicitly track the remote
	branchRemotes := git.BranchRemotes()

	// Enumerate local branches
	branches, err := git.LocalBranches()
	if err != nil {
		return err
	}

	// A branch checked out in another worktree is fast-forwarded inside that
	// worktree, so its files move with it. git can't delete it, so it's kept.
	worktrees := git.WorktreeBranches()
	otherWorktree := func(branch string) (string, bool) {
		path, ok := worktrees[branch]
		return path, ok && branch != currentBranch
	}

	for _, branch := range branches {
		localRef := fmt.Sprintf("refs/heads/%s", branch)
		remoteRef := fmt.Sprintf("refs/remotes/%s/%s", remote, branch)
		gone := false

		if branchRemotes[branch] == remote {
			// Branch is configured to track this remote.
			// Try to resolve its upstream; if that fails the upstream was deleted.
			if upstream, err := git.UpstreamRef(branch); err == nil {
				remoteRef = upstream
			} else {
				remoteRef = ""
				gone = true
			}
		} else if !git.HasRef(remoteRef) {
			// No tracking config and no matching branch on the remote — skip it.
			remoteRef = ""
		}

		if remoteRef != "" {
			// The branch has a remote counterpart — compare them.
			r, err := git.NewRange(localRef, remoteRef)
			if err != nil {
				return err
			}

			if r.IsIdentical() {
				continue
			}

			if r.IsAncestor() {
				// Local is behind — fast-forward.
				if path, ok := otherWorktree(branch); ok {
					if dryRun {
						fmt.Fprintf(stdout, "%sWould update branch %s%s%s in %s (at %s).\n",
							green, brightGreen, branch, reset, path, r.A[:7])
						continue
					}
					if err := git.MergeFFOnlyIn(path, remoteRef); err != nil {
						fmt.Fprintf(stderr, "warning: couldn't fast-forward '%s' in %s: %s\n", branch, path, err)
						continue
					}
					fmt.Fprintf(stdout, "%sUpdated branch %s%s%s in %s (was %s).\n",
						green, brightGreen, branch, reset, path, r.A[:7])
					continue
				}
				if dryRun {
					fmt.Fprintf(stdout, "%sWould update branch %s%s%s (at %s).\n",
						green, brightGreen, branch, reset, r.A[:7])
					continue
				}
				if branch == currentBranch {
					if err := git.MergeFFOnly(remoteRef); err != nil {
						return fmt.Errorf("failed to fast-forward %s: %w", branch, err)
					}
				} else {
					if err := git.UpdateRef(localRef, remoteRef); err != nil {
						return fmt.Errorf("failed to update %s: %w", branch, err)
					}
				}
				fmt.Fprintf(stdout, "%sUpdated branch %s%s%s (was %s).\n",
					green, brightGreen, branch, reset, r.A[:7])
			} else {
				fmt.Fprintf(stderr, "warning: '%s' seems to contain unpushed commits\n", branch)
			}
		} else if gone {
			// The upstream branch was deleted from the remote.
			r, err := git.NewRange(localRef, defaultRef)
			if err != nil {
				return err
			}

			// A regular merge means every local commit is already in the
			// default branch, so deleting loses nothing.
			merged := r.IsAncestor()

			// A squash-merge only shows that the default branch has the same
			// overall change, so also require that nothing was left unpushed.
			if !merged && isSquashMerged(localRef, defaultRef, branch) {
				if !wasPushed(r.A, remote, upstreams[branch], remoteSHAs) {
					fmt.Fprintf(stderr, "warning: '%s' looks squash-merged into '%s', but may have commits that were never pushed; keeping it\n",
						branch, defaultBranch)
					continue
				}
				merged = true
			}

			if !merged {
				fmt.Fprintf(stderr, "warning: '%s' was deleted on %s, but appears not merged into '%s'\n",
					branch, remote, defaultBranch)
				continue
			}

			if path, ok := otherWorktree(branch); ok {
				fmt.Fprintf(stderr, "warning: '%s' is checked out in %s; not deleting it\n", branch, path)
				continue
			}

			if dryRun {
				fmt.Fprintf(stdout, "%sWould delete branch %s%s%s (at %s).\n",
					red, brightRed, branch, reset, r.A[:7])
				continue
			}

			if branch == currentBranch {
				if err := git.Switch(defaultBranch); err != nil {
					return fmt.Errorf("failed to switch to %s: %w", defaultBranch, err)
				}
				currentBranch = defaultBranch
			}
			if err := git.DeleteBranch(branch); err != nil {
				fmt.Fprintf(stderr, "warning: failed to delete '%s': %s\n", branch, err)
				continue
			}
			fmt.Fprintf(stdout, "%sDeleted branch %s%s%s (was %s).\n",
				red, brightRed, branch, reset, r.A[:7])
		}
	}

	return nil
}

// wasPushed reports whether every commit on a gone branch reached the remote:
// its local tip is contained in either the upstream's last known commit or
// the head commit of a merged pull request for that upstream branch.
//
// The remote-tracking ref only survives until something runs fetch --prune
// (including git pull with fetch.prune=true), so the merged pull request is
// the usual evidence. Without either, the branch is treated as not pushed.
func wasPushed(localSHA, remote, upstreamRef string, remoteSHAs map[string]string) bool {
	if contains(remoteSHAs[upstreamRef], localSHA) {
		return true
	}
	if upstreamRef == "" {
		return false
	}

	url, err := git.RemoteURL(remote)
	if err != nil {
		return false
	}
	head := strings.TrimPrefix(upstreamRef, "refs/remotes/"+remote+"/")
	for _, sha := range mergedPRHeads(url, head) {
		if contains(sha, localSHA) {
			return true
		}
	}
	return false
}

// contains reports whether commit tip contains commit sha. It's false when
// tip is empty or isn't in the local object store.
func contains(tip, sha string) bool {
	return tip != "" && (tip == sha || git.IsAncestor(sha, tip))
}

// isSquashMerged detects whether a branch was squash-merged into the target.
//
// The trick: create a temporary commit whose tree matches the branch tip, parented
// at the merge-base of the two branches. Then ask git cherry whether that diff
// already exists in the target. A "-" prefix means the patch was already applied,
// which is exactly what a squash-merge looks like.
func isSquashMerged(branchRef, targetRef, branchName string) bool {
	ancestor, err := git.MergeBase(targetRef, branchRef)
	if err != nil {
		return false
	}

	tree, err := git.TreeHash(branchRef)
	if err != nil {
		return false
	}

	dangling, err := git.CommitTree(tree, ancestor, fmt.Sprintf("temp squash-merge check for %s", branchName))
	if err != nil {
		return false
	}

	result, err := git.Cherry(targetRef, dangling)
	if err != nil {
		return false
	}

	return strings.HasPrefix(result, "-")
}
