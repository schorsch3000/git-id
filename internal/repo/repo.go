// Package repo applies identities to git repositories and reads which one a
// repository uses.
package repo

import (
	"fmt"
	"strings"

	"git-id/internal/gitcfg"
	"git-id/internal/store"
)

const (
	// SSHPrefix starts every core.sshCommand written by git-id.
	SSHPrefix = "git id ssh "
	// IdentityKey is the repo-local config key holding the identity label.
	IdentityKey = "id.identity"
)

// Apply writes an identity into the repo config of dir. A foreign
// core.sshCommand is only replaced with force.
func Apply(dir string, r store.Resolved, force bool) error {
	if !force {
		cur, ok, err := gitcfg.GetLocal(dir, "core.sshCommand")
		if err != nil {
			return err
		}
		if ok && !strings.HasPrefix(cur, SSHPrefix) {
			return fmt.Errorf("%s: core.sshCommand is already set to %q; use --force to replace it", dir, cur)
		}
	}
	for _, kv := range [][2]string{
		{"user.name", r.Name},
		{"user.email", r.Email},
		{IdentityKey, r.Label},
		{"core.sshCommand", SSHPrefix + r.Label},
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
	if err := Apply(top, r, force); err != nil {
		return "", err
	}
	if err := store.AddRepo(store.RegistryPath(bookPath), top); err != nil {
		return "", err
	}
	return top, nil
}
