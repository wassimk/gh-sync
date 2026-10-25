#!/usr/bin/env bash
#
# Builds a throwaway repo in /tmp/gh-sync-demo for recording the README demo (see demo/gh-sync.tape).
#
#   remote.git  bare repo standing in for GitHub
#   teammate    clone that pushes, merges, and deletes branches
#   my-app      the clone the demo runs `gh sync` in
#
# Every branch case gh sync handles is set up: behind, merged, squash-merged, unpushed, and unmerged.

set -euo pipefail

DEMO_DIR=/tmp/gh-sync-demo
REPO_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)

# Ignore personal git config (signing, aliases, hooks) and pin dates so hashes are stable
export GIT_CONFIG_GLOBAL=/dev/null
export GIT_CONFIG_NOSYSTEM=1
export GIT_AUTHOR_NAME='Demo User' GIT_AUTHOR_EMAIL='demo@example.com'
export GIT_COMMITTER_NAME='Demo User' GIT_COMMITTER_EMAIL='demo@example.com'
export GIT_AUTHOR_DATE='2026-01-01T12:00:00Z' GIT_COMMITTER_DATE='2026-01-01T12:00:00Z'

commit() {
  echo "$1" >>"$2"
  git add "$2"
  git commit -q -m "$1"
}

rm -rf "$DEMO_DIR"
mkdir -p "$DEMO_DIR"
cd "$DEMO_DIR"

git init -q --bare -b main remote.git

# Seed main and the feature branches
git clone -q remote.git teammate 2>/dev/null
cd teammate
commit 'Initial commit' README.md
commit 'Add app skeleton' app.txt

for branch in feature-login add-user-api fix-typo experiment spike; do
  git switch -q -c "$branch" main
  commit "Start $branch" "$branch.txt"
done
git switch -q main
git push -q --all origin

# Clone my-app and track every branch
cd "$DEMO_DIR"
git clone -q remote.git my-app 2>/dev/null
cd my-app
for branch in feature-login add-user-api fix-typo experiment spike; do
  git branch -q --track "$branch" "origin/$branch"
done

# Teammate activity on the remote
cd "$DEMO_DIR/teammate"

git switch -q feature-login
commit 'Add login form validation' feature-login.txt
git push -q origin feature-login

git switch -q main
git merge -q --no-ff -m 'Merge pull request #11 from add-user-api' add-user-api
git merge -q --squash fix-typo >/dev/null 2>&1
git commit -q -m 'Fix typo in README (#12)'
commit 'Bump version to 1.2.0' app.txt
git push -q origin main
git push -q origin --delete add-user-api fix-typo spike

# Local work in my-app that was never pushed
cd "$DEMO_DIR/my-app"
git switch -q experiment
commit 'Try a new cache layer' experiment.txt
git switch -q main

# Fetch without pruning so `git branch -vv` shows what's behind before gh sync runs
git fetch -q origin

# Build gh sync from this checkout, so the demo shows the current code
cd "$REPO_DIR"
go build -o "$DEMO_DIR/bin/gh-sync" .

# Environment for the demo shell: same git isolation, and `gh sync` runs the fresh build
cat >"$DEMO_DIR/env.sh" <<ENV
export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1
export GIT_AUTHOR_NAME='Demo User' GIT_AUTHOR_EMAIL='demo@example.com'
export GIT_COMMITTER_NAME='Demo User' GIT_COMMITTER_EMAIL='demo@example.com'
gh() { if [ "\$1" = sync ]; then shift; "$DEMO_DIR/bin/gh-sync" "\$@"; else command gh "\$@"; fi; }
ENV
