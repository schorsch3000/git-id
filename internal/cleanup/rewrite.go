package cleanup

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"git-id/internal/gitcfg"
)

// filterRepoSub is the git subcommand provided by git-filter-repo. It is a
// variable so tests can simulate the tool being absent.
var filterRepoSub = "filter-repo"

// ErrNoFilterRepo explains how to get the external tool; git-id does not bundle it.
var ErrNoFilterRepo = fmt.Errorf("git-filter-repo is required to rewrite history but is not installed; " +
	"install it (for example `apt install git-filter-repo` or `pip install git-filter-repo`)")

// CheckFilterRepo reports whether `git filter-repo` is available.
func CheckFilterRepo(dir string) error {
	if _, err := gitcfg.Git(dir, filterRepoSub, "--version"); err != nil {
		return ErrNoFilterRepo
	}
	return nil
}

// IsDirty reports uncommitted changes to tracked files; the rewrite resets the
// work tree and would lose them. Untracked files are not touched by it.
func IsDirty(dir string) (bool, error) {
	out, err := gitcfg.Git(dir, "status", "--porcelain", "--untracked-files=no")
	return out != "", err
}

// PushedCount returns how many of hashes are reachable from remote-tracking
// refs, that is were probably pushed already.
func PushedCount(dir string, hashes []string) (int, error) {
	out, err := gitcfg.Git(dir, "rev-list", "--remotes")
	if err != nil {
		return 0, err
	}
	remote := map[string]bool{}
	for _, h := range strings.Split(out, "\n") {
		remote[h] = true
	}
	n := 0
	for _, h := range hashes {
		if remote[h] {
			n++
		}
	}
	return n, nil
}

// Backup writes a bundle of every ref into the git directory and returns its path.
func Backup(dir string) (string, error) {
	gitDir, err := gitcfg.Git(dir, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return "", err
	}
	path := filepath.Join(gitDir, "git-id-backup-"+time.Now().Format("20060102-150405")+".bundle")
	if _, err := gitcfg.Git(dir, "bundle", "create", path, "--all"); err != nil {
		return "", err
	}
	return path, nil
}

// RestoreHint is the command that puts the refs of a backup bundle back.
func RestoreHint(bundle string) string {
	return fmt.Sprintf("git fetch --update-head-ok %q '+refs/*:refs/*' && git reset --hard", bundle)
}

// bytesLiteral renders s as a python bytes literal with every byte escaped, so
// quotes, backslashes and non-ASCII text cannot break the callback code.
func bytesLiteral(s string) string {
	var sb strings.Builder
	sb.WriteString(`b"`)
	for i := 0; i < len(s); i++ {
		fmt.Fprintf(&sb, `\x%02x`, s[i])
	}
	sb.WriteString(`"`)
	return sb.String()
}

// callback returns the body of a filter-repo callback replacing the variable
// v according to m, or "" if there is nothing to replace.
func callback(v string, m map[string]string) string {
	if len(m) == 0 {
		return ""
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	pairs := make([]string, len(keys))
	for i, k := range keys {
		pairs[i] = bytesLiteral(k) + ": " + bytesLiteral(m[k])
	}
	return fmt.Sprintf("return {%s}.get(%s, %s)", strings.Join(pairs, ", "), v, v)
}

// Rewrite applies the plan with git filter-repo. With ref empty every ref is
// rewritten, otherwise only that one. --partial keeps the origin remote,
// remote-tracking refs and the reflog; the callbacks also cover tag authors.
func Rewrite(dir string, p Plan, ref string) error {
	args := []string{filterRepoSub, "--partial", "--force"}
	if ref != "" {
		args = append(args, "--refs", ref)
	}
	if cb := callback("name", p.Names); cb != "" {
		args = append(args, "--name-callback", cb)
	}
	if cb := callback("email", p.Emails); cb != "" {
		args = append(args, "--email-callback", cb)
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git filter-repo failed: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
