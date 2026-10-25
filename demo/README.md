# Demo GIF

`gh-sync.gif` is generated from `gh-sync.tape` with [VHS](https://github.com/charmbracelet/vhs). Regenerate it whenever the output or the README demo should change.

## Requirements

- [VHS](https://github.com/charmbracelet/vhs#installation) (`brew install vhs`)
- Go, to build gh-sync from this checkout
- git and bash
- [JetBrainsMono Nerd Font](https://www.nerdfonts.com/font-downloads) (optional, the tape tries MonoLisaCode first and VHS falls back to a default font)

## Regenerate

From the repo root:

```shell
vhs demo/gh-sync.tape
```

This overwrites `demo/gh-sync.gif`. Commit it along with any tape changes.

## How it works

- `setup.sh` builds a throwaway repo in `/tmp/gh-sync-demo`: a bare `remote.git` standing in for GitHub, a `teammate` clone that pushes, merges, and deletes branches, and `my-app` where the demo runs.
- Every case gh sync handles is staged: branches behind the remote, a merged branch, a squash-merged branch, unpushed work, and a branch deleted upstream without being merged.
- It builds gh-sync from this checkout and writes `env.sh`, which defines a `gh` function so `gh sync` runs the fresh build instead of the installed extension.
- Git runs with personal config ignored and a fixed name, email, and commit date, so signing, aliases, and hooks never interfere and commit hashes are the same on every render.
- Steps wait on screen text (`Wait+Screen`), so if the output changes the render fails instead of recording a broken demo.
