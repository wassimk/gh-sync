# README demo for gh-sync
#
# Render from the repo root:
#   vhs demo/gh-sync.tape
#
# demo/setup.sh builds a throwaway repo in /tmp/gh-sync-demo and a fresh gh-sync binary.

Output demo/gh-sync.gif

Require bash
Require git
Require go

Set Shell "bash"
Set FontFamily "MonoLisaCode, JetBrainsMono Nerd Font"
Set FontSize 16
Set Width 1000
Set Height 540
Set Padding 20
Set Theme "OneHalfDark"
Set TypingSpeed 60ms
Set CursorBlink false

# Setup: sandbox repo, demo environment, clean prompt
Hide
Type "bash demo/setup.sh && source /tmp/gh-sync-demo/env.sh && cd /tmp/gh-sync-demo/my-app"
Enter
Wait@60s />\s*$/
Type "PS1='\[\e[36m\]my-app\[\e[0m\] ❯ ' && clear"
Enter
Sleep 500ms
Show

Sleep 800ms

# Before: branches behind, merged, squash-merged, unpushed, and deleted upstream
Type "git branch -vv"
Sleep 400ms
Enter
Wait+Screen /spike/
Sleep 3s

Type "gh sync"
Sleep 500ms
Enter
Wait+Screen /not merged into 'main'/
Sleep 3s

# After: merged branches are gone, outdated ones fast-forwarded
Type "git branch -vv"
Sleep 400ms
Enter
Wait+Screen /\[origin\/spike: gone\]/

# Hold on the result before looping
Sleep 4s
