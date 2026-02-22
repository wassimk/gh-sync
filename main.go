package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/wassimk/gh-sync/internal/git"
)

var errHelp = errors.New("help requested")

func main() {
	verbose, err := parseArgs(os.Args[1:])
	if err != nil {
		if errors.Is(err, errHelp) {
			printUsage(os.Stdout)
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}

	useColor := isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())
	git.Verbose = verbose
	git.Color = useColor

	if err := sync(os.Stdout, os.Stderr, useColor); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}
}

func parseArgs(args []string) (verbose bool, err error) {
	for _, arg := range args {
		switch {
		case arg == "--verbose" || arg == "-v":
			verbose = true
		case arg == "--help" || arg == "-h":
			return false, errHelp
		default:
			return false, fmt.Errorf("unknown argument: %s", arg)
		}
	}

	return verbose, nil
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, `Usage: gh sync [flags]

Fetch from the primary remote and update local branches.

If a local branch is outdated, fast-forward it.
If a local branch contains unpushed work, warn about it.
If a branch seems merged and its upstream was deleted, delete it.

Flags:
  --verbose, -v     Log each git command to stderr
  -h, --help        Show this help`)
}

func sync(stdout, stderr io.Writer, useColor bool) error {
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

			shouldDelete := r.IsAncestor()

			// If it wasn't a regular merge, check for a squash-merge.
			if !shouldDelete {
				shouldDelete = isSquashMerged(localRef, defaultRef, branch)
			}

			if shouldDelete {
				if branch == currentBranch {
					if err := git.Checkout(defaultBranch); err != nil {
						return fmt.Errorf("failed to checkout %s: %w", defaultBranch, err)
					}
					currentBranch = defaultBranch
				}
				if err := git.DeleteBranch(branch); err != nil {
					return fmt.Errorf("failed to delete %s: %w", branch, err)
				}
				fmt.Fprintf(stdout, "%sDeleted branch %s%s%s (was %s).\n",
					red, brightRed, branch, reset, r.A[:7])
			} else {
				fmt.Fprintf(stderr, "warning: '%s' was deleted on %s, but appears not merged into '%s'\n",
					branch, remote, defaultBranch)
			}
		}
	}

	return nil
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
