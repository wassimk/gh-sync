## gh-sync

A [`gh`](https://cli.github.com/) CLI extension that keeps local branches in sync with a remote. It fetches, fast-forwards outdated branches, warns about unpushed work, and cleans up branches whose upstream was deleted after merging (including squash merges).

This was built to replace the [`sync`](https://hub.github.com/hub-sync.1.html) command from the deprecated [`hub`](https://hub.github.com/) tool.

### Installation

```shell
gh extension install wassimk/gh-sync
```

### Usage

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
   - **Delete** if the upstream was removed and the branch was merged (or squash-merged) into the default branch
   - **Warn** if the upstream was removed but the branch appears unmerged

Branches without explicit tracking configuration are matched by name against the remote.

### Flags

```
--verbose, -v     Log each git command to stderr
```

### Examples

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

### Upgrade / Uninstall

```shell
gh extension upgrade wassimk/gh-sync
gh extension remove gh-sync
```

### Attribution

Inspired by [`jacobwgillespie/git-sync`](https://github.com/jacobwgillespie/git-sync), rebuilt as a `gh` CLI extension.
