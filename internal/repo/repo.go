// Package repo applies identities to git repositories and reads which one a
// repository uses.
package repo

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"git-id/internal/gitcfg"
	"git-id/internal/store"
)

// IdentityKey is the repo-local config key holding the identity label.
const IdentityKey = "id.identity"

// sshCommandRe matches every core.sshCommand written by git-id, with or
// without the address book pinned through GIT_ID_BOOK.
var sshCommandRe = regexp.MustCompile(`^(GIT_ID_BOOK='(?:[^']|'\\'')*' )?git id ssh [A-Za-z0-9._-]+$`)

// IsOurSSHCommand reports whether cmd is a core.sshCommand written by git-id.
func IsOurSSHCommand(cmd string) bool { return sshCommandRe.MatchString(cmd) }

// SSHCommand returns the core.sshCommand for an identity. The wrapper looks
// the key up in the address book at run time, so when the book is not at the
// default location its path is pinned: git runs the command in a shell that
// knows neither --book nor a $GIT_ID_BOOK set in some other terminal.
func SSHCommand(bookPath, label string) string {
	cmd := "git id ssh " + label
	if def, err := store.DefaultPath(); err != nil || def != bookPath {
		cmd = "GIT_ID_BOOK=" + shellQuote(bookPath) + " " + cmd
	}
	return cmd
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// Apply writes an identity into the repo config of dir. A foreign
// core.sshCommand is only replaced with force.
func Apply(dir, bookPath string, r store.Resolved, force bool) error {
	if !force {
		cur, ok, err := gitcfg.GetLocal(dir, "core.sshCommand")
		if err != nil {
			return err
		}
		if ok && !IsOurSSHCommand(cur) {
			return fmt.Errorf("%s: core.sshCommand is already set to %q; use --force to replace it", dir, cur)
		}
	}
	for _, kv := range [][2]string{
		{"user.name", r.Name},
		{"user.email", r.Email},
		{IdentityKey, r.Label},
		{"core.sshCommand", SSHCommand(bookPath, r.Label)},
	} {
		if err := gitcfg.SetLocal(dir, kv[0], kv[1]); err != nil {
			return err
		}
	}
	return nil
}

// Identity returns the identity label set in the repository containing dir.
func Identity(dir string) (label string, ok bool, err error) {
	top, err := gitcfg.Toplevel(dir)
	if err != nil {
		return "", false, fmt.Errorf("not inside a git repository")
	}
	label, ok, err = gitcfg.GetLocal(top, IdentityKey)
	return label, ok && label != "", err
}

// Use applies identity label to the repository containing dir and registers
// the repository for `sync`. It returns the repository root.
func Use(bookPath string, b *store.Book, label, dir string, force bool) (string, error) {
	r, err := b.Resolve(label)
	if err != nil {
		return "", err
	}
	top, err := gitcfg.Toplevel(dir)
	if err != nil {
		return "", fmt.Errorf("not inside a git repository")
	}
	if err := Apply(top, bookPath, r, force); err != nil {
		return "", err
	}
	if err := store.AddRepo(store.RegistryPath(bookPath), top); err != nil {
		return "", err
	}
	return top, nil
}

// SSHOverrideWarning is shown when $GIT_SSH_COMMAND is set: git prefers it
// over core.sshCommand, so the key chosen by git-id would be bypassed.
const SSHOverrideWarning = "warning: $GIT_SSH_COMMAND is set in your environment; git prefers it over " +
	"core.sshCommand, so git-id's key selection is bypassed. Unset it."

// SSHOverridden reports whether $GIT_SSH_COMMAND overrides git-id's key selection.
func SSHOverridden() bool { return os.Getenv("GIT_SSH_COMMAND") != "" }
