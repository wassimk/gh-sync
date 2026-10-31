// Package gh queries GitHub through the gh CLI that runs this extension.
package gh

import (
	"encoding/json"
	"os/exec"
)

// MergedPRHeads returns the head commit of every merged pull request whose
// head branch is named branch, in the repository at remoteURL. It returns nil
// when gh can't answer, such as for a remote that isn't on GitHub.
func MergedPRHeads(remoteURL, branch string) []string {
	out, err := exec.Command("gh", "pr", "list",
		"--repo", remoteURL,
		"--head", branch,
		"--state", "merged",
		"--json", "headRefOid",
	).Output()
	if err != nil {
		return nil
	}

	var prs []struct {
		HeadRefOid string `json:"headRefOid"`
	}
	if err := json.Unmarshal(out, &prs); err != nil {
		return nil
	}

	heads := make([]string, 0, len(prs))
	for _, pr := range prs {
		heads = append(heads, pr.HeadRefOid)
	}
	return heads
}
