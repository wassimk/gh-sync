// Package proc finds which processes are working inside a directory.
package proc

import (
	"os/exec"
	"path/filepath"
	"strings"
)

// CwdUsers maps each directory that is some process's current working
// directory to the names of those processes. It uses lsof, and returns ok
// false when lsof isn't available or fails, so callers can treat every
// directory as possibly in use.
func CwdUsers() (users map[string][]string, ok bool) {
	out, err := exec.Command("lsof", "-a", "-d", "cwd", "-Fcn").Output()
	if err != nil && len(out) == 0 {
		return nil, false
	}

	users = make(map[string][]string)
	var name string
	for _, line := range strings.Split(string(out), "\n") {
		switch {
		case strings.HasPrefix(line, "c"):
			name = line[1:]
		case strings.HasPrefix(line, "n"):
			dir := resolve(line[1:])
			users[dir] = append(users[dir], name)
		}
	}
	return users, true
}

// UsersInside returns the names of processes whose working directory is dir
// or a directory inside it.
func UsersInside(users map[string][]string, dir string) []string {
	dir = resolve(dir)
	var names []string
	seen := map[string]bool{}
	for cwd, procs := range users {
		if cwd != dir && !strings.HasPrefix(cwd, dir+string(filepath.Separator)) {
			continue
		}
		for _, p := range procs {
			if !seen[p] {
				seen[p] = true
				names = append(names, p)
			}
		}
	}
	return names
}

// resolve follows symlinks so /var and /private/var compare equal on macOS.
func resolve(dir string) string {
	if r, err := filepath.EvalSymlinks(dir); err == nil {
		return r
	}
	return filepath.Clean(dir)
}
