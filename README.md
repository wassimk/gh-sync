## gh-sync

A [`gh`](https://cli.github.com/) CLI extension that keeps local branches in sync with a remote. It fetches, fast-forwards outdated branches, warns about unpushed work, and cleans up branches whose upstream was deleted after merging (including squash merges).

![gh-sync demo](demo/gh-sync.gif)

This was built to replace the [`sync`](https://hub.github.com/hub-sync.1.html) command from the deprecated [`hub`](https://hub.github.com/) tool.

### 🛠️ Installation

```shell
gh extension install wassimk/gh-sync
```

### 💻 Usage

Run inside any git repository:

```shell
gh sync
```

That's it. The command will:

1. Find the primary remote (`upstream` > `github` > `origin`)
2. Fetch with pruning
3. For each local branch:
   - **Fast-forward** if the branch is behind its remote counterpart
   - **Warn** if the branch has unpushed commits
   - **Delete** if the upstream was removed and the branch was merged into the default branch, or squash-merged with nothing left unpushed (confirmed against the merged pull request on GitHub)
   - **Warn** if the upstream was removed but the branch appears unmerged

Branches without explicit tracking configuration are matched by name against the remote.

A branch checked out in another worktree is fast-forwarded inside that worktree, so its files move with it. It isn't deleted unless you pass `--prune-worktrees`.

### ⚙️ Flags

```
--dry-run, -n       Show what would change without changing any branches
--prune-worktrees   Also remove worktrees of merged branches whose upstream
                    was deleted, when they're clean and not in use
--verbose, -v       Log each git command to stderr
```

`--prune-worktrees` only removes a worktree whose branch gh sync would otherwise delete, and only when `git status` there is clean and no process (a shell, editor, or dev server) has its working directory inside it. Ignored files in that worktree, such as `node_modules` or `.env`, are removed with it. Combine it with `--dry-run` to see the list first.

### 📝 Examples

Fast-forward a branch that fell behind:

```
Updated branch feature-login (was 3a1b2c4).
```

Warn about local work that hasn't been pushed:

```
warning: 'experiment' seems to contain unpushed commits
```

Clean up a merged branch whose remote was deleted:

```
Deleted branch add-user-api (was 7f8e9d0).
```

### 📦 Upgrade / Uninstall

```shell
gh extension upgrade wassimk/gh-sync
gh extension remove gh-sync
```

### 🙏 Attribution

Inspired by [`jacobwgillespie/git-sync`](https://github.com/jacobwgillespie/git-sync), rebuilt as a `gh` CLI extension.
